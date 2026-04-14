package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

	// Create a config
	expiresAt := time.Now().Add(24 * time.Hour)
	cfg := &Config{
		Username:             "test@example.com",
		APIToken:             "test-api-token",
		BearerToken:          "test-bearer-token",
		BearerTokenExpiresAt: expiresAt,
		APIEndpoint:          "https://air.nvidia.com/api",
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
	if loaded.BearerToken != cfg.BearerToken {
		t.Errorf("BearerToken mismatch: got %q, want %q", loaded.BearerToken, cfg.BearerToken)
	}
	if loaded.APIEndpoint != cfg.APIEndpoint {
		t.Errorf("APIEndpoint mismatch: got %q, want %q", loaded.APIEndpoint, cfg.APIEndpoint)
	}
	// Compare timestamps with tolerance due to JSON serialization
	if loaded.BearerTokenExpiresAt.Unix() != cfg.BearerTokenExpiresAt.Unix() {
		t.Errorf("BearerTokenExpiresAt mismatch: got %v, want %v", loaded.BearerTokenExpiresAt, cfg.BearerTokenExpiresAt)
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
		Username:             "test@example.com",
		APIToken:             "test-api-token",
		BearerToken:          "test-bearer-token",
		BearerTokenExpiresAt: time.Now().Add(24 * time.Hour),
		APIEndpoint:          "https://air.nvidia.com/api",
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

// TestIsTokenExpired tests the IsTokenExpired method.
func TestIsTokenExpired(t *testing.T) {
	now := time.Now()

	// Token expired in the past
	cfg := &Config{
		BearerTokenExpiresAt: now.Add(-1 * time.Hour),
	}
	if !cfg.IsTokenExpired(now) {
		t.Error("Expected token to be expired, but IsTokenExpired returned false")
	}

	// Token expires in the future
	cfg = &Config{
		BearerTokenExpiresAt: now.Add(1 * time.Hour),
	}
	if cfg.IsTokenExpired(now) {
		t.Error("Expected token to be valid, but IsTokenExpired returned true")
	}

	// Token expires exactly now (edge case)
	cfg = &Config{
		BearerTokenExpiresAt: now,
	}
	if !cfg.IsTokenExpired(now) {
		t.Error("Expected token expiring at 'now' to be expired")
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
		Username:             "test@example.com",
		APIToken:             "test-api-token",
		BearerToken:          "test-bearer-token",
		BearerTokenExpiresAt: time.Now().Add(24 * time.Hour),
		APIEndpoint:          "https://air.nvidia.com/api",
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

func TestResolveAPIEndpoint(t *testing.T) {
	t.Run("uses configured endpoint first", func(t *testing.T) {
		cfg := &Config{APIEndpoint: "https://example.com/api"}
		got := ResolveAPIEndpoint(cfg, "https://fallback.example/api")
		if got != "https://example.com/api" {
			t.Fatalf("ResolveAPIEndpoint() = %q, want configured endpoint", got)
		}
	})

	t.Run("falls back when config endpoint is empty", func(t *testing.T) {
		cfg := &Config{APIEndpoint: "   "}
		got := ResolveAPIEndpoint(cfg, "https://fallback.example/api")
		if got != "https://fallback.example/api" {
			t.Fatalf("ResolveAPIEndpoint() = %q, want fallback endpoint", got)
		}
	})

	t.Run("uses project default when both are empty", func(t *testing.T) {
		got := ResolveAPIEndpoint(nil, "")
		if got != constant.DefaultAPIEndpoint {
			t.Fatalf("ResolveAPIEndpoint() = %q, want %q", got, constant.DefaultAPIEndpoint)
		}
	})
}
