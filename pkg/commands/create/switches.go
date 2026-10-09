package create

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
)

const (
	// switchReadyTimeout bounds how long a switch may take to become reachable after the
	// simulation is ACTIVE. Cumulus VX switches can lose their first DHCP round and then
	// wait minutes for the client retry.
	switchReadyTimeout = 20 * time.Minute
	// switchMgmtIPTimeout bounds how long we wait for the management IP after restarting DHCP.
	switchMgmtIPTimeout    = 2 * time.Minute
	switchDHCPKickInterval = 5 * time.Second
	switchProgressInterval = 30 * time.Second

	// kickDHCPScript restarts the eth0 DHCP client so the lease is requested right away
	// instead of after the client's retry back-off.
	kickDHCPScript = `p=/run/dhclient.eth0.pid; if [ -f "$p" ]; then kill "$(cat "$p")" 2>/dev/null; fi; ifup --force eth0`
)

func resetSwitchPasswords(ctx context.Context, switchNodes []api.Node, bastionAddr, keyPath string) error {
	if len(switchNodes) == 0 {
		logging.Info("No switch nodes found, skipping switch password reset")
		return nil
	}

	logging.Info("Resetting switches passwords via bastion...")

	baseCfg := bastion.BastionExecConfig{
		BastionUser: constant.DefaultUbuntuUser,
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		TargetUser:  constant.DefaultCumulusUser,
		TargetPass:  constant.DefaultCumulusOldPassword,
	}
	oobIface := resolveBastionOOBInterface(switchNodes, baseCfg)

	errCh := make(chan error, len(switchNodes))
	var wg sync.WaitGroup

	for _, n := range switchNodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgmtIP, err := node.ResolveMgmtIP(n)
			if err != nil {
				logging.Verbose("Failed to resolve management IP for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("failed to resolve management IP for switch %s: %w", n.Name, err)
				return
			}
			if mgmtIP == "" {
				logging.Verbose("Management IP missing for switch %s", n.Name)
				errCh <- fmt.Errorf("switch %s does not have a management IP", n.Name)
				return
			}

			linkLocal := switchLinkLocalAddr(n, oobIface)
			switchAddr, viaLinkLocal, err := waitSwitchReachable(ctx, n.Name, baseCfg, mgmtIP, linkLocal)
			if err != nil {
				logging.Verbose("Switch %s not reachable: %v", n.Name, err)
				errCh <- fmt.Errorf("switch %s unreachable: %w", n.Name, err)
				return
			}

			if viaLinkLocal {
				logging.Info("Switch %s reachable via link-local %s, updating password...", n.Name, linkLocal)
			} else {
				logging.Info("Switch %s reachable, updating password...", n.Name)
			}

			if err := ssh.ChangeSwitchPasswordViaBastion(ssh.SwitchPasswordResetConfig{
				BastionAddr: bastionAddr,
				BastionUser: constant.DefaultUbuntuUser,
				BastionKey:  keyPath,
				SwitchAddr:  switchAddr,
				SwitchUser:  constant.DefaultCumulusUser,
				OldPassword: constant.DefaultCumulusOldPassword,
				NewPassword: constant.DefaultCumulusNewPassword,
				Timeout:     120 * time.Second,
			}); err != nil {
				logging.Verbose("Failed to update switch password for %s: %v", n.Name, err)
				errCh <- fmt.Errorf("update switch password for %s failed: %w", n.Name, err)
				return
			}

			logging.Info("✓ Switch %s password updated.", n.Name)

			if viaLinkLocal {
				if err := ensureSwitchMgmtIP(ctx, n.Name, baseCfg, switchAddr, mgmtIP); err != nil {
					logging.Verbose("Switch %s management IP not ready: %v", n.Name, err)
					errCh <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	if err := joinErrors(errCh); err != nil {
		return err
	}

	logging.Info("✓ Switch password reset completed.")
	return nil
}

// resolveBastionOOBInterface returns the bastion interface facing the switch management
// network, or "" when it cannot be determined.
func resolveBastionOOBInterface(switchNodes []api.Node, cfg bastion.BastionExecConfig) string {
	for _, n := range switchNodes {
		mgmtIP, err := node.ResolveMgmtIP(n)
		if err != nil || mgmtIP == "" {
			continue
		}
		iface, err := bastion.ResolveInterfaceViaBastion(cfg, mgmtIP)
		if err != nil {
			logging.Verbose("Could not resolve bastion interface towards %s: %v", mgmtIP, err)
			return ""
		}
		logging.Verbose("Bastion reaches switch management network via %s", iface)
		return iface
	}
	return ""
}

// switchLinkLocalAddr returns "[fe80::...%iface]:22" derived from the switch management
// MAC, or "" when it cannot be derived. Link-local works before the switch gets a DHCP lease.
func switchLinkLocalAddr(n api.Node, iface string) string {
	if iface == "" {
		return ""
	}
	mac := node.ResolveMgmtMAC(n)
	if mac == "" {
		return ""
	}
	ll, err := node.LinkLocalFromMAC(mac)
	if err != nil {
		logging.Verbose("Switch %s link-local address unavailable: %v", n.Name, err)
		return ""
	}
	return fmt.Sprintf("[%s%%%s]:22", ll, iface)
}

// waitSwitchReachable waits until the switch answers on its link-local address (independent
// of DHCP) or on its management IP, whichever comes first.
func waitSwitchReachable(ctx context.Context, name string, cfg bastion.BastionExecConfig, mgmtIP, linkLocal string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, switchReadyTimeout)
	defer cancel()

	type result struct {
		addr         string
		viaLinkLocal bool
		err          error
	}
	resCh := make(chan result, 2)
	pending := 1

	pingCfg := cfg
	pingCfg.TargetAddr = mgmtIP + ":22"
	go func() {
		err := bastion.WaitPingViaBastion(ctx, pingCfg, 0)
		resCh <- result{addr: pingCfg.TargetAddr, err: err}
	}()

	if linkLocal != "" {
		pending++
		go func() {
			err := bastion.WaitTCPViaBastion(ctx, cfg, linkLocal, 0)
			resCh <- result{addr: linkLocal, viaLinkLocal: true, err: err}
		}()
	}

	ticker := time.NewTicker(switchProgressInterval)
	defer ticker.Stop()
	start := time.Now()

	var errs []error
	for pending > 0 {
		select {
		case r := <-resCh:
			pending--
			if r.err == nil {
				return r.addr, r.viaLinkLocal, nil
			}
			errs = append(errs, r.err)
		case <-ticker.C:
			logging.Info("Still waiting for switch %s to become reachable (%s elapsed)...", name, time.Since(start).Round(time.Second))
		}
	}
	return "", false, errors.Join(errs...)
}

// ensureSwitchMgmtIP makes the switch request its DHCP lease right away and waits until the
// management IP answers, instead of waiting for the DHCP client retry back-off.
func ensureSwitchMgmtIP(ctx context.Context, name string, cfg bastion.BastionExecConfig, linkLocal, mgmtIP string) error {
	ctx, cancel := context.WithTimeout(ctx, switchMgmtIPTimeout)
	defer cancel()

	pingCfg := cfg
	pingCfg.TargetAddr = mgmtIP + ":22"

	kickCfg := cfg
	kickCfg.TargetAddr = linkLocal
	kickCfg.TargetPass = constant.DefaultCumulusNewPassword
	kickCfg.Command = fmt.Sprintf("printf '%%s\\n' %s | sudo -S -p '' sh -c %s",
		shellQuote(constant.DefaultCumulusNewPassword), shellQuote(kickDHCPScript))

	for {
		if err := bastion.WaitPingViaBastion(ctx, pingCfg, switchDHCPKickInterval); err == nil {
			logging.Info("✓ Switch %s management IP %s is reachable.", name, mgmtIP)
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("switch %s management IP %s not reachable after restarting DHCP: %w", name, mgmtIP, ctx.Err())
		}

		logging.Verbose("Restarting DHCP client on switch %s via %s", name, linkLocal)
		res, err := bastion.ExecCommandViaBastion(kickCfg)
		switch {
		case err != nil:
			logging.Verbose("Restarting DHCP on switch %s failed: %v", name, err)
		case res.ExitCode != 0:
			logging.Verbose("Restarting DHCP on switch %s exited with %d: %s", name, res.ExitCode, strings.TrimSpace(res.Stderr))
		}
	}
}

func copySwitchConfigs(directory string, switchNodes []api.Node, bastionAddr, keyPath string) error {
	if len(switchNodes) == 0 {
		return nil
	}

	logging.Info("Copying switch configs via bastion")

	errCh := make(chan error, len(switchNodes))
	var wg sync.WaitGroup

	for _, n := range switchNodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgmtIP, err := node.ResolveMgmtIP(n)
			if err != nil {
				logging.Verbose("Failed to resolve management IP for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("failed to resolve management IP for switch %s: %w", n.Name, err)
				return
			}
			if mgmtIP == "" {
				logging.Verbose("Management IP missing for switch %s", n.Name)
				errCh <- fmt.Errorf("switch %s does not have a management IP", n.Name)
				return
			}

			configPath := filepath.Join(directory, n.Name+".yaml")
			if _, err := os.Stat(configPath); err != nil {
				if os.IsNotExist(err) {
					logging.Verbose("Config not found for switch %s at %s, skipping", n.Name, configPath)
					return
				}
				logging.Verbose("Stat config failed for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("stat config failed for switch %s: %w", n.Name, err)
				return
			}

			logging.Info("Copying config for switch %s (%s)...", n.Name, mgmtIP)
			if err := ssh.CopyFileViaBastion(ssh.BastionCopyConfig{
				BastionAddr: bastionAddr,
				BastionUser: constant.DefaultUbuntuUser,
				BastionKey:  keyPath,
				TargetAddr:  mgmtIP + ":22",
				TargetUser:  constant.DefaultCumulusUser,
				TargetPass:  constant.DefaultCumulusNewPassword,
				Timeout:     120 * time.Second,
			}, configPath, constant.SwitchConfigRemotePath); err != nil {
				logging.Verbose("Failed to copy config to switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("copy config to switch %s failed: %w", n.Name, err)
				return
			}

			logging.Info("✓ Config copied to switch %s.", n.Name)
		}()
	}

	wg.Wait()
	close(errCh)
	if err := joinErrors(errCh); err != nil {
		return err
	}

	logging.Info("✓ Switch configs copied.")
	return nil
}

func applySwitchConfigs(switchNodes []api.Node, bastionAddr, keyPath string) error {
	if len(switchNodes) == 0 {
		return nil
	}

	logging.Info("Applying switch configs on switches...")

	errCh := make(chan error, len(switchNodes))
	var wg sync.WaitGroup

	for _, n := range switchNodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgmtIP, err := node.ResolveMgmtIP(n)
			if err != nil {
				logging.Verbose("Failed to resolve management IP for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("failed to resolve management IP for switch %s: %w", n.Name, err)
				return
			}
			if mgmtIP == "" {
				logging.Verbose("Management IP missing for switch %s", n.Name)
				errCh <- fmt.Errorf("switch %s does not have a management IP", n.Name)
				return
			}

			cmd := fmt.Sprintf("nv config replace %s && nv config apply -y", constant.SwitchConfigRemotePath)
			execCfg := bastion.BastionExecConfig{
				BastionUser: constant.DefaultUbuntuUser,
				BastionAddr: bastionAddr,
				BastionKey:  keyPath,
				TargetUser:  constant.DefaultCumulusUser,
				TargetAddr:  mgmtIP + ":22",
				// The password here is a placeholder and is only used for resetting.
				// The actual password can be set in the `hashed-password` field of the
				// switch configuration file (examples/simple/switch-gpu-leaf1.yaml).
				TargetPass: constant.DefaultCumulusNewPassword,
				Command:    cmd,
			}
			res, err := execSwitchApplyWithRetry(execCfg, n.Name)
			if err != nil {
				logging.Verbose("Failed to apply config on switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("apply config on switch %s failed: %w", n.Name, err)
				return
			}
			if res != nil && res.ExitCode != 0 {
				logging.Verbose("Apply config stderr for switch %s: %s", n.Name, res.Stderr)
				errCh <- fmt.Errorf("apply config on switch %s failed with exit %d", n.Name, res.ExitCode)
				return
			}

			logging.Info("✓ Config applied on switch %s.", n.Name)
		}()
	}

	wg.Wait()
	close(errCh)
	if err := joinErrors(errCh); err != nil {
		return err
	}

	logging.Info("✓ Switch configs applied.")
	return nil
}

func execSwitchApplyWithRetry(cfg bastion.BastionExecConfig, switchName string) (*bastion.ExecResult, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}

		res, err := bastion.ExecCommandViaBastion(cfg)
		if err == nil {
			return res, nil
		}

		lastErr = err
		if !shouldRetrySwitchApply(err) {
			return nil, err
		}

		logging.Verbose("Retrying apply config on switch %s after transient error (attempt %d/3): %v", switchName, attempt+1, err)
		time.Sleep(time.Second)
	}
	return nil, lastErr
}

func shouldRetrySwitchApply(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "ssh dial failed") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connect failed") ||
		strings.Contains(msg, "handshake failed") ||
		strings.Contains(msg, "connection timed out")
}
