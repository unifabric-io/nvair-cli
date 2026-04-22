package delete

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	forwardutil "github.com/unifabric-io/nvair-cli/pkg/forward"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/simulation"
)

// Command represents the delete subcommand.
type Command struct {
	ResourceType   string // "simulation" or "forward"
	ResourceName   string
	SimulationName string
	TargetNode     string
	TargetPort     int
	APIEndpoint    string
	Stderr         io.Writer
	Verbose        bool
}

// NewCommand creates a new delete command instance.
func NewCommand() *Command {
	return &Command{APIEndpoint: api.DefaultBaseURL}
}

// Register registers the delete command flags.
func (dc *Command) Register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&dc.APIEndpoint, "api-endpoint", dc.APIEndpoint, "API endpoint URL")
}

// ValidateArgs validates positional arguments for the delete command.
func ValidateArgs(args []string) error {
	if err := cobra.ExactArgs(2)(nil, args); err != nil {
		return err
	}

	if args[0] != "simulation" {
		return fmt.Errorf("invalid resource type: %s. Must be 'simulation'", args[0])
	}

	if strings.TrimSpace(args[1]) == "" {
		return fmt.Errorf("%s name is required", args[0])
	}

	return nil
}

// Execute runs the delete command.
func (dc *Command) Execute() error {
	if dc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Delete command started")

	if dc.ResourceType == "" {
		return fmt.Errorf("usage: nvair delete <simulation> <name> | nvair delete forward --target-node <name> --target-port <port> [-s <simulation>]")
	}

	switch dc.ResourceType {
	case "simulation":
		if strings.TrimSpace(dc.ResourceName) == "" {
			return fmt.Errorf("usage: nvair delete <simulation> <name>")
		}
	case "forward":
		dc.TargetNode = strings.TrimSpace(dc.TargetNode)
		if dc.TargetNode == "" {
			return output.NewValidationError("--target-node is required")
		}
		if dc.TargetPort <= 0 || dc.TargetPort > 65535 {
			return output.NewValidationError("--target-port must be between 1 and 65535")
		}
	default:
		return fmt.Errorf("invalid resource type: %s. Must be 'simulation' or 'forward'", dc.ResourceType)
	}

	switch dc.ResourceType {
	case "simulation":
		logging.Verbose("Deleting %s: %s", dc.ResourceType, dc.ResourceName)
	case "forward":
		logging.Verbose("Deleting %s target: %s", dc.ResourceType, dc.forwardTarget())
	}

	cfg, err := config.Load()
	if err != nil || cfg.APIToken == "" {
		logging.Verbose("Not authenticated")
		return fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}
	logging.Verbose("Authentication verified")

	apiClient := api.NewClient(dc.APIEndpoint, cfg.APIToken)

	var deleteErr error
	switch dc.ResourceType {
	case "simulation":
		deleteErr = apiClient.DeleteSimulation(dc.ResourceName)
	case "forward":
		deleteErr = dc.deleteForward(apiClient)
	default:
		return fmt.Errorf("invalid resource type: %s. Must be 'simulation' or 'forward'", dc.ResourceType)
	}

	if deleteErr != nil {
		logging.Verbose("API request failed: %v", deleteErr)
		return deleteErr
	}

	logging.Info("✓ %s '%s' deleted successfully.", strings.Title(dc.ResourceType), dc.ResourceName)

	return nil
}

func (dc *Command) deleteForward(apiClient *api.Client) error {
	resolvedSimulation, err := simulation.Resolve(apiClient, dc.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(dc.errWriter(), resolvedSimulation.Name)
	}

	services, err := apiClient.GetServices(resolvedSimulation.ID)
	if err != nil {
		return err
	}

	matches := make([]api.EnableSSHResponse, 0)
	for _, service := range services {
		targetHost, targetPort := resolveForwardTarget(service)
		if targetHost == dc.TargetNode && targetPort == dc.TargetPort {
			matches = append(matches, service)
		}
	}

	if len(matches) == 0 {
		return output.NewNotFoundError(fmt.Sprintf("forward service not found for target %s", dc.forwardTarget()))
	}

	if len(matches) > 1 {
		descriptions := make([]string, 0, len(matches))
		for _, service := range matches {
			descriptions = append(descriptions, fmt.Sprintf("%s (%s)", service.ID, service.Name))
		}
		return output.NewValidationError(
			fmt.Sprintf("multiple forward services found for target %s: %s", dc.forwardTarget(), strings.Join(descriptions, ", ")),
		)
	}

	target := matches[0]
	if err := apiClient.DeleteServiceByID(target.ID); err != nil {
		return err
	}

	dc.ResourceName = target.Name
	return nil
}

func (dc *Command) errWriter() io.Writer {
	if dc.Stderr != nil {
		return dc.Stderr
	}
	return io.Discard
}

func (dc *Command) forwardTarget() string {
	return net.JoinHostPort(dc.TargetNode, strconv.Itoa(dc.TargetPort))
}

func resolveForwardTarget(service api.EnableSSHResponse) (string, int) {
	if parsed, ok := forwardutil.ParseServiceName(service.Name); ok {
		return parsed.TargetHost, parsed.TargetPort
	}
	if forwardutil.IsBastionSSHServiceName(service.Name) {
		return constant.OOBMgmtServerName, 22
	}

	return service.NodeName, service.DestPort
}
