package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

// TestLoadNonExistent tests that loading a non-existent config returns an error.
func TestLoadNonExistent(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "nvair-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock home directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Try to load non-existent config
	_, err = Load()
	if err == nil {
		t.Error("Expected error when loading non-existent config, but got nil")
	}
}

// TestSaveAndLoad tests that we can save a config and load it back correctly.
func TestSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	cfg := &Config{
		Username:    "test@example.com",
		APIToken:    "test-api-token",
		APIEndpoint: "https://api.dsx-air.nvidia.com/api",
	}

	// Save the config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load the config back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded config matches the original
	if loaded.Username != cfg.Username {
		t.Errorf("Username mismatch: got %q, want %q", loaded.Username, cfg.Username)
	}
	if loaded.APIToken != cfg.APIToken {
		t.Errorf("APIToken mismatch: got %q, want %q", loaded.APIToken, cfg.APIToken)
	}
	if loaded.APIEndpoint != cfg.APIEndpoint {
		t.Errorf("APIEndpoint mismatch: got %q, want %q", loaded.APIEndpoint, cfg.APIEndpoint)
	}
}

// TestSaveFilePermissions verifies that the config file is created with 0600 permissions.
func TestSaveFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	cfg := &Config{
		Username:    "test@example.com",
		APIToken:    "test-api-token",
		APIEndpoint: "https://api.dsx-air.nvidia.com/api",
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Check file permissions
	configPath, _ := ConfigPath()
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	// Check that permissions are 0600
	expectedMode := os.FileMode(0600)
	actualMode := info.Mode() & os.ModePerm
	if actualMode != expectedMode {
		t.Errorf("Config file permissions mismatch: got %#o, want %#o", actualMode, expectedMode)
	}
}

// TestConfigPath verifies that ConfigPath returns the expected path.
func TestConfigPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".config", constant.DefaultKeyName, "config.json")
	if path != expectedPath {
		t.Errorf("ConfigPath mismatch: got %q, want %q", path, expectedPath)
	}
}

// TestSaveCreatesDirectory verifies that Save creates the config directory if it doesn't exist.
func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	cfg := &Config{
		Username:    "test@example.com",
		APIToken:    "test-api-token",
		APIEndpoint: "https://api.dsx-air.nvidia.com/api",
	}

	// Verify directory doesn't exist before save
	configPath, _ := ConfigPath()
	dir := filepath.Dir(configPath)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Config directory should not exist before Save()")
	}

	// Save should create the directory
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Config directory was not created: %v", err)
	}
}
