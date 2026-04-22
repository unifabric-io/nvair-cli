package add

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

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
	TargetPort     int
	TargetNode     string
	Stderr         io.Writer
	Verbose        bool
}

// NewCommand creates a new add command with defaults.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: api.DefaultBaseURL,
	}
}

// Register registers add subcommands and flags.
func (ac *Command) Register(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&ac.APIEndpoint, "api-endpoint", ac.APIEndpoint, "API endpoint URL")

	forwardCmd := &cobra.Command{
		Use:   "forward",
		Short: "Add a forward service in a simulation",
		RunE: func(cmd *cobra.Command, args []string) error {
			ac.Stderr = cmd.ErrOrStderr()
			ac.SimulationName = strings.TrimSpace(ac.SimulationName)
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
	plan, err := ac.planForward(services)
	if err != nil {
		return err
	}
	if plan.Existing != nil {
		address := resolveForwardAddress(*plan.Existing)
		logging.Info("✓ Forward already exists.")
		logging.Info("  %s -> %s", address, net.JoinHostPort(ac.TargetNode, strconv.Itoa(ac.TargetPort)))
		if !plan.RequiresIPTables {
			return nil
		}
		logging.Info("  configuring iptables NAT")
		if err := ac.setupIPTables(apiClient, simulationID, ac.TargetNode, plan.ListenPort, ac.TargetPort); err != nil {
			return fmt.Errorf("iptables setup failed: %w", err)
		}
		return nil
	}

	outboundInterfaceID, err := ac.findOutboundInterfaceID(apiClient, simulationID)
	if err != nil {
		return err
	}

	serviceName := forwardutil.BuildServiceName(plan.ListenPort, ac.TargetPort, ac.TargetNode)
	serviceResp, err := apiClient.CreateService(simulationID, outboundInterfaceID, serviceName, plan.ListenPort, plan.ServiceType)
	if err != nil {
		return fmt.Errorf("failed to create forward service: %w", err)
	}

	address := resolveForwardAddress(*serviceResp)
	logging.Info("✓ Forward service created successfully.")
	logging.Info("  %s -> %s", address, net.JoinHostPort(ac.TargetNode, strconv.Itoa(ac.TargetPort)))

	if !plan.RequiresIPTables {
		return nil
	}
	if err := ac.setupIPTables(apiClient, simulationID, ac.TargetNode, plan.ListenPort, ac.TargetPort); err != nil {
		return fmt.Errorf("forward created but iptables setup failed: %w", err)
	}
	return nil
}

type forwardPlan struct {
	Existing         *api.EnableSSHResponse
	ListenPort       int
	ServiceType      string
	RequiresIPTables bool
}

func (ac *Command) planForward(services []api.EnableSSHResponse) (forwardPlan, error) {
	if ac.isDirectBastionForward() {
		for i := range services {
			svc := &services[i]
			if svc.DestPort != 22 {
				continue
			}
			if forwardutil.IsBastionSSHServiceName(svc.Name) && strings.EqualFold(svc.ServiceType, "ssh") {
				return forwardPlan{
					Existing:         svc,
					ListenPort:       22,
					ServiceType:      "ssh",
					RequiresIPTables: false,
				}, nil
			}

			return forwardPlan{}, output.NewValidationError(
				fmt.Sprintf("listen port 22 is already used by service %q. Delete that forward first using its target node and target port",
					svc.Name),
			)
		}

		return forwardPlan{
			ListenPort:       22,
			ServiceType:      "ssh",
			RequiresIPTables: false,
		}, nil
	}

	for i := range services {
		svc := &services[i]
		parsed, ok := forwardutil.ParseServiceName(svc.Name)
		if !ok {
			continue
		}
		if parsed.TargetHost != ac.TargetNode || parsed.TargetPort != ac.TargetPort {
			continue
		}

		serviceType := strings.TrimSpace(strings.ToLower(svc.ServiceType))
		if serviceType == "" {
			serviceType = "other"
		}

		return forwardPlan{
			Existing:         svc,
			ListenPort:       svc.DestPort,
			ServiceType:      serviceType,
			RequiresIPTables: true,
		}, nil
	}

	listenPort, err := nextAvailableListenPort(services, autoForwardListenPortStart)
	if err != nil {
		return forwardPlan{}, err
	}

	return forwardPlan{
		ListenPort:       listenPort,
		ServiceType:      "other",
		RequiresIPTables: true,
	}, nil
}

func (ac *Command) isDirectBastionForward() bool {
	return ac.TargetNode == constant.OOBMgmtServerName && ac.TargetPort == 22
}

func nextAvailableListenPort(services []api.EnableSSHResponse, start int) (int, error) {
	usedPorts := make(map[int]struct{}, len(services))
	for _, svc := range services {
		if svc.DestPort > 0 {
			usedPorts[svc.DestPort] = struct{}{}
		}
	}

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
	if err != nil || cfg.APIToken == "" {
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}

	return api.NewClient(apiEndpoint, cfg.APIToken), cfg, nil
}

// setupIPTables SSHes into oob-mgmt-server and configures DNAT + MASQUERADE rules
// so that traffic arriving on listenPort is forwarded to targetNode:targetPort.
func (ac *Command) setupIPTables(apiClient *api.Client, simulationID, targetNode string, listenPort, targetPort int) error {
	// Resolve target host to an IP address.
	dstIP, err := ac.resolveDestinationIP(apiClient, simulationID, targetNode)
	if err != nil {
		return err
	}

	// Find the SSH bastion service address.
	sshHost, sshPort, err := ac.findSSHService(apiClient, simulationID)
	if err != nil {
		return fmt.Errorf("could not locate SSH service for bastion: %w", err)
	}

	keyPath, err := defaultKeyPathFn()
	if err != nil {
		return fmt.Errorf("failed to get SSH key path: %w", err)
	}

	dst := net.JoinHostPort(dstIP, strconv.Itoa(targetPort))
	script := fmt.Sprintf(`set -euo pipefail
sudo sysctl -w net.ipv4.ip_forward=1
if ! sudo iptables -t nat -C PREROUTING -p tcp --dport %d -j DNAT --to-destination '%s' >/dev/null 2>&1; then
  sudo iptables -t nat -A PREROUTING -p tcp --dport %d -j DNAT --to-destination '%s'
fi
if ! sudo iptables -t nat -C OUTPUT -p tcp -m addrtype --dst-type LOCAL --dport %d -j DNAT --to-destination '%s' >/dev/null 2>&1; then
  sudo iptables -t nat -A OUTPUT -p tcp -m addrtype --dst-type LOCAL --dport %d -j DNAT --to-destination '%s'
fi
if ! sudo iptables -t nat -C POSTROUTING -p tcp -d '%s' --dport %d -j MASQUERADE >/dev/null 2>&1; then
  sudo iptables -t nat -A POSTROUTING -p tcp -d '%s' --dport %d -j MASQUERADE
fi`,
		listenPort, dst,
		listenPort, dst,
		listenPort, dst,
		listenPort, dst,
		dstIP, targetPort,
		dstIP, targetPort,
	)

	cfg := bastion.BastionExecConfig{
		BastionUser: constant.DefaultUbuntuUser,
		BastionAddr: net.JoinHostPort(sshHost, strconv.Itoa(sshPort)),
		BastionKey:  keyPath,
		TargetUser:  constant.DefaultUbuntuUser,
		Command:     script,
	}

	result, err := execCommandOnBastionFn(cfg)
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
		if strings.EqualFold(svc.ServiceType, "ssh") &&
			forwardutil.IsBastionSSHServiceName(svc.Name) &&
			svc.Host != "" &&
			svc.SrcPort > 0 {
			return svc.Host, svc.SrcPort, nil
		}
	}
	return "", 0, fmt.Errorf("SSH service not found; run 'nvair create' first")
}
