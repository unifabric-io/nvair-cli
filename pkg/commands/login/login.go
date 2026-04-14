package login

import (
	"fmt"
	"net/mail"
	"os"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
)

// Command handles the login workflow.
type Command struct {
	Username    string
	APIToken    string
	APIEndpoint string
	KeyName     string
	Verbose     bool
}

// NewCommand creates a new login command.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: constant.DefaultAPIEndpoint,
		KeyName:     "nvair-cli",
	}
}

// Register registers login command flags.
func (lc *Command) Register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVarP(&lc.Username, "user", "u", lc.Username, "Username/Email (required)")
	flags.StringVarP(&lc.APIToken, "password", "p", lc.APIToken, "API token (required)")
	flags.StringVar(&lc.APIEndpoint, "api-endpoint", lc.APIEndpoint, "API endpoint URL")
	flags.StringVar(&lc.KeyName, "key-name", lc.KeyName, "SSH key name to use for authentication")
}

// Execute runs the login command with the provided flags.
func (lc *Command) Execute() error {
	if lc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	logging.Verbose("Login command started with username: %s", lc.Username)

	if err := lc.validateFlags(); err != nil {
		logging.Verbose("Flag validation failed: %v", err)
		return err
	}

	logging.Verbose("Flags validated successfully")

	logging.Verbose("Step 1/6: Authenticating with API endpoint: %s", lc.APIEndpoint)
	apiClient := api.NewClient(lc.APIEndpoint, "")
	bearerToken, expiresAt, err := apiClient.AuthLogin(lc.Username, lc.APIToken)
	if err != nil {
		logging.Verbose("Authentication failed: %v", err)
		return output.NewAuthError("Authentication failed", err)
	}
	logging.Verbose("Authentication successful, bearer token obtained, expires at: %s", expiresAt)

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

	logging.Verbose("Step 3/6: Creating authenticated API client for SSH key operations")
	authClient := api.NewClient(lc.APIEndpoint, bearerToken)

	logging.Verbose("Step 4/6: Checking if SSH key is already registered")
	keys, err := authClient.GetSSHKeys()
	if err != nil {
		logging.Error("Warning: Could not check existing SSH keys: %v", err)
	} else {
		logging.Verbose("Retrieved %d existing SSH keys", len(keys))
	}

	var existingKey *api.GetSSHKeyResponse
	for _, key := range keys {
		if key.Name == lc.KeyName {
			existingKey = &key
			break
		}
	}

	if existingKey != nil {
		if existingKey.Fingerprint == kp.Fingerprint {
			logging.Verbose("Step 5/6: SSH key with matching name and fingerprint already registered, skipping upload")
		} else {
			logging.Verbose("Step 5/6: SSH key with matching name but different fingerprint exists, deleting old key")
			if err := authClient.DeleteSSHKey(existingKey.ID); err != nil {
				logging.Info("Warning: Could not delete existing SSH key: %v", err)
			} else {
				logging.Verbose("Existing SSH key deleted successfully")
				logging.Verbose("Uploading new SSH key")
				publicKeyStr := string(kp.PublicKey)
				if _, err := authClient.CreateSSHKey(publicKeyStr, lc.KeyName); err != nil {
					logging.Error("Could not upload SSH key: %v", err)
					return err
				} else {
					logging.Verbose("SSH key uploaded successfully")
				}
			}
		}
	} else {
		logging.Verbose("Step 5/6: SSH key not found, uploading new key")
		publicKeyStr := string(kp.PublicKey)
		if _, err := authClient.CreateSSHKey(publicKeyStr, lc.KeyName); err != nil {
			logging.Error("Could not upload SSH key: %v", err)
			return err
		} else {
			logging.Verbose("SSH key uploaded successfully")
		}
	}

	logging.Verbose("Step 6/6: Saving configuration to disk")
	cfg := &config.Config{
		Username:             lc.Username,
		APIToken:             lc.APIToken,
		BearerToken:          bearerToken,
		BearerTokenExpiresAt: expiresAt,
		APIEndpoint:          lc.APIEndpoint,
	}

	if err := cfg.Save(); err != nil {
		logging.Error("Failed to save configuration: %v", err)
		return output.NewFileError("Failed to save configuration", err)
	}

	configPath, _ := config.ConfigPath()
	logging.Info("✓ Login successful. Credentials saved to %s", configPath)

	return nil
}

func (lc *Command) validateFlags() error {
	if lc.Username == "" {
		return output.NewValidationError("Email/username is required (-u or --user)")
	}

	if lc.APIToken == "" {
		return output.NewValidationError("API token is required (-p or --password)")
	}

	if !isValidEmail(lc.Username) {
		return output.NewValidationError(fmt.Sprintf("Invalid email format: %s", lc.Username))
	}

	return nil
}

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}
