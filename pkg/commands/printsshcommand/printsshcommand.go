package printsshcommand

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	forwardutil "github.com/unifabric-io/nvair-cli/pkg/forward"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/simulation"
	sshpkg "github.com/unifabric-io/nvair-cli/pkg/ssh"
)

// Command prints the bastion SSH command for a simulation.
type Command struct {
	SimulationName string
	APIEndpoint    string
	Verbose        bool
}

// NewCommand creates a new print-ssh-command command.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: "https://air.nvidia.com/api",
	}
}

// Register registers print-ssh-command flags.
func (pc *Command) Register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVarP(&pc.SimulationName, "simulation", "s", pc.SimulationName, "Simulation name (optional when only one simulation exists)")
	flags.StringVar(&pc.APIEndpoint, "api-endpoint", pc.APIEndpoint, "API endpoint URL")
}

// Execute runs the print-ssh-command command.
func (pc *Command) Execute(cmd *cobra.Command) error {
	pc.configureVerbose()

	apiClient, _, err := ensureAuthenticatedClient(pc.APIEndpoint)
	if err != nil {
		return err
	}

	resolvedSimulation, err := simulation.Resolve(apiClient, pc.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(cmd.ErrOrStderr(), resolvedSimulation.Name)
	}

	sshHost, sshPort, err := pc.findSSHService(apiClient, resolvedSimulation.ID)
	if err != nil {
		return err
	}

	keyPath, err := sshpkg.DefaultKeyPath()
	if err != nil {
		return fmt.Errorf("failed to get SSH key path: %w", err)
	}

	sshCommand := buildSSHCommand(sshHost, sshPort, keyPath)
	_, err = fmt.Fprintln(cmd.OutOrStdout(), sshCommand)
	return err
}

func (pc *Command) configureVerbose() {
	if pc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}
}

func ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
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

func (pc *Command) findSSHService(apiClient *api.Client, simulationID string) (string, int, error) {
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

func buildSSHCommand(host string, port int, keyPath string) string {
	return fmt.Sprintf("ssh -i %s -p %d %s@%s", shellQuote(keyPath), port, constant.DefaultUbuntuUser, host)
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'"'"'`) + "'"
}
