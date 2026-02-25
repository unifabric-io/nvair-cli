package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

// CreateCommand represents the create subcommand for creating simulations
type CreateCommand struct {
	Directory      string
	DryRun         bool
	APIEndpoint    string
	Verbose        bool
	InstallDocker  bool
	InstallKubeadm bool
}

// NewCreateCommand creates a new CreateCommand instance
func NewCreateCommand() *CreateCommand {
	return &CreateCommand{
		APIEndpoint:    "https://air.nvidia.com/api",
		InstallDocker:  true, // Default to true
		InstallKubeadm: true, // Default to true
	}
}

// Register registers the create command flags
func (cc *CreateCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&cc.Directory, "d", "", "Directory path containing topology.json (required)")
	fs.StringVar(&cc.Directory, "directory", "", "Directory path containing topology.json (required)")
	fs.BoolVar(&cc.DryRun, "dry-run", false, "Validate topology without creating simulation")
	fs.StringVar(&cc.APIEndpoint, "api-endpoint", "https://air.nvidia.com/api", "API endpoint URL")
	fs.BoolVar(&cc.InstallDocker, "install-docker", true, "Install Docker on nodes (default: true)")
	fs.BoolVar(&cc.InstallKubeadm, "install-kubeadm", true, "Install Kubeadm on GPU nodes (default: true)")
	fs.BoolVar(&cc.Verbose, "v", false, "Enable verbose output")
	fs.BoolVar(&cc.Verbose, "verbose", false, "Enable verbose output")
}

// Execute runs the create command
func (cc *CreateCommand) Execute() error {
	// Enable verbose logging if requested
	if cc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Create command started")

	// Validate required flags
	if cc.Directory == "" {
		logging.Verbose("Directory flag is required")
		return fmt.Errorf("directory flag is required (-d or --directory)")
	}

	logging.Verbose("Directory specified: %s, dry-run: %v", cc.Directory, cc.DryRun)

	// Load topology from directory
	logging.Verbose("Loading topology from directory: %s", cc.Directory)
	topo, err := topology.LoadTopologyFromDirectory(cc.Directory)
	if err != nil {
		logging.Verbose("Failed to load topology: %v", err)
		return fmt.Errorf("failed to load topology: %w", err)
	}
	logging.Verbose("Topology loaded successfully: %s", topo.Title)

	// Validate topology
	logging.Verbose("Validating topology structure")
	result := topology.ValidateTopology(topo)
	if !result.Valid {
		logging.Verbose("Topology validation failed with %d errors", len(result.Errors))
		fmt.Fprintf(os.Stderr, "%s", topology.FormatValidationErrors(result.Errors))
		return fmt.Errorf("topology validation failed")
	}
	logging.Verbose("Topology validation passed")

	// If dry-run, stop here
	if cc.DryRun {
		logging.Verbose("Dry-run mode enabled, skipping API call")
		fmt.Printf("✓ Topology validation passed. Ready to create.\n")
		return nil
	}

	// Check authentication
	logging.Verbose("Checking authentication status")
	cfg, err := config.Load()
	if err != nil || cfg.BearerToken == "" {
		logging.Verbose("Not authenticated")
		return fmt.Errorf("not authenticated. Please run 'nvcli login' first")
	}
	logging.Verbose("Authentication verified")

	// Check if token is expired
	logging.Verbose("Checking token expiration")
	if cfg.IsTokenExpired(time.Now()) {
		logging.Verbose("Bearer token has expired, attempting to refresh with saved API token")

		// Try to refresh token using saved API token
		if cfg.APIToken == "" {
			logging.Verbose("No saved API token available for refresh")
			return fmt.Errorf("authentication token has expired and no API token available. Please run 'nvcli login' again")
		}

		// Attempt to get new bearer token
		apiClient := api.NewClient(cc.APIEndpoint, "")
		newBearerToken, expiresAt, err := apiClient.AuthLogin(cfg.Username, cfg.APIToken)
		if err != nil {
			logging.Verbose("Failed to refresh token: %v", err)
			return fmt.Errorf("authentication token expired and refresh failed: %w", err)
		}

		// Update config with new token
		logging.Verbose("Successfully refreshed bearer token")
		cfg.BearerToken = newBearerToken
		cfg.BearerTokenExpiresAt = expiresAt

		// Save updated config
		if err := cfg.Save(); err != nil {
			logging.Verbose("Warning: Failed to save refreshed token: %v", err)
			// Don't fail here - we have a valid token even if we can't save it
		}
	}
	logging.Verbose("Token is valid")

	// Create simulation via API
	logging.Verbose("Submitting topology to API for simulation creation")
	apiClient := api.NewClient(cc.APIEndpoint, cfg.BearerToken)
	simResp, err := apiClient.CreateSimulation(topo)
	if err != nil {
		logging.Verbose("API request failed: %v", err)
		return fmt.Errorf("failed to create simulation: %w", err)
	}

	logging.Verbose("Simulation created successfully with ID: %s, Title: %s", simResp.ID, simResp.Title)
	fmt.Printf("✓ Simulation created successfully. ID: %s, Name: %s\n", simResp.ID, simResp.Title)

	// Set simulation state to "load"
	logging.Verbose("Setting simulation state to 'load'")
	ctrlResp, err := apiClient.ControlSimulation(simResp.ID, "load")
	if err != nil {
		logging.Verbose("Failed to set simulation state: %v", err)
		return fmt.Errorf("failed to set simulation state to load: %w", err)
	}

	logging.Verbose("Simulation state set to 'load', result: %s, jobs: %v", ctrlResp.Result, ctrlResp.Jobs)
	fmt.Printf("✓ Simulation state set to 'load'. Jobs: %v\n", ctrlResp.Jobs)

	// Wait for all jobs to complete
	if len(ctrlResp.Jobs) > 0 {
		logging.Verbose("Waiting for %d jobs to complete", len(ctrlResp.Jobs))
		fmt.Printf("Waiting for jobs to complete...\n")
		err = cc.WaitForJobs(apiClient, ctrlResp.Jobs)
		if err != nil {
			logging.Verbose("Job wait failed: %v", err)
			return err
		}
		fmt.Printf("✓ All jobs completed successfully.\n")
	}

	// Fetch nodes and find oob-mgmt-server
	logging.Verbose("Fetching simulation nodes")
	nodes, err := apiClient.GetNodes(simResp.ID)
	if err != nil {
		logging.Verbose("Failed to fetch nodes: %v", err)
		return fmt.Errorf("failed to fetch nodes: %w", err)
	}

	var oobMgmtServerID string
	for _, node := range nodes {
		if node.Name == "oob-mgmt-server" {
			oobMgmtServerID = node.ID
			break
		}
	}

	if oobMgmtServerID == "" {
		logging.Verbose("oob-mgmt-server node not found")
		return fmt.Errorf("oob-mgmt-server node not found in simulation")
	}

	logging.Verbose("Found oob-mgmt-server with ID: %s", oobMgmtServerID)

	// Fetch interfaces for oob-mgmt-server
	logging.Verbose("Fetching interfaces for oob-mgmt-server")
	interfaces, err := apiClient.GetNodeInterfaces(simResp.ID, oobMgmtServerID)
	if err != nil {
		logging.Verbose("Failed to fetch interfaces: %v", err)
		return fmt.Errorf("failed to fetch node interfaces: %w", err)
	}

	// Find outbound interface
	var outboundInterfaceID string
	for _, intf := range interfaces {
		if intf.Outbound {
			outboundInterfaceID = intf.ID
			break
		}
	}

	if outboundInterfaceID == "" {
		logging.Verbose("No outbound interface found on oob-mgmt-server")
		return fmt.Errorf("no outbound interface found on oob-mgmt-server node")
	}

	fmt.Printf("✓ Found outbound interface on oob-mgmt-server. Interface ID: %s\n", outboundInterfaceID)

	// Create SSH service for the outbound interface
	logging.Verbose("Creating SSH service for the outbound interface")
	sshResponse, err := apiClient.CreateSSHService(simResp.ID, outboundInterfaceID)
	if err != nil {
		logging.Verbose("Failed to create SSH service: %v", err)
		return fmt.Errorf("failed to create SSH service: %w", err)
	}

	fmt.Printf("✓ SSH service created successfully. Host: %s, Port: %d\n", sshResponse.Host, sshResponse.SrcPort)

	// Change remote password
	logging.Verbose("Changing remote password for ubuntu user")
	time.Sleep(time.Second * 6)

	keyPath, err := ssh.DefaultKeyPath()
	if err != nil {
		logging.Verbose("Failed to get default SSH key path: %v", err)
		return fmt.Errorf("failed to get SSH key path: %w", err)
	}

	err = ssh.ChangeRemotePassword(ssh.ChangePasswordConfig{
		Addr:        sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
		User:        "ubuntu",
		PrivateKey:  keyPath,
		OldPassword: "nvidia",
		NewPassword: "dangerous",
		Timeout:     15 * time.Second,
	})
	if err != nil {
		logging.Verbose("Failed to change remote password: %v", err)
		return fmt.Errorf("failed to change remote password: %w", err)
	}

	logging.Verbose("Remote password changed successfully")
	fmt.Printf("✓ Remote password changed successfully.\n")

	// Reset switch passwords via bastion before further installs
	logging.Verbose("Resetting switch passwords via bastion")
	fmt.Printf("Resetting switch passwords on switches...\n")

	var switchNodes []api.Node
	for _, n := range nodes {
		if strings.HasPrefix(n.Name, "switch") {
			switchNodes = append(switchNodes, n)
		}
	}

	if len(switchNodes) == 0 {
		logging.Verbose("No switch nodes found, skipping switch password reset")
		fmt.Printf("No switches found, skip password reset.\n")
	} else {
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

				// Wait for switch reachable via bastion
				pingCfg := bastion.BastionExecConfig{
					BastionUser: "ubuntu",
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionKey:  keyPath,
					TargetUser:  "cumulus",
					TargetAddr:  meta.MgmtIP + ":22",
					TargetPass:  "cumulus",
					Command:     "",
				}

				if err := bastion.WaitPingViaBastion(context.Background(), pingCfg, 180*time.Second); err != nil {
					logging.Verbose("Switch %s not reachable: %v", n.Name, err)
					errCh <- fmt.Errorf("switch %s unreachable: %w", n.Name, err)
					return
				}

				fmt.Printf("Switch %s reachable, updating password...\n", n.Name)

				if err := ssh.ChangeSwitchPasswordViaBastion(ssh.SwitchPasswordResetConfig{
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionUser: "ubuntu",
					BastionKey:  keyPath,
					SwitchAddr:  meta.MgmtIP + ":22",
					SwitchUser:  "cumulus",
					OldPassword: "cumulus",
					NewPassword: "Dangerous1#",
					Timeout:     120 * time.Second,
				}); err != nil {
					logging.Verbose("Failed to update switch password for %s: %v", n.Name, err)
					errCh <- fmt.Errorf("update switch password for %s failed: %w", n.Name, err)
					return
				}

				fmt.Printf("✓ Switch %s password updated.\n", n.Name)
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}

		fmt.Printf("✓ Switch password reset completed.\n")
	}

	// scp switch config file to switches via bastion
	if len(switchNodes) > 0 {
		logging.Verbose("Copying switch configs via bastion")
		fmt.Printf("Copying switch configs via bastion...\n")

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

				configPath := filepath.Join(cc.Directory, n.Name+".yaml")
				if _, err := os.Stat(configPath); err != nil {
					if os.IsNotExist(err) {
						logging.Verbose("Config not found for switch %s at %s, skipping", n.Name, configPath)
						return
					}
					logging.Verbose("Stat config failed for switch %s: %v", n.Name, err)
					errCh <- fmt.Errorf("stat config failed for switch %s: %w", n.Name, err)
					return
				}

				fmt.Printf("Copying config for switch %s (%s)...\n", n.Name, meta.MgmtIP)
				if err := ssh.CopySwitchConfigViaBastion(ssh.SwitchPasswordResetConfig{
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionUser: "ubuntu",
					BastionKey:  keyPath,
					SwitchAddr:  meta.MgmtIP + ":22",
					SwitchUser:  "cumulus",
					OldPassword: "Dangerous1#",
					NewPassword: "",
					Timeout:     120 * time.Second,
				}, configPath, "/home/cumulus/config.yml"); err != nil {
					logging.Verbose("Failed to copy config to switch %s: %v", n.Name, err)
					errCh <- fmt.Errorf("copy config to switch %s failed: %w", n.Name, err)
					return
				}

				fmt.Printf("✓ Config copied to switch %s.\n", n.Name)
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}

		fmt.Printf("✓ Switch configs copied.\n")
	}

	// Apply switch configs via bastion
	if len(switchNodes) > 0 {
		logging.Verbose("Applying switch configs via bastion")
		fmt.Printf("Applying switch configs on switches...\n")

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
					BastionUser: "ubuntu",
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionKey:  keyPath,
					TargetUser:  "cumulus",
					TargetAddr:  meta.MgmtIP + ":22",
					TargetPass:  "Dangerous1#",
					Command:     cmd,
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

				fmt.Printf("✓ Config applied on switch %s.\n", n.Name)
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}

		fmt.Printf("✓ Switch configs applied.\n")
	}

	// Upload netplan configs and apply on nodes via bastion
	logging.Verbose("Uploading netplan configs via bastion")
	fmt.Printf("Uploading netplan configs on nodes via bastion...\n")

	var nodeNetplanTargets []api.Node
	for _, n := range nodes {
		if strings.HasPrefix(n.Name, "node") {
			nodeNetplanTargets = append(nodeNetplanTargets, n)
		}
	}

	if len(nodeNetplanTargets) == 0 {
		logging.Verbose("No compute nodes found for netplan upload, skipping")
		fmt.Printf("No node netplan files to upload.\n")
	} else {
		errCh := make(chan error, len(nodeNetplanTargets))
		var wg sync.WaitGroup

		for _, n := range nodeNetplanTargets {
			n := n
			wg.Add(1)
			go func() {
				defer wg.Done()

				meta, err := node.ParseNodeMetadata(n.Metadata)
				if err != nil {
					logging.Verbose("Failed to parse metadata for node %s: %v", n.Name, err)
					errCh <- fmt.Errorf("parse metadata failed for node %s: %w", n.Name, err)
					return
				}

				fileName := n.Name
				if !strings.HasSuffix(fileName, ".netplan.yaml") {
					fileName += ".netplan.yaml"
				}
				configPath := filepath.Join(cc.Directory, fileName)

				if _, err := os.Stat(configPath); err != nil {
					if os.IsNotExist(err) {
						logging.Verbose("Netplan config not found for node %s at %s, skipping", n.Name, configPath)
						fmt.Printf("No netplan file for node %s, skipping.\n", n.Name)
						return
					}
					logging.Verbose("Stat netplan config failed for node %s: %v", n.Name, err)
					errCh <- fmt.Errorf("stat netplan config failed for node %s: %w", n.Name, err)
					return
				}

				tmpDst := "/tmp/dataplan"
				fmt.Printf("Copying netplan for node %s (%s)...\n", n.Name, meta.MgmtIP)
				if err := ssh.CopyFileViaBastion(ssh.BastionCopyConfig{
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionUser: "ubuntu",
					BastionKey:  keyPath,
					TargetAddr:  meta.MgmtIP + ":22",
					TargetUser:  "ubuntu",
					TargetPass:  "nvidia",
					Timeout:     60 * time.Second,
				}, configPath, tmpDst); err != nil {
					logging.Verbose("Failed to upload netplan for node %s: %v", n.Name, err)
					errCh <- fmt.Errorf("upload netplan for node %s failed: %w", n.Name, err)
					return
				}

				res, err := bastion.ExecCommandViaBastion(bastion.BastionExecConfig{
					BastionUser: "ubuntu",
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionKey:  keyPath,
					TargetUser:  "ubuntu",
					TargetAddr:  meta.MgmtIP + ":22",
					TargetPass:  "nvidia",
					Command:     fmt.Sprintf("sudo mv %s /etc/netplan/%s && sudo netplan apply", tmpDst, fileName),
				})
				if err != nil {
					logging.Verbose("Failed to apply netplan on node %s: %v", n.Name, err)
					errCh <- fmt.Errorf("apply netplan on node %s failed: %w", n.Name, err)
					return
				}
				if res != nil && res.ExitCode != 0 {
					logging.Verbose("Netplan apply stderr for node %s: %s", n.Name, res.Stderr)
					errCh <- fmt.Errorf("apply netplan on node %s failed with exit %d", n.Name, res.ExitCode)
					return
				}

				fmt.Printf("✓ Netplan uploaded and applied on node %s.\n", n.Name)
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}

		fmt.Printf("✓ Netplan configs uploaded and applied on nodes.\n")
	}

	// Install Docker if requested
	if cc.InstallDocker {
		logging.Verbose("Installing Docker on nodes")
		fmt.Printf("Installing Docker on nodes...\n")

		// Fetch all nodes
		allNodes, err := apiClient.GetNodes(simResp.ID)
		if err != nil {
			logging.Verbose("Failed to fetch all nodes: %v", err)
			return fmt.Errorf("failed to fetch nodes for docker installation: %w", err)
		}

		// Filter nodes that start with "node"
		var nodeList []api.Node
		for _, n := range allNodes {
			if strings.HasPrefix(n.Name, "node") {
				nodeList = append(nodeList, n)
			}
		}

		// Sort nodes by name
		node.SortNodesByName(nodeList)

		// Install Docker on each node concurrently
		errCh := make(chan error, len(nodeList))
		var wg sync.WaitGroup
		for _, n := range nodeList {
			n := n // capture loop var
			wg.Add(1)
			go func() {
				defer wg.Done()
				nodeMetadata, err := node.ParseNodeMetadata(n.Metadata)
				if err != nil {
					logging.Verbose("Failed to parse node metadata for %s: %v", n.Name, err)
					errCh <- fmt.Errorf("parse node metadata failed for %s: %w", n.Name, err)
					return
				}

				fmt.Printf("Installing docker on node %s (%s)...\n", n.Name, nodeMetadata.MgmtIP)
				_, _, err = bastion.InstallDockerViaBastion(bastion.BastionExecConfig{
					BastionUser: "ubuntu",
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionKey:  keyPath,
					TargetUser:  "ubuntu",
					TargetAddr:  nodeMetadata.MgmtIP + ":22",
					TargetPass:  "nvidia",
					Command:     "",
				}, time.Second*240)
				if err != nil {
					logging.Verbose("Failed to install docker on node %s: %v", n.Name, err)
					errCh <- fmt.Errorf("install docker on node %s failed: %w", n.Name, err)
					return
				}
				fmt.Printf("✓ Docker installed on node %s successfully.\n", n.Name)
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				return err
			}
		}

		fmt.Printf("✓ Docker installation completed.\n")
	}

	// Install Nginx on bastion
	logging.Verbose("Installing Nginx on bastion")
	fmt.Printf("Installing Nginx on bastion...\n")

	err = bastion.InstallNginx(context.Background(), bastion.BastionExecConfig{
		BastionUser: "ubuntu",
		BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
		BastionKey:  keyPath,
		TargetUser:  "ubuntu",
		TargetAddr:  sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
		TargetPass:  "dangerous",
		Command:     "",
	})
	if err != nil {
		logging.Verbose("Failed to install nginx: %v", err)
		return fmt.Errorf("failed to install nginx: %w", err)
	}

	logging.Verbose("Nginx installed successfully")
	fmt.Printf("✓ Nginx installed successfully.\n")

	// Install Kubeadm if requested
	if cc.InstallKubeadm {
		logging.Verbose("Installing Kubeadm on GPU nodes")
		fmt.Printf("Installing Kubeadm on GPU nodes...\n")

		// Fetch all nodes
		allNodes, err := apiClient.GetNodes(simResp.ID)
		if err != nil {
			logging.Verbose("Failed to fetch all nodes: %v", err)
			return fmt.Errorf("failed to fetch nodes for kubeadm installation: %w", err)
		}

		// Filter GPU nodes
		gpuNodes := node.FilterGPUNodes(allNodes)
		if len(gpuNodes) == 0 {
			logging.Verbose("No GPU nodes found for kubeadm installation")
			fmt.Printf("⚠ No GPU nodes found for kubeadm installation.\n")
		} else {
			// Sort nodes by name
			node.SortNodesByName(gpuNodes)

			var kubeadmJoinCommand string
			var mastNodeMetadata *node.NodeMetadata

			// Pre-parse metadata to avoid duplicating work and to fail fast
			type nodeInfo struct {
				node api.Node
				meta *node.NodeMetadata
			}
			infos := make([]nodeInfo, 0, len(gpuNodes))
			for _, n := range gpuNodes {
				nodeMetadata, err := node.ParseNodeMetadata(n.Metadata)
				if err != nil {
					logging.Verbose("Failed to parse node metadata for %s: %v", n.Name, err)
					return fmt.Errorf("parse node metadata failed for %s: %w", n.Name, err)
				}
				infos = append(infos, nodeInfo{node: n, meta: nodeMetadata})
			}

			// Install kubeadm on each GPU node concurrently
			errCh := make(chan error, len(infos))
			var wg sync.WaitGroup
			for _, info := range infos {
				info := info
				wg.Add(1)
				go func() {
					defer wg.Done()
					fmt.Printf("Installing kubeadm on node %s (%s)...\n", info.node.Name, info.meta.MgmtIP)
					_, err := bastion.InstallKubeadmViaBastion(bastion.BastionExecConfig{
						BastionUser: "ubuntu",
						BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
						BastionKey:  keyPath,
						TargetUser:  "ubuntu",
						TargetAddr:  info.meta.MgmtIP + ":22",
						TargetPass:  "nvidia",
						Command:     "",
					})
					if err != nil {
						logging.Verbose("Failed to install kubeadm on node %s: %v", info.node.Name, err)
						errCh <- fmt.Errorf("install kubeadm on node %s failed: %w", info.node.Name, err)
						return
					}
					fmt.Printf("✓ Kubeadm installed on node %s successfully.\n", info.node.Name)
				}()
			}

			wg.Wait()
			close(errCh)
			for err := range errCh {
				if err != nil {
					return err
				}
			}

			// Init and join remain sequential to preserve ordering
			for i, info := range infos {
				if i == 0 {
					mastNodeMetadata = info.meta
					fmt.Printf("Initializing Kubeadm on master node %s...\n", info.node.Name)
					_, err := bastion.InitKubeadmViaBastion(bastion.BastionExecConfig{
						BastionUser: "ubuntu",
						BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
						BastionKey:  keyPath,
						TargetUser:  "ubuntu",
						TargetAddr:  info.meta.MgmtIP + ":22",
						TargetPass:  "nvidia",
						Command:     "",
					})
					if err != nil {
						logging.Verbose("Failed to init kubeadm on master node %s: %v", info.node.Name, err)
						return fmt.Errorf("init kubeadm on master node %s failed: %w", info.node.Name, err)
					}

					kubeadmJoinCommand, _, err = bastion.GetKubeadmJoinCommandViaBastion(bastion.BastionExecConfig{
						BastionUser: "ubuntu",
						BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
						BastionKey:  keyPath,
						TargetUser:  "ubuntu",
						TargetAddr:  info.meta.MgmtIP + ":22",
						TargetPass:  "nvidia",
						Command:     "",
					})
					if err != nil {
						logging.Verbose("Failed to get kubeadm join command on master node %s: %v", info.node.Name, err)
						return fmt.Errorf("get kubeadm join command on master node %s failed: %w", info.node.Name, err)
					}
					fmt.Printf("✓ Kubeadm initialized on master node %s\n", info.node.Name)
				} else {
					fmt.Printf("Joining kubeadm on worker node %s...\n", info.node.Name)
					joinCmdFull := fmt.Sprintf("sudo %s", kubeadmJoinCommand)
					_, err := bastion.JoinKubeadmViaBastion(bastion.BastionExecConfig{
						BastionUser: "ubuntu",
						BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
						BastionKey:  keyPath,
						TargetUser:  "ubuntu",
						TargetAddr:  info.meta.MgmtIP + ":22",
						TargetPass:  "nvidia",
						Command:     joinCmdFull,
					})
					if err != nil {
						logging.Verbose("Failed to join kubeadm on worker node %s: %v", info.node.Name, err)
						return fmt.Errorf("join kubeadm on worker node %s failed: %w", info.node.Name, err)
					}
					fmt.Printf("✓ Kubeadm joined on worker node %s\n", info.node.Name)
				}
			}

			fmt.Printf("✓ Kubeadm installation completed.\n")

			// Configure port forward for Kubernetes API server
			if mastNodeMetadata != nil {
				logging.Verbose("Configuring Nginx port forward for Kubernetes API server to %s", mastNodeMetadata.MgmtIP)
				fmt.Printf("Configuring Nginx port forward for Kubernetes API server...\n")

				forwardServers := []bastion.ForwardServer{
					{
						Name:       "k8s-api",
						LocalPort:  6443,
						RemoteIP:   mastNodeMetadata.MgmtIP,
						RemotePort: 6443,
					},
				}

				_, err = bastion.UpdateNginxConfig(context.Background(), bastion.BastionExecConfig{
					BastionUser: "ubuntu",
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionKey:  keyPath,
					TargetUser:  "ubuntu",
					TargetAddr:  sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					TargetPass:  "dangerous",
					Command:     "",
				}, forwardServers)
				if err != nil {
					logging.Verbose("Failed to configure nginx port forward: %v", err)
					return fmt.Errorf("failed to configure nginx port forward: %w", err)
				}

				fmt.Printf("✓ Kubernetes API server port forward configured (6443 -> %s:6443)\n", mastNodeMetadata.MgmtIP)

				// Create Kubernetes API service
				logging.Verbose("Creating Kubernetes API service for the outbound interface")
				k8sResponse, err := apiClient.CreateKubernetesAPIService(simResp.ID, outboundInterfaceID)
				if err != nil {
					logging.Verbose("Failed to create Kubernetes API service: %v", err)
					return fmt.Errorf("failed to create kubernetes api service: %w", err)
				}

				fmt.Printf("✓ Kubernetes API service created successfully.\n")
				fmt.Printf("  Access endpoint: https://%s:%d\n", k8sResponse.Host, k8sResponse.SrcPort)
				// Retrieve and save kubeconfig
				logging.Verbose("Retrieving kubeconfig from master node")
				kubeconfigContent, err := bastion.GetKubeconfigViaBastion(bastion.BastionExecConfig{
					BastionUser: "ubuntu",
					BastionAddr: sshResponse.Host + ":" + strconv.Itoa(sshResponse.SrcPort),
					BastionKey:  keyPath,
					TargetUser:  "ubuntu",
					TargetAddr:  mastNodeMetadata.MgmtIP + ":22",
					TargetPass:  "nvidia",
					Command:     "",
				})
				if err != nil {
					logging.Verbose("Failed to retrieve kubeconfig: %v", err)
					return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
				}

				// debug log kubeconfigContent
				logging.Verbose("Kubeconfig content retrieved. Length: %d characters", len(kubeconfigContent))

				// Parse kubeconfig using clientcmd
				kubeconfig, err := clientcmd.Load([]byte(kubeconfigContent))
				if err != nil {
					logging.Verbose("Failed to parse kubeconfig: %v", err)
					return fmt.Errorf("failed to parse kubeconfig: %w", err)
				}
				logging.Verbose("✓ Kubeconfig parsed using clientcmd")

				// Modify kubeconfig structure
				if len(kubeconfig.Clusters) > 0 {
					// Update server address in the first cluster
					for clusterName := range kubeconfig.Clusters {
						cluster := kubeconfig.Clusters[clusterName]
						cluster.Server = fmt.Sprintf("https://%s:%d", k8sResponse.Host, k8sResponse.SrcPort)
						cluster.CertificateAuthority = ""
						cluster.CertificateAuthorityData = nil
						cluster.InsecureSkipTLSVerify = true
						logging.Verbose("✓ Updated cluster %s: server=%s, insecure-skip-tls-verify=true", clusterName, cluster.Server)
						break // Only update the first cluster
					}
				}

				// Serialize back to YAML using clientcmd
				modifiedKubeconfigBytes, err := clientcmd.Write(*kubeconfig)
				if err != nil {
					logging.Verbose("Failed to serialize kubeconfig: %v", err)
					return fmt.Errorf("failed to serialize kubeconfig: %w", err)
				}
				modifiedKubeconfig := string(modifiedKubeconfigBytes)
				logging.Verbose("✓ Kubeconfig serialized back to YAML")
				logging.Verbose("Modified kubeconfig length: %d", len(modifiedKubeconfig))

				// Save kubeconfig to local file
				homeDir, err := os.UserHomeDir()
				if err != nil {
					logging.Verbose("Failed to get home directory: %v", err)
					return fmt.Errorf("failed to get home directory: %w", err)
				}

				kubeconfigPath := filepath.Join(homeDir, ".kube", "nvair-kubeconfig-simulation")
				kubeconfigDir := filepath.Dir(kubeconfigPath)
				os.MkdirAll(kubeconfigDir, 0755)
				logging.Verbose("Kubeconfig directory created: %s", kubeconfigDir)

				logging.Verbose("Writing kubeconfig to: %s", kubeconfigPath)
				logging.Verbose("Content length: %d bytes", len(modifiedKubeconfig))

				err = os.WriteFile(kubeconfigPath, []byte(modifiedKubeconfig), 0600)
				if err != nil {
					logging.Verbose("Failed to save kubeconfig: %v", err)
					return fmt.Errorf("failed to save kubeconfig: %w", err)
				}

				// Verify file was written
				info, err := os.Stat(kubeconfigPath)
				if err != nil {
					logging.Verbose("Failed to verify kubeconfig file: %v", err)
				} else {
					logging.Verbose("Kubeconfig file verified. Size: %d bytes", info.Size())
				}

				fmt.Printf("\n\nRun kubectl with this kubeconfig:\n")
				fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
				fmt.Printf("  kubectl get pods -A -w\n")
			}
		}
	}

	return nil
}

// WaitForJobs waits for all specified jobs to reach a terminal state (COMPLETE, FAILED, or CANCELLED)
// It polls each job every 2 seconds up to a maximum of 10 minutes
func (cc *CreateCommand) WaitForJobs(apiClient *api.Client, jobIDs []string) error {
	const (
		pollInterval = 2 * time.Second
		maxWaitTime  = 10 * time.Minute
	)

	logging.Verbose("WaitForJobs: Starting to monitor %d jobs", len(jobIDs))
	startTime := time.Now()
	jobStates := make(map[string]string)

	// Initialize all jobs as pending
	for _, jobID := range jobIDs {
		jobStates[jobID] = "PENDING"
	}

	for {
		// Check if we've exceeded the maximum wait time
		if time.Since(startTime) > maxWaitTime {
			logging.Verbose("WaitForJobs: Timeout waiting for jobs after %v", time.Since(startTime))
			incompleteJobs := []string{}
			for id, state := range jobStates {
				if state != "COMPLETE" && state != "FAILED" && state != "CANCELLED" {
					incompleteJobs = append(incompleteJobs, id)
				}
			}
			return fmt.Errorf("timeout waiting for jobs to complete (waited %v). Incomplete jobs: %v", time.Since(startTime), incompleteJobs)
		}

		allComplete := true

		// Check status of each job
		for _, jobID := range jobIDs {
			job, err := apiClient.GetJob(jobID)
			if err != nil {
				logging.Verbose("WaitForJobs: Error fetching job %s: %v", jobID, err)
				allComplete = false
				continue
			}

			jobStates[jobID] = job.State
			logging.Verbose("WaitForJobs: Job %s state: %s", jobID, job.State)

			// Check if job reached a terminal state
			if job.State != "COMPLETE" && job.State != "FAILED" && job.State != "CANCELLED" {
				allComplete = false
			}

			// If a job failed, return error immediately
			if job.State == "FAILED" {
				logging.Verbose("WaitForJobs: Job %s failed", jobID)
				return fmt.Errorf("job %s failed", jobID)
			}

			// If a job was cancelled, return error
			if job.State == "CANCELLED" {
				logging.Verbose("WaitForJobs: Job %s was cancelled", jobID)
				return fmt.Errorf("job %s was cancelled", jobID)
			}
		}

		// If all jobs are complete, exit
		if allComplete {
			logging.Verbose("WaitForJobs: All jobs completed successfully")
			return nil
		}

		// Wait before polling again
		time.Sleep(pollInterval)
	}
}
