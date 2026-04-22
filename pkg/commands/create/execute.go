package create

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
)

// Execute runs the create command.
func (cc *Command) Execute() error {
	if cc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Create command started")

	if cc.Directory == "" {
		logging.Verbose("Directory flag is required")
		return fmt.Errorf("directory flag is required (-d or --directory)")
	}

	logging.Verbose("Directory specified: %s, dry-run: %v", cc.Directory, cc.DryRun)

	topo, err := loadTopology(cc.Directory)
	if err != nil {
		return err
	}

	if err := validateGenericUbuntuNodeNetplans(cc.Directory, topo); err != nil {
		return err
	}

	if cc.DryRun {
		logging.Info("✓ Topology loaded and validated (dry-run)")
		return nil
	} else {
		logging.Info("✓ Topology loaded and validated")
	}

	apiClient, _, err := ensureAuthenticatedClient(cc.APIEndpoint)
	if err != nil {
		return err
	}

	if err := deleteDuplicateSimulations(apiClient, topo, cc.DeleteIfExists); err != nil {
		return err
	}

	simResp, err := apiClient.CreateSimulation(topo)
	if err != nil {
		logging.Verbose("API request failed: %v", err)
		return fmt.Errorf("failed to create simulation: %w", err)
	}

	logging.Info("✓ Simulation created successfully. ID: %s, Name: %s", simResp.ID, simResp.Title)

	ctrlResp, err := apiClient.ControlSimulation(simResp.ID, "load")
	if err != nil {
		logging.Verbose("Failed to set simulation state: %v", err)
		return fmt.Errorf("failed to set simulation state to load: %w", err)
	}

	logging.Info("✓ Simulation state set to 'load', result: %s, jobs: %v", ctrlResp.Result, ctrlResp.Jobs)

	if len(ctrlResp.Jobs) > 0 {
		logging.Info("Waiting for %d jobs to complete ...", len(ctrlResp.Jobs))

		if err := cc.WaitForJobs(apiClient, ctrlResp.Jobs); err != nil {
			logging.Verbose("Job wait failed: %v", err)
			return err
		}
		logging.Info("✓ All jobs completed successfully.")
	}

	nodes, err := apiClient.GetNodes(simResp.ID)
	if err != nil {
		logging.Verbose("Failed to fetch nodes: %v", err)
		return fmt.Errorf("failed to fetch nodes: %w", err)
	}

	images, err := apiClient.GetImages()
	if err != nil {
		logging.Verbose("Failed to fetch images: %v", err)
		return fmt.Errorf("failed to fetch images: %w", err)
	}
	nodes = resolveNodeImageNames(nodes, images)

	oobMgmtServerID, err := findOOBMgmtServer(nodes)
	if err != nil {
		return err
	}

	interfaces, err := apiClient.GetNodeInterfaces(simResp.ID, oobMgmtServerID)
	if err != nil {
		logging.Verbose("Failed to fetch interfaces: %v", err)
		return fmt.Errorf("failed to fetch node interfaces: %w", err)
	}

	outboundInterfaceID, err := findOutboundInterface(interfaces)
	if err != nil {
		return err
	}
	logging.Info("✓ Found outbound interface on oob-mgmt-server. Interface ID: %s", outboundInterfaceID)

	sshResponse, err := apiClient.CreateSSHService(simResp.ID, outboundInterfaceID)
	if err != nil {
		logging.Verbose("Failed to create SSH service: %v", err)
		return fmt.Errorf("failed to create SSH service: %w", err)
	}

	logging.Info("✓ SSH service created successfully. Host: %s, Port: %d", sshResponse.Host, sshResponse.SrcPort)
	bastionAddr := fmt.Sprintf("%s:%d", sshResponse.Host, sshResponse.SrcPort)

	keyPath, err := ssh.DefaultKeyPath()
	if err != nil {
		logging.Verbose("Failed to get default SSH key path: %v", err)
		return fmt.Errorf("failed to get SSH key path: %w", err)
	}

	logging.Info("Waiting for SSH access to become ready...")
	if err := ssh.WaitForPublicKeyAuth(ssh.WaitForSSHConfig{
		Addr:           bastionAddr,
		User:           constant.DefaultUbuntuUser,
		PrivateKey:     keyPath,
		ConnectTimeout: 10 * time.Second,
		ReadyTimeout:   120 * time.Second,
		PollInterval:   2 * time.Second,
	}); err != nil {
		logging.Verbose("SSH access did not become ready: %v", err)
		return fmt.Errorf("ssh service became reachable but did not accept public key authentication in time: %w", err)
	}

	logging.Verbose("Changing remote password for %s user", constant.DefaultUbuntuUser)

	if err := ssh.ChangeRemotePassword(ssh.ChangePasswordConfig{
		Addr:        bastionAddr,
		User:        constant.DefaultUbuntuUser,
		PrivateKey:  keyPath,
		OldPassword: constant.DefaultUbuntuPassword,
		// This is just a placeholder to skip the password reset on the jump host.
		// It is safe because we use SSH keys to connect.
		NewPassword: constant.DefaultBastionNewPassword,
		Timeout:     15 * time.Second,
	}); err != nil {
		logging.Verbose("Failed to change remote password: %v", err)
		return fmt.Errorf("failed to change remote password: %w", err)
	}

	logging.Verbose("Remote password changed successfully")

	switchNodes := filterCumulusSwitchNodes(nodes)

	if err := resetSwitchPasswords(context.Background(), switchNodes, bastionAddr, keyPath); err != nil {
		return err
	}

	if err := copySwitchConfigs(cc.Directory, switchNodes, bastionAddr, keyPath); err != nil {
		return err
	}

	if err := applySwitchConfigs(switchNodes, bastionAddr, keyPath); err != nil {
		return err
	}

	netplanTargets := filterGenericUbuntuNodes(nodes)
	if err := uploadNetplanConfigs(cc.Directory, netplanTargets, bastionAddr, keyPath); err != nil {
		return err
	}

	logging.Info("✓ Create simulation successfully.")
	logging.Info("  Bastion SSH address: %s", bastionAddr)
	logging.Info("  Bastion SSH command: %s", formatBastionSSHCommand(sshResponse.Host, sshResponse.SrcPort, keyPath))
	logging.Info("  To get more details about the simulation:")
	logging.Info("    nvair get simulation")
	logging.Info("    nvair get nodes -s %s", simResp.Title)
	logging.Info("    nvair get forwards -s %s", simResp.Title)
	return nil
}

func formatBastionSSHCommand(host string, port int, keyPath string) string {
	return fmt.Sprintf("ssh -i %s -p %d %s@%s", shellQuote(keyPath), port, constant.DefaultUbuntuUser, host)
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'"'"'`) + "'"
}
