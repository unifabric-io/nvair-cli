package add

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	forwardutil "github.com/unifabric-io/nvair-cli/pkg/forward"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/simulation"
	sshpkg "github.com/unifabric-io/nvair-cli/pkg/ssh"
)

var (
	defaultKeyPathFn       = sshpkg.DefaultKeyPath
	execCommandOnBastionFn = bastion.ExecCommandOnBastion
)

const autoForwardListenPortStart = 20000

// Command represents the add subcommand.
type Command struct {
	APIEndpoint    string
	SimulationName string
	ForwardName    string
	TargetPort     int
	TargetNode     string
	Stderr         io.Writer
	Verbose        bool
}

// NewCommand creates a new add command with defaults.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: constant.DefaultAPIEndpoint,
	}
}

// Register registers add subcommands and flags.
func (ac *Command) Register(cmd *cobra.Command) {
	forwardCmd := &cobra.Command{
		Use:     "forward <forward-name>",
		Aliases: []string{"forwards"},
		Short:   "Add a forward service in a simulation",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ac.Stderr = cmd.ErrOrStderr()
			ac.SimulationName = strings.TrimSpace(ac.SimulationName)
			ac.ForwardName = strings.TrimSpace(args[0])
			ac.TargetNode = strings.TrimSpace(ac.TargetNode)
			return ac.executeForward()
		},
	}
	forwardCmd.Flags().StringVarP(&ac.SimulationName, "simulation", "s", "", "Simulation name (optional when only one simulation exists)")
	forwardCmd.Flags().IntVar(&ac.TargetPort, "target-port", 0, "Target port on target node")
	forwardCmd.Flags().StringVar(&ac.TargetNode, "target-node", "", "Target node name")

	cmd.AddCommand(forwardCmd)
}

func (ac *Command) executeForward() error {
	ac.configureVerbose()

	ac.ForwardName = strings.TrimSpace(ac.ForwardName)
	if ac.ForwardName == "" {
		return output.NewValidationError("forward name is required")
	}
	if ac.TargetPort <= 0 || ac.TargetPort > 65535 {
		return output.NewValidationError("--target-port must be between 1 and 65535")
	}
	if ac.TargetNode == "" {
		return output.NewValidationError("--target-node is required")
	}

	apiClient, _, err := ensureAuthenticatedClient(ac.APIEndpoint)
	if err != nil {
		return err
	}

	resolvedSimulation, err := simulation.Resolve(apiClient, ac.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(ac.errWriter(), resolvedSimulation.Name)
	}
	simulationID := resolvedSimulation.ID

	services, err := apiClient.GetServices(simulationID)
	if err != nil {
		return err
	}
	plan, err := ac.planForward(apiClient, simulationID, services)
	if err != nil {
		return err
	}
	if plan.Existing != nil {
		return ac.existingForwardNameError(apiClient, simulationID, *plan.Existing)
	}

	outboundInterfaceID, err := ac.findOutboundInterfaceID(apiClient, simulationID)
	if err != nil {
		return err
	}

	serviceResp, err := apiClient.CreateService(simulationID, outboundInterfaceID, ac.ForwardName, plan.ListenPort, plan.ServiceType)
	if err != nil {
		return fmt.Errorf("failed to create forward service: %w", err)
	}

	address := resolveForwardAddress(*serviceResp)
	logging.Info("✓ Forward service created successfully.")
	logging.Info("  %s -> %s", address, net.JoinHostPort(ac.TargetNode, strconv.Itoa(ac.TargetPort)))

	if err := ac.setupIPTables(apiClient, simulationID, ac.TargetNode, plan.ListenPort, ac.TargetPort); err != nil {
		return fmt.Errorf("forward created but iptables setup failed: %w", err)
	}
	return nil
}

type forwardPlan struct {
	Existing    *api.EnableSSHResponse
	ListenPort  int
	ServiceType string
}

func (ac *Command) planForward(apiClient *api.Client, simulationID string, services []api.EnableSSHResponse) (forwardPlan, error) {
	for i := range services {
		svc := &services[i]
		if svc.Name != ac.ForwardName {
			continue
		}

		serviceType := strings.TrimSpace(strings.ToLower(svc.ServiceType))
		if serviceType == "" {
			serviceType = "other"
		}

		return forwardPlan{
			Existing:    svc,
			ListenPort:  svc.DestPort,
			ServiceType: serviceType,
		}, nil
	}

	usedPorts, err := ac.usedForwardListenPorts(apiClient, simulationID)
	if err != nil {
		return forwardPlan{}, err
	}

	listenPort, err := nextAvailableListenPort(usedPorts, autoForwardListenPortStart)
	if err != nil {
		return forwardPlan{}, err
	}

	return forwardPlan{
		ListenPort:  listenPort,
		ServiceType: "other",
	}, nil
}

func (ac *Command) usedForwardListenPorts(apiClient *api.Client, simulationID string) (map[int]struct{}, error) {
	usedPorts := make(map[int]struct{})
	result, err := ac.runCommandOnBastion(apiClient, simulationID, "sudo iptables-save -t nat 2>/dev/null || sudo iptables -t nat -S")
	if err != nil {
		return nil, fmt.Errorf("failed to inspect iptables rules: %w", err)
	}
	if result != nil && result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to inspect iptables rules: exit code %d: %s", result.ExitCode, result.Stderr)
	}
	if result == nil {
		return usedPorts, nil
	}

	for port := range forwardutil.ParseCommentPorts(result.Stdout) {
		usedPorts[port] = struct{}{}
	}

	return usedPorts, nil
}

func (ac *Command) inspectForwardTargets(apiClient *api.Client, simulationID string) (map[int]forwardutil.IPTablesTarget, error) {
	result, err := ac.runCommandOnBastion(apiClient, simulationID, "sudo iptables-save -t nat 2>/dev/null || sudo iptables -t nat -S")
	if err != nil {
		return nil, fmt.Errorf("failed to inspect iptables rules: %w", err)
	}
	if result != nil && result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to inspect iptables rules: exit code %d: %s", result.ExitCode, result.Stderr)
	}

	if result == nil {
		return map[int]forwardutil.IPTablesTarget{}, nil
	}
	return forwardutil.ParseIPTablesTargets(result.Stdout), nil
}

func (ac *Command) existingForwardNameError(apiClient *api.Client, simulationID string, existing api.EnableSSHResponse) error {
	message := fmt.Sprintf("Forward name %q already used", ac.ForwardName)

	targets, err := ac.inspectForwardTargets(apiClient, simulationID)
	if err != nil {
		logging.Verbose("Skipping existing forward target inspection: %v", err)
	} else if current, ok := targets[existing.DestPort]; ok {
		currentTarget := ac.formatExistingForwardTarget(apiClient, simulationID, current)
		message = fmt.Sprintf("%s for %s", message, currentTarget)
	}

	return output.NewValidationError(
		fmt.Sprintf("%s. Delete it or use a different name.", message),
	)
}

func sameForwardHost(a, b string) bool {
	ipA := net.ParseIP(a)
	ipB := net.ParseIP(b)
	if ipA != nil && ipB != nil {
		return ipA.Equal(ipB)
	}
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (ac *Command) formatExistingForwardTarget(apiClient *api.Client, simulationID string, target forwardutil.IPTablesTarget) string {
	host := target.Host
	nodes, err := apiClient.GetNodes(simulationID)
	if err == nil {
		for _, n := range nodes {
			metadata, err := node.ParseNodeMetadata(n.Metadata)
			if err != nil {
				continue
			}
			if sameForwardHost(metadata.MgmtIP, target.Host) {
				host = n.Name
				break
			}
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(target.Port))
}

func nextAvailableListenPort(usedPorts map[int]struct{}, start int) (int, error) {
	for port := start; port <= 65535; port++ {
		if _, exists := usedPorts[port]; !exists {
			return port, nil
		}
	}

	return 0, output.NewValidationError(
		fmt.Sprintf("no available forward listen ports remain in the %d-65535 range", start),
	)
}

func (ac *Command) configureVerbose() {
	if ac.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}
}

func (ac *Command) errWriter() io.Writer {
	if ac.Stderr != nil {
		return ac.Stderr
	}
	return io.Discard
}

func (ac *Command) findOutboundInterfaceID(apiClient *api.Client, simulationID string) (string, error) {
	nodes, err := apiClient.GetNodes(simulationID)
	if err != nil {
		return "", err
	}

	oobMgmtServerID := ""
	for _, n := range nodes {
		if n.Name == constant.OOBMgmtServerName {
			oobMgmtServerID = n.ID
			break
		}
	}
	if oobMgmtServerID == "" {
		return "", fmt.Errorf("oob-mgmt-server node not found in simulation")
	}

	interfaces, err := apiClient.GetNodeInterfaces(simulationID, oobMgmtServerID)
	if err != nil {
		return "", err
	}
	for _, intf := range interfaces {
		if intf.Outbound {
			return intf.ID, nil
		}
	}

	return "", fmt.Errorf("no outbound interface found on oob-mgmt-server node")
}

func (ac *Command) runCommandOnBastion(apiClient *api.Client, simulationID, command string) (*bastion.ExecResult, error) {
	sshHost, sshPort, err := ac.findSSHService(apiClient, simulationID)
	if err != nil {
		return nil, fmt.Errorf("could not locate SSH service for bastion: %w", err)
	}

	keyPath, err := defaultKeyPathFn()
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH key path: %w", err)
	}

	cfg := bastion.BastionExecConfig{
		BastionUser: constant.DefaultUbuntuUser,
		BastionAddr: net.JoinHostPort(sshHost, strconv.Itoa(sshPort)),
		BastionKey:  keyPath,
		TargetUser:  constant.DefaultUbuntuUser,
		Command:     command,
	}

	return execCommandOnBastionFn(cfg)
}

func resolveForwardAddress(service api.EnableSSHResponse) string {
	if link := strings.TrimSpace(service.Link); link != "" {
		return link
	}

	host := strings.TrimSpace(service.Host)
	switch {
	case host != "" && service.SrcPort > 0:
		return net.JoinHostPort(host, strconv.Itoa(service.SrcPort))
	case host != "":
		return host
	case service.SrcPort > 0:
		return strconv.Itoa(service.SrcPort)
	default:
		return "-"
	}
}

func ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil || cfg.BearerToken == "" {
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}

	endpoint := config.ResolveAPIEndpoint(cfg, apiEndpoint)

	if cfg.IsTokenExpired(time.Now()) {
		if cfg.APIToken == "" {
			return nil, nil, fmt.Errorf("authentication token has expired and no API token available. Please run 'nvair login' again")
		}

		refreshClient := api.NewClient(endpoint, "")
		newBearerToken, expiresAt, err := refreshClient.AuthLogin(cfg.Username, cfg.APIToken)
		if err != nil {
			return nil, nil, fmt.Errorf("authentication token expired and refresh failed: %w", err)
		}

		cfg.BearerToken = newBearerToken
		cfg.BearerTokenExpiresAt = expiresAt
		if err := cfg.Save(); err != nil {
			logging.Verbose("Warning: Failed to save refreshed token: %v", err)
			return nil, nil, fmt.Errorf("authentication token refreshed but failed to persist new token: %w", err)
		}
	}

	return api.NewClient(endpoint, cfg.BearerToken), cfg, nil
}

// setupIPTables SSHes into oob-mgmt-server and configures DNAT + MASQUERADE rules
// so that traffic arriving on listenPort is forwarded to targetNode:targetPort.
func (ac *Command) setupIPTables(apiClient *api.Client, simulationID, targetNode string, listenPort, targetPort int) error {
	// Resolve target host to an IP address.
	dstIP, err := ac.resolveDestinationIP(apiClient, simulationID, targetNode)
	if err != nil {
		return err
	}

	dst := net.JoinHostPort(dstIP, strconv.Itoa(targetPort))
	comment := forwardutil.PortComment(listenPort)
	script := fmt.Sprintf(`set -euo pipefail
sudo sysctl -w net.ipv4.ip_forward=1
if ! sudo iptables -t nat -C PREROUTING -p tcp --dport %d -m comment --comment '%s' -j DNAT --to-destination '%s' >/dev/null 2>&1; then
  sudo iptables -t nat -A PREROUTING -p tcp --dport %d -m comment --comment '%s' -j DNAT --to-destination '%s'
fi
if ! sudo iptables -t nat -C OUTPUT -p tcp -m addrtype --dst-type LOCAL --dport %d -m comment --comment '%s' -j DNAT --to-destination '%s' >/dev/null 2>&1; then
  sudo iptables -t nat -A OUTPUT -p tcp -m addrtype --dst-type LOCAL --dport %d -m comment --comment '%s' -j DNAT --to-destination '%s'
fi
if ! sudo iptables -t nat -C POSTROUTING -p tcp -d '%s' --dport %d -m comment --comment '%s' -j MASQUERADE >/dev/null 2>&1; then
  sudo iptables -t nat -A POSTROUTING -p tcp -d '%s' --dport %d -m comment --comment '%s' -j MASQUERADE
fi`,
		listenPort, comment, dst,
		listenPort, comment, dst,
		listenPort, comment, dst,
		listenPort, comment, dst,
		dstIP, targetPort, comment,
		dstIP, targetPort, comment,
	)

	result, err := ac.runCommandOnBastion(apiClient, simulationID, script)
	if err != nil {
		return fmt.Errorf("iptables setup failed: %w", err)
	}
	if result != nil && result.ExitCode != 0 {
		return fmt.Errorf("iptables setup exited with code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

// resolveDestinationIP returns the IP for the given destination (node name or raw IP).
func (ac *Command) resolveDestinationIP(apiClient *api.Client, simulationID, destination string) (string, error) {
	// If it's already a valid IP, use it directly.
	if ip := net.ParseIP(destination); ip != nil {
		return destination, nil
	}

	// Otherwise treat it as a node name and look up its mgmt_ip.
	nodes, err := apiClient.GetNodes(simulationID)
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}
	for _, n := range nodes {
		if n.Name != destination {
			continue
		}
		metadata, err := node.ParseNodeMetadata(n.Metadata)
		if err != nil {
			return "", fmt.Errorf("failed to parse metadata for node %s: %w", n.Name, err)
		}
		if metadata.MgmtIP == "" {
			return "", fmt.Errorf("node %s has no mgmt_ip in metadata", n.Name)
		}
		return strings.TrimSpace(metadata.MgmtIP), nil
	}
	return "", fmt.Errorf("destination %q is not a valid IP and no node with that name was found", destination)
}

// findSSHService returns the bastion SSH host and port for the simulation.
func (ac *Command) findSSHService(apiClient *api.Client, simulationID string) (string, int, error) {
	services, err := apiClient.GetServices(simulationID)
	if err != nil {
		return "", 0, err
	}
	for _, svc := range services {
		if svc.NodeName == constant.OOBMgmtServerName &&
			strings.EqualFold(svc.ServiceType, "ssh") &&
			svc.Host != "" &&
			svc.SrcPort > 0 {
			return svc.Host, svc.SrcPort, nil
		}
	}
	return "", 0, fmt.Errorf("SSH service for %s not found; run 'nvair create' first", constant.OOBMgmtServerName)
}
