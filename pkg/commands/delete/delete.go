package delete

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
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/simulation"
	sshpkg "github.com/unifabric-io/nvair-cli/pkg/ssh"
)

var (
	defaultKeyPathFn       = sshpkg.DefaultKeyPath
	execCommandOnBastionFn = bastion.ExecCommandOnBastion
)

const managedForwardListenPortStart = 20000

// Command represents the delete subcommand.
type Command struct {
	ResourceType   string // "simulation" or "forward"
	ResourceName   string
	SimulationName string
	APIEndpoint    string
	Stderr         io.Writer
	Verbose        bool
}

// NewCommand creates a new delete command instance.
func NewCommand() *Command {
	return &Command{APIEndpoint: "https://air.nvidia.com/api"}
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
		return fmt.Errorf("usage: nvair delete <simulation> <name> | nvair delete forward <forward-name> [-s <simulation>]")
	}

	switch dc.ResourceType {
	case "simulation":
		if strings.TrimSpace(dc.ResourceName) == "" {
			return fmt.Errorf("usage: nvair delete <simulation> <name>")
		}
	case "forward":
		dc.ResourceName = strings.TrimSpace(dc.ResourceName)
		if dc.ResourceName == "" {
			return output.NewValidationError("forward name is required")
		}
	default:
		return fmt.Errorf("invalid resource type: %s. Must be 'simulation' or 'forward'", dc.ResourceType)
	}

	switch dc.ResourceType {
	case "simulation":
		logging.Verbose("Deleting %s: %s", dc.ResourceType, dc.ResourceName)
	case "forward":
		logging.Verbose("Deleting %s: %s", dc.ResourceType, dc.ResourceName)
	}

	cfg, err := config.Load()
	if err != nil || cfg.BearerToken == "" {
		logging.Verbose("Not authenticated")
		return fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}
	logging.Verbose("Authentication verified")

	logging.Verbose("Checking token expiration")
	if cfg.IsTokenExpired(time.Now()) {
		logging.Verbose("Bearer token has expired, attempting to refresh with saved API token")

		if cfg.APIToken == "" {
			logging.Verbose("No saved API token available for refresh")
			return fmt.Errorf("authentication token has expired and no API token available. Please run 'nvair login' again")
		}

		apiClient := api.NewClient(dc.APIEndpoint, "")
		newBearerToken, expiresAt, err := apiClient.AuthLogin(cfg.Username, cfg.APIToken)
		if err != nil {
			logging.Verbose("Failed to refresh token: %v", err)
			return fmt.Errorf("authentication token expired and refresh failed: %w", err)
		}

		logging.Verbose("Successfully refreshed bearer token")
		cfg.BearerToken = newBearerToken
		cfg.BearerTokenExpiresAt = expiresAt

		if err := cfg.Save(); err != nil {
			logging.Verbose("Warning: Failed to save refreshed token: %v", err)
		}
	}

	apiClient := api.NewClient(dc.APIEndpoint, cfg.BearerToken)

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

	matches := make([]api.EnableSSHResponse, 0, 1)
	for _, service := range services {
		if service.Name == dc.ResourceName {
			matches = append(matches, service)
		}
	}

	if len(matches) == 0 {
		return output.NewNotFoundError(fmt.Sprintf("forward service %q not found", dc.ResourceName))
	}

	if len(matches) > 1 {
		descriptions := make([]string, 0, len(matches))
		for _, service := range matches {
			descriptions = append(descriptions, fmt.Sprintf("%s (%s)", service.ID, service.Name))
		}
		return output.NewValidationError(
			fmt.Sprintf("multiple forward services found for name %q: %s", dc.ResourceName, strings.Join(descriptions, ", ")),
		)
	}

	target := matches[0]
	if err := dc.cleanupIPTables(services, target.DestPort); err != nil {
		return err
	}
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

func (dc *Command) cleanupIPTables(services []api.EnableSSHResponse, listenPort int) error {
	if listenPort < managedForwardListenPortStart {
		return nil
	}

	result, err := dc.runCommandOnBastion(services, cleanupIPTablesScript(listenPort))
	if err != nil {
		return fmt.Errorf("failed to clean iptables rules for forward %q: %w", dc.ResourceName, err)
	}
	if result != nil && result.ExitCode != 0 {
		return fmt.Errorf("failed to clean iptables rules for forward %q: exit code %d: %s", dc.ResourceName, result.ExitCode, result.Stderr)
	}
	return nil
}

func cleanupIPTablesScript(listenPort int) string {
	comment := forwardutil.PortComment(listenPort)
	return fmt.Sprintf(`set -euo pipefail
comment='%s'
delete_chain_rules() {
  chain="$1"
  sudo iptables -t nat -S "$chain" 2>/dev/null |
    grep -F -- "$comment" |
    sed -n "s/^-A $chain /-D $chain /p" |
    while IFS= read -r delete_rule; do
      [ -n "$delete_rule" ] && printf '%%s\n' "$delete_rule" | xargs -r sudo iptables -t nat || true
    done || true
}
delete_chain_rules PREROUTING
delete_chain_rules OUTPUT
delete_chain_rules POSTROUTING`, comment)
}

func (dc *Command) runCommandOnBastion(services []api.EnableSSHResponse, command string) (*bastion.ExecResult, error) {
	sshHost, sshPort, err := findSSHService(services)
	if err != nil {
		return nil, err
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

func findSSHService(services []api.EnableSSHResponse) (string, int, error) {
	for _, svc := range services {
		if strings.EqualFold(svc.ServiceType, "ssh") && svc.Host != "" && svc.SrcPort > 0 {
			return svc.Host, svc.SrcPort, nil
		}
	}
	return "", 0, fmt.Errorf("SSH service not found; run 'nvair create' first")
}
