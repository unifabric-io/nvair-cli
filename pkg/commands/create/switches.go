package create

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
)

func resetSwitchPasswords(ctx context.Context, switchNodes []api.Node, bastionAddr, keyPath string) error {
	if len(switchNodes) == 0 {
		logging.Info("No switch nodes found, skipping switch password reset")
		return nil
	}

	logging.Info("Resetting switches passwords via bastion...")

	errCh := make(chan error, len(switchNodes))
	var wg sync.WaitGroup

	for _, n := range switchNodes {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta, err := node.ParseNodeMetadata(n.Metadata)
			if err != nil {
				logging.Verbose("Failed to parse metadata for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("parse metadata failed for switch %s: %w", n.Name, err)
				return
			}

			pingCfg := bastion.BastionExecConfig{
				BastionUser: constant.DefaultUbuntuUser,
				BastionAddr: bastionAddr,
				BastionKey:  keyPath,
				TargetUser:  constant.DefaultCumulusUser,
				TargetAddr:  meta.MgmtIP + ":22",
				TargetPass:  constant.DefaultCumulusOldPassword,
				Command:     "",
			}

			if err := bastion.WaitPingViaBastion(ctx, pingCfg, 180*time.Second); err != nil {
				logging.Verbose("Switch %s not reachable: %v", n.Name, err)
				errCh <- fmt.Errorf("switch %s unreachable: %w", n.Name, err)
				return
			}

			logging.Info("Switch %s reachable, updating password...", n.Name)

			if err := ssh.ChangeSwitchPasswordViaBastion(ssh.SwitchPasswordResetConfig{
				BastionAddr: bastionAddr,
				BastionUser: constant.DefaultUbuntuUser,
				BastionKey:  keyPath,
				SwitchAddr:  meta.MgmtIP + ":22",
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
			meta, err := node.ParseNodeMetadata(n.Metadata)
			if err != nil {
				logging.Verbose("Failed to parse metadata for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("parse metadata failed for switch %s: %w", n.Name, err)
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

			logging.Info("Copying config for switch %s (%s)...", n.Name, meta.MgmtIP)
			if err := ssh.CopySwitchConfigViaBastion(ssh.SwitchCopyConfig{
				BastionAddr:    bastionAddr,
				BastionUser:    constant.DefaultUbuntuUser,
				BastionKey:     keyPath,
				SwitchAddr:     meta.MgmtIP + ":22",
				SwitchUser:     constant.DefaultCumulusUser,
				SwitchPassword: constant.DefaultCumulusNewPassword,
				Timeout:        120 * time.Second,
			}, configPath, "/home/cumulus/config.yml"); err != nil {
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
			meta, err := node.ParseNodeMetadata(n.Metadata)
			if err != nil {
				logging.Verbose("Failed to parse metadata for switch %s: %v", n.Name, err)
				errCh <- fmt.Errorf("parse metadata failed for switch %s: %w", n.Name, err)
				return
			}

			cmd := "nv config replace /home/cumulus/config.yml && nv config apply -y"
			res, err := bastion.ExecCommandViaBastion(bastion.BastionExecConfig{
				BastionUser: constant.DefaultUbuntuUser,
				BastionAddr: bastionAddr,
				BastionKey:  keyPath,
				TargetUser:  constant.DefaultCumulusUser,
				TargetAddr:  meta.MgmtIP + ":22",
				// The password here is a placeholder and is only used for resetting.
				// The actual password can be set in the `hashed-password` field of the
				// switch configuration file (examples/simple/switch-gpu-leaf1.yaml).
				TargetPass: constant.DefaultCumulusNewPassword,
				Command:    cmd,
			})
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
