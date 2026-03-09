package logout

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/output"
)

// Command handles the logout workflow.
type Command struct {
	Verbose bool
}

// NewCommand creates a new logout command.
func NewCommand() *Command {
	return &Command{}
}

// Register registers logout command flags.
func (lc *Command) Register(cmd *cobra.Command) {
	_ = cmd
}

// Execute runs the logout command.
func (lc *Command) Execute() error {
	if lc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Logout command started")

	logging.Verbose("Checking if user is logged in")
	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
			logging.Verbose("Config file does not exist, user is already logged out")
			logging.Info("✓ Already logged out")
			return nil
		}
		logging.Verbose("Failed to load configuration: %v", err)
		return output.NewConfigError("Failed to load configuration", err)
	}

	logging.Verbose("Config loaded successfully for user: %s", cfg.Username)

	if cfg.Username == "" || cfg.BearerToken == "" {
		logging.Verbose("No valid credentials found in config")
		logging.Info("✓ Already logged out")
		return nil
	}

	logging.Verbose("Deleting config file")
	configPath, err := config.ConfigPath()
	if err != nil {
		logging.Verbose("Failed to determine config path: %v", err)
		return output.NewConfigError("Failed to determine config path", err)
	}

	logging.Verbose("Config path: %s", configPath)
	if err := os.Remove(configPath); err != nil {
		if !os.IsNotExist(err) {
			logging.Verbose("Failed to delete configuration file: %v", err)
			return output.NewConfigError("Failed to delete configuration", err)
		}
		logging.Verbose("Config file does not exist, but logout is proceeding")
	}

	logging.Info("✓ Successfully logged out user %s", cfg.Username)
	return nil
}
