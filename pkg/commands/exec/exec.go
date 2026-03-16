package exec

import (
	"fmt"
	"io"
	"net"
	"os"
	"sort"
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
	execCommandViaBastionFn      = bastion.ExecCommandViaBastion
	execCommandOnBastionFn       = bastion.ExecCommandOnBastion
	interactiveSessionViaBastion = bastion.InteractiveSessionViaBastion
	interactiveSessionOnBastion  = bastion.InteractiveSessionOnBastion
	interactiveCommandViaBastion = bastion.InteractiveCommandViaBastion
	interactiveCommandOnBastion  = bastion.InteractiveCommandOnBastion
	defaultKeyPathFn             = sshpkg.DefaultKeyPath
)

// Command represents the exec subcommand.
type Command struct {
	SimulationName string
	APIEndpoint    string
	Stdin          bool
	TTY            bool
	Stderr         io.Writer
	Verbose        bool
}

type resolvedCredentials struct {
	TargetUser    string
	TargetPass    string
	DirectBastion bool
}

// NewCommand creates a new exec command with defaults.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: "https://air.nvidia.com/api",
	}
}

// Register registers exec flags.
func (ec *Command) Register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVarP(&ec.SimulationName, "simulation", "s", ec.SimulationName, "Simulation name (optional when only one simulation exists)")
	flags.StringVar(&ec.APIEndpoint, "api-endpoint", ec.APIEndpoint, "API endpoint URL")
	flags.BoolVarP(&ec.Stdin, "stdin", "i", ec.Stdin, "Keep stdin open for interactive session")
	flags.BoolVarP(&ec.TTY, "tty", "t", ec.TTY, "Allocate a TTY for interactive session")
}

// Execute runs the exec command.
func (ec *Command) Execute(args []string, dashIndex int) error {
	ec.configureVerbose()

	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return output.NewUsageError("node name is required")
	}

	nodeName := strings.TrimSpace(args[0])
	command, err := parseRemoteCommand(args, dashIndex)
	if err != nil {
		return err
	}
	interactive := ec.Stdin && ec.TTY
	if (ec.Stdin || ec.TTY) && !interactive {
		return output.NewValidationError("interactive mode requires both -i and -t")
	}
	if !interactive && command == "" {
		return output.NewValidationError("non-interactive mode requires a command after -- (use -it for interactive shell)")
	}

	apiClient, _, err := ec.ensureAuthenticatedClient(ec.APIEndpoint)
	if err != nil {
		return err
	}

	resolvedSimulation, err := simulation.Resolve(apiClient, ec.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(ec.errWriter(), resolvedSimulation.Name)
	}
	simulationID := resolvedSimulation.ID

	sshHost, sshPort, err := ec.findSSHService(apiClient, simulationID)
	if err != nil {
		return err
	}

	targetNode, targetMgmtIP, err := ec.findNodeByName(apiClient, simulationID, nodeName)
	if err != nil {
		return err
	}

	credentials := ec.resolveCredentials(*targetNode)

	keyPath, err := defaultKeyPathFn()
	if err != nil {
		return fmt.Errorf("failed to get SSH key path: %w", err)
	}

	cfg := bastion.BastionExecConfig{
		BastionUser: constant.DefaultUbuntuUser,
		BastionAddr: net.JoinHostPort(sshHost, strconv.Itoa(sshPort)),
		BastionKey:  keyPath,
		TargetUser:  credentials.TargetUser,
		TargetAddr:  net.JoinHostPort(targetMgmtIP, "22"),
		TargetPass:  credentials.TargetPass,
		Command:     command,
	}

	if interactive {
		if command != "" {
			if credentials.DirectBastion {
				return interactiveCommandOnBastion(cfg)
			}
			return interactiveCommandViaBastion(cfg)
		}
		if credentials.DirectBastion {
			return interactiveSessionOnBastion(cfg)
		}
		return interactiveSessionViaBastion(cfg)
	}

	var result *bastion.ExecResult
	if credentials.DirectBastion {
		result, err = execCommandOnBastionFn(cfg)
	} else {
		result, err = execCommandViaBastionFn(cfg)
	}
	if err != nil {
		return err
	}

	if result != nil {
		if result.Stdout != "" {
			_, _ = fmt.Fprint(os.Stdout, result.Stdout)
		}
		if result.Stderr != "" {
			_, _ = fmt.Fprint(os.Stderr, result.Stderr)
		}
		if result.ExitCode != 0 {
			return output.NewSilentExitCodeError(result.ExitCode)
		}
	}

	return nil
}

func parseRemoteCommand(args []string, dashIndex int) (string, error) {
	// Interactive mode: only node-name positional arg is provided.
	if len(args) == 1 {
		return "", nil
	}

	if dashIndex < 0 {
		if len(args) > 1 {
			return "", output.NewUsageError("command arguments must be provided after --")
		}
		return "", nil
	}

	if dashIndex >= len(args) {
		return "", nil
	}

	commandArgs := args[dashIndex:]
	quoted := make([]string, 0, len(commandArgs))
	for _, arg := range commandArgs {
		quoted = append(quoted, shellQuote(arg))
	}

	return strings.TrimSpace(strings.Join(quoted, " ")), nil
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'"'"'`) + "'"
}

func (ec *Command) configureVerbose() {
	if ec.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}
}

func (ec *Command) ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil || cfg.BearerToken == "" {
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}

	if cfg.IsTokenExpired(time.Now()) {
		if cfg.APIToken == "" {
			return nil, nil, fmt.Errorf("authentication token has expired and no API token available. Please run 'nvair login' again")
		}

		refreshClient := api.NewClient(apiEndpoint, "")
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

	return api.NewClient(apiEndpoint, cfg.BearerToken), cfg, nil
}

func (ec *Command) errWriter() io.Writer {
	if ec.Stderr != nil {
		return ec.Stderr
	}
	return io.Discard
}

func (ec *Command) findSSHService(apiClient *api.Client, simulationID string) (string, int, error) {
	services, err := apiClient.GetServices(simulationID)
	if err != nil {
		return "", 0, err
	}

	for _, service := range services {
		if strings.EqualFold(service.ServiceType, "ssh") &&
			forwardutil.IsBastionSSHServiceName(service.Name) &&
			service.Host != "" &&
			service.SrcPort > 0 {
			return service.Host, service.SrcPort, nil
		}
	}

	return "", 0, fmt.Errorf("SSH service not found, run nvair create first")
}

func (ec *Command) findNodeByName(apiClient *api.Client, simulationID, nodeName string) (*api.Node, string, error) {
	nodes, err := apiClient.GetNodes(simulationID)
	if err != nil {
		return nil, "", err
	}

	images, err := apiClient.GetImages()
	if err != nil {
		return nil, "", err
	}
	imageNames := make(map[string]string, len(images))
	for _, image := range images {
		imageNames[image.ID] = image.Name
	}

	availableNodeNames := make([]string, 0, len(nodes))
	for _, n := range nodes {
		availableNodeNames = append(availableNodeNames, n.Name)
	}
	sort.Strings(availableNodeNames)

	for _, n := range nodes {
		if n.Name != nodeName {
			continue
		}

		metadata, err := node.ParseNodeMetadata(n.Metadata)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse node metadata for %s: %w", n.Name, err)
		}

		mgmtIP := strings.TrimSpace(metadata.MgmtIP)
		if mgmtIP == "" {
			return nil, "", fmt.Errorf("node %s metadata does not contain mgmt_ip", n.Name)
		}

		if resolvedImageName, ok := imageNames[n.OS]; ok {
			n.OSName = resolvedImageName
		} else {
			n.OSName = n.OS
		}

		return &n, mgmtIP, nil
	}

	return nil, "", output.NewNotFoundError(
		fmt.Sprintf("node not found: %s (available: %s)", nodeName, strings.Join(availableNodeNames, ", ")),
	)
}

func (ec *Command) resolveCredentials(targetNode api.Node) resolvedCredentials {
	if targetNode.Name == constant.OOBMgmtServerName {
		return resolvedCredentials{
			TargetUser:    constant.DefaultUbuntuUser,
			DirectBastion: true,
		}
	}

	imageName := strings.ToLower(strings.TrimSpace(targetNode.OSName))
	if imageName == "" {
		imageName = strings.ToLower(strings.TrimSpace(targetNode.OS))
	}
	if strings.Contains(imageName, "cumulus") {
		return resolvedCredentials{
			TargetUser: constant.DefaultCumulusUser,
			TargetPass: constant.DefaultCumulusNewPassword,
		}
	}

	return resolvedCredentials{
		TargetUser: constant.DefaultUbuntuUser,
		TargetPass: constant.DefaultUbuntuPassword,
	}
}
