package commands

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

// DeleteCommand represents the delete subcommand
type DeleteCommand struct {
	ResourceType string // "simulation" or "service"
	ResourceName string
	APIEndpoint  string
	Verbose      bool
}

// NewDeleteCommand creates a new DeleteCommand instance
func NewDeleteCommand() *DeleteCommand {
	return &DeleteCommand{
		APIEndpoint: "https://air.nvidia.com/api",
	}
}

// Register registers the delete command flags
func (dc *DeleteCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&dc.APIEndpoint, "api-endpoint", "https://air.nvidia.com/api", "API endpoint URL")
	fs.BoolVar(&dc.Verbose, "v", false, "Enable verbose output")
	fs.BoolVar(&dc.Verbose, "verbose", false, "Enable verbose output")
}

// Execute runs the delete command with positional arguments
// Expected: delete <simulation|service> <name>
func (dc *DeleteCommand) Execute(args []string) error {
	// Enable verbose logging if requested
	if dc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Delete command started")

	if len(args) < 2 {
		return fmt.Errorf("usage: nvcli delete <simulation|service> <name>")
	}

	dc.ResourceType = args[0]
	dc.ResourceName = args[1]

	if dc.ResourceType != "simulation" && dc.ResourceType != "service" {
		return fmt.Errorf("invalid resource type: %s. Must be 'simulation' or 'service'", dc.ResourceType)
	}

	if dc.ResourceName == "" {
		return fmt.Errorf("%s name is required", dc.ResourceType)
	}

	logging.Verbose("Deleting %s: %s", dc.ResourceType, dc.ResourceName)

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
		apiClient := api.NewClient(dc.APIEndpoint, "")
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

	// Delete resource via API
	logging.Verbose("Submitting delete request to API")
	apiClient := api.NewClient(dc.APIEndpoint, cfg.BearerToken)

	var deleteErr error
	switch dc.ResourceType {
	case "simulation":
		deleteErr = apiClient.DeleteSimulation(dc.ResourceName)
	case "service":
		deleteErr = apiClient.DeleteService(dc.ResourceName)
	}

	if deleteErr != nil {
		logging.Verbose("API request failed: %v", deleteErr)
		return deleteErr
	}

	logging.Verbose("%s deleted successfully", strings.Title(dc.ResourceType))
	fmt.Printf("✓ %s '%s' deleted successfully.\n", strings.Title(dc.ResourceType), dc.ResourceName)

	return nil
}
