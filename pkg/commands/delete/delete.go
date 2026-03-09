package delete

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

// Command represents the delete subcommand.
type Command struct {
	ResourceType string // "simulation"
	ResourceName string
	APIEndpoint  string
	Verbose      bool
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

	if dc.ResourceType == "" || dc.ResourceName == "" {
		return fmt.Errorf("usage: nvair delete <simulation> <name>")
	}

	logging.Verbose("Deleting %s: %s", dc.ResourceType, dc.ResourceName)

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
	default:
		return fmt.Errorf("invalid resource type: %s. Must be 'simulation'", dc.ResourceType)
	}

	if deleteErr != nil {
		logging.Verbose("API request failed: %v", deleteErr)
		return deleteErr
	}

	logging.Info("✓ %s '%s' deleted successfully.", strings.Title(dc.ResourceType), dc.ResourceName)

	return nil
}
