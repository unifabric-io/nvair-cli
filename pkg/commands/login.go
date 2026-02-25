package commands

import (
	"flag"
	"fmt"
	"net/mail"
	"os"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
)

// LoginCommand handles the login workflow.
type LoginCommand struct {
	Username    string
	APIToken    string
	APIEndpoint string
	KeyName     string
	Verbose     bool
}

// NewLoginCommand creates a new login command.
func NewLoginCommand() *LoginCommand {
	return &LoginCommand{
		APIEndpoint: "https://air.nvidia.com/api",
		KeyName:     "nvair-cli",
	}
}

// Register registers login command flags.
func (lc *LoginCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&lc.Username, "u", "", "Username/email (required)")
	fs.StringVar(&lc.Username, "user", "", "Username/email (required)")
	fs.StringVar(&lc.APIToken, "p", "", "API token (required)")
	fs.StringVar(&lc.APIToken, "password", "", "API token (required)")
	fs.StringVar(&lc.APIEndpoint, "api-endpoint", "https://air.nvidia.com/api", "API endpoint URL")
	fs.StringVar(&lc.KeyName, "key-name", "nvair-cli", "SSH key name to use for authentication")
	fs.BoolVar(&lc.Verbose, "v", false, "Enable verbose output")
	fs.BoolVar(&lc.Verbose, "verbose", false, "Enable verbose output")
}

// Execute runs the login command with the provided flags.
// Returns nil on success or an error on failure.
func (lc *LoginCommand) Execute() error {
	// Enable verbose logging if requested
	if lc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Login command started with username: %s", lc.Username)

	// Validate required flags
	if err := lc.validateFlags(); err != nil {
		logging.Verbose("Flag validation failed: %v", err)
		return err
	}

	logging.Verbose("Flags validated successfully")

	// Step 1: Authenticate with the platform
	logging.Verbose("Step 1/6: Authenticating with API endpoint: %s", lc.APIEndpoint)
	apiClient := api.NewClient(lc.APIEndpoint, "")
	bearerToken, expiresAt, err := apiClient.AuthLogin(lc.Username, lc.APIToken)
	if err != nil {
		logging.Verbose("Authentication failed: %v", err)
		return output.NewAuthError("Authentication failed", err)
	}
	logging.Verbose("Authentication successful, bearer token obtained, expires at: %s", expiresAt)

	// Step 2: Ensure SSH key pair exists (generate if missing)
	logging.Verbose("Step 2/6: Ensuring SSH key pair exists")
	keyPath, err := ssh.DefaultKeyPath()
	if err != nil {
		logging.Verbose("Failed to determine SSH key path: %v", err)
		return output.NewSSHKeyError("Failed to determine SSH key path", err)
	}
	logging.Verbose("SSH key path determined: %s", keyPath)

	kp, err := ssh.LoadOrGenerateKeyPair(keyPath)
	if err != nil {
		logging.Verbose("Failed to load or generate SSH key pair: %v", err)
		return output.NewSSHKeyError("Failed to load or generate SSH key pair", err)
	}
	logging.Verbose("SSH key pair ready, fingerprint: %s", kp.Fingerprint)

	// Step 3: Create an authenticated API client for SSH key operations
	logging.Verbose("Step 3/6: Creating authenticated API client for SSH key operations")
	authClient := api.NewClient(lc.APIEndpoint, bearerToken)

	// Step 4: Check if SSH key is already registered
	logging.Verbose("Step 4/6: Checking if SSH key is already registered")
	keys, err := authClient.GetSSHKeys()
	if err != nil {
		// Log warning but continue - don't block login
		logging.Verbose("Warning: Could not check existing SSH keys: %v", err)
		fmt.Printf("⚠ Warning: Could not check existing SSH keys: %v\n", err)
	} else {
		logging.Verbose("Retrieved %d existing SSH keys", len(keys))
	}

	// Find existing key with matching name
	var existingKey *api.GetSSHKeyResponse
	for _, key := range keys {
		if key.Name == lc.KeyName {
			existingKey = &key
			break
		}
	}

	// Step 5: Handle SSH key registration
	if existingKey != nil {
		if existingKey.Fingerprint == kp.Fingerprint {
			logging.Verbose("Step 5/6: SSH key with matching name and fingerprint already registered, skipping upload")
		} else {
			logging.Verbose("Step 5/6: SSH key with matching name but different fingerprint exists, deleting old key")
			err := authClient.DeleteSSHKey(existingKey.ID)
			if err != nil {
				logging.Verbose("Warning: Could not delete existing SSH key: %v", err)
				fmt.Printf("⚠ Warning: Could not delete existing SSH key: %v\n", err)
			} else {
				logging.Verbose("Existing SSH key deleted successfully")
				// Now upload the new key
				logging.Verbose("Uploading new SSH key")
				publicKeyStr := string(kp.PublicKey)
				_, err := authClient.CreateSSHKey(publicKeyStr, lc.KeyName)
				if err != nil {
					logging.Verbose("Warning: Could not upload SSH key: %v", err)
					fmt.Printf("⚠ Warning: Could not upload SSH key: %v\n", err)
				} else {
					logging.Verbose("SSH key uploaded successfully")
				}
			}
		}
	} else {
		logging.Verbose("Step 5/6: SSH key not found, uploading new key")
		publicKeyStr := string(kp.PublicKey)
		_, err := authClient.CreateSSHKey(publicKeyStr, lc.KeyName)
		if err != nil {
			logging.Verbose("Warning: Could not upload SSH key: %v", err)
			fmt.Printf("⚠ Warning: Could not upload SSH key: %v\n", err)
		} else {
			logging.Verbose("SSH key uploaded successfully")
		}
	}

	// Step 6: Save configuration to disk
	logging.Verbose("Step 6/6: Saving configuration to disk")
	cfg := &config.Config{
		Username:             lc.Username,
		APIToken:             lc.APIToken,
		BearerToken:          bearerToken,
		BearerTokenExpiresAt: expiresAt,
		APIEndpoint:          lc.APIEndpoint,
	}

	if err := cfg.Save(); err != nil {
		logging.Verbose("Failed to save configuration: %v", err)
		return output.NewFileError("Failed to save configuration", err)
	}

	// Step 7: Display success message
	configPath, _ := config.ConfigPath()
	logging.Verbose("Configuration saved to: %s", configPath)
	fmt.Printf("✓ Login successful. Credentials saved to %s\n", configPath)

	return nil
}

// validateFlags validates that required flags are provided and well-formed.
func (lc *LoginCommand) validateFlags() error {
	if lc.Username == "" {
		return output.NewValidationError("Email/username is required (-u or --user)")
	}

	if lc.APIToken == "" {
		return output.NewValidationError("API token is required (-p or --password)")
	}

	// Basic email validation
	if !isValidEmail(lc.Username) {
		return output.NewValidationError(fmt.Sprintf("Invalid email format: %s", lc.Username))
	}

	return nil
}

// isValidEmail performs basic email format validation.
func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}
