package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/output"
)

// LogoutCommand handles the logout workflow.
type LogoutCommand struct {
	// Force flag to skip confirmation (useful for scripting)
	Force   bool
	Verbose bool
}

// NewLogoutCommand creates a new logout command.
func NewLogoutCommand() *LogoutCommand {
	return &LogoutCommand{}
}

// Register registers logout command flags.
func (lc *LogoutCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&lc.Force, "f", false, "Force logout without confirmation")
	fs.BoolVar(&lc.Force, "force", false, "Force logout without confirmation")
	fs.BoolVar(&lc.Verbose, "v", false, "Enable verbose output")
	fs.BoolVar(&lc.Verbose, "verbose", false, "Enable verbose output")
}

// Execute runs the logout command.
// Returns nil on success or an error on failure.
func (lc *LogoutCommand) Execute() error {
	// Enable verbose logging if requested
	if lc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Logout command started")

	// Check if user is logged in
	logging.Verbose("Checking if user is logged in")
	cfg, err := config.Load()
	if err != nil {
		// Config doesn't exist - already logged out
		if os.IsNotExist(err) {
			logging.Verbose("Config file does not exist, user is already logged out")
			fmt.Println("✓ Already logged out")
			return nil
		}
		logging.Verbose("Failed to load configuration: %v", err)
		return output.NewConfigError("Failed to load configuration", err)
	}

	logging.Verbose("Config loaded successfully for user: %s", cfg.Username)

	// Verify we have valid credentials
	if cfg.Username == "" || cfg.BearerToken == "" {
		logging.Verbose("No valid credentials found in config")
		fmt.Println("✓ Already logged out")
		return nil
	}

	// Request confirmation unless forced
	if !lc.Force {
		logging.Verbose("Requesting user confirmation before logout")
		fmt.Printf("This will log out user %s. Continue? (y/n): ", cfg.Username)
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil {
			logging.Verbose("Failed to read user input: %v", err)
			return output.NewValidationError("Failed to read user input")
		}
		if response != "y" && response != "Y" && response != "yes" && response != "YES" {
			logging.Verbose("User declined logout confirmation")
			fmt.Println("Logout cancelled")
			return nil
		}
		logging.Verbose("User confirmed logout")
	} else {
		logging.Verbose("Force flag enabled, skipping confirmation")
	}

	// Delete the config file
	logging.Verbose("Deleting config file")
	configPath, err := config.ConfigPath()
	if err != nil {
		logging.Verbose("Failed to determine config path: %v", err)
		return output.NewConfigError("Failed to determine config path", err)
	}

	logging.Verbose("Config path: %s", configPath)
	if err := os.Remove(configPath); err != nil {
		// File doesn't exist is acceptable
		if !os.IsNotExist(err) {
			logging.Verbose("Failed to delete configuration file: %v", err)
			return output.NewConfigError("Failed to delete configuration", err)
		}
		logging.Verbose("Config file does not exist, but logout is proceeding")
	}

	logging.Verbose("Successfully deleted config file for user: %s", cfg.Username)
	fmt.Printf("✓ Successfully logged out user %s\n", cfg.Username)
	return nil
}
