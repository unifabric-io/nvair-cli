package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

// Config represents the local configuration stored on the user's machine.
type Config struct {
	Username             string    `json:"username"`
	APIToken             string    `json:"apiToken"`
	BearerToken          string    `json:"bearerToken"`
	BearerTokenExpiresAt time.Time `json:"bearerTokenExpiresAt"`
	APIEndpoint          string    `json:"apiEndpoint"`
}

// IsTokenExpired returns true if the bearer token has expired.
func (c *Config) IsTokenExpired(now time.Time) bool {
	return !c.BearerTokenExpiresAt.After(now)
}

// ConfigPath returns the path to the config file.
// Default: ~/.config/nvair.unifabric.io/config.json
func ConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", constant.DefaultKeyName, "config.json"), nil
}

// Load reads the config file and unmarshals it into a Config struct.
// Returns an empty Config and error if the file doesn't exist or is unreadable.
func Load() (*Config, error) {
	configPath, err := ConfigPath()
	if err != nil {
		logging.Verbose("Load: Failed to determine config path: %v", err)
		return nil, err
	}

	logging.Verbose("Load: Loading configuration from %s", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		logging.Verbose("Load: Failed to read config file: %v", err)
		return nil, err
	}

	logging.Verbose("Load: Config file read successfully")

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		logging.Verbose("Load: Failed to unmarshal config JSON: %v", err)
		return nil, err
	}

	logging.Verbose("Load: Configuration loaded successfully for user: %s", cfg.Username)
	return &cfg, nil
}

// Save writes the config to disk with 0600 permissions (read/write owner only).
func (c *Config) Save() error {
	configPath, err := ConfigPath()
	if err != nil {
		logging.Verbose("Save: Failed to determine config path: %v", err)
		return err
	}

	logging.Verbose("Save: Saving configuration to %s", configPath)

	// Ensure the directory exists
	dir := filepath.Dir(configPath)
	logging.Verbose("Save: Ensuring config directory exists: %s", dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Verbose("Save: Failed to create config directory: %v", err)
		return err
	}

	// Marshal to JSON
	logging.Verbose("Save: Marshaling configuration to JSON")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		logging.Verbose("Save: Failed to marshal config to JSON: %v", err)
		return err
	}

	// Write to file with restricted permissions
	// First write to a temporary file, then rename (atomic operation)
	tmpFile := configPath + ".tmp"
	logging.Verbose("Save: Writing to temporary file: %s", tmpFile)
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		logging.Verbose("Save: Failed to write temporary config file: %v", err)
		return err
	}

	// Atomic rename
	logging.Verbose("Save: Atomically renaming temporary file to config file")
	if err := os.Rename(tmpFile, configPath); err != nil {
		logging.Verbose("Save: Failed to rename temporary file: %v", err)
		os.Remove(tmpFile) // Clean up temp file on error
		return err
	}

	logging.Verbose("Save: Configuration saved successfully")
	return nil
}
