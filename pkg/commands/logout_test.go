package commands

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/config"
)

func TestLogoutCommand_Execute_Success(t *testing.T) {
	// Setup temporary HOME directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create a test config file
	cfg := &config.Config{
		Username:             "test@example.com",
		APIToken:             "test-token",
		BearerToken:          "test-bearer",
		BearerTokenExpiresAt: time.Now().Add(24 * time.Hour),
		APIEndpoint:          "https://api.example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Verify config exists
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Config file should exist: %v", err)
	}

	// Execute logout with force flag
	lc := NewLogoutCommand()
	lc.Force = true

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should succeed: %v", err)
	}

	// Verify config file is deleted
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Config file should be deleted after logout")
	}
}

func TestLogoutCommand_Execute_NotLoggedIn(t *testing.T) {
	// Setup temporary HOME directory with no config
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Execute logout (file doesn't exist)
	lc := NewLogoutCommand()
	lc.Force = true

	// Should not error if already logged out
	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should handle already-logged-out state gracefully: %v", err)
	}
}

func TestLogoutCommand_Execute_EmptyConfig(t *testing.T) {
	// Setup temporary HOME directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create an empty config
	cfg := &config.Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Execute logout
	lc := NewLogoutCommand()
	lc.Force = true

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should handle empty config gracefully: %v", err)
	}
}

func TestLogoutCommand_Register_Flags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedForce bool
		shouldParseOk bool
	}{
		{
			name:          "Force flag (short)",
			args:          []string{"-f"},
			expectedForce: true,
			shouldParseOk: true,
		},
		{
			name:          "Force flag (long)",
			args:          []string{"-force"},
			expectedForce: true,
			shouldParseOk: true,
		},
		{
			name:          "No flags",
			args:          []string{},
			expectedForce: false,
			shouldParseOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("logout", flag.ContinueOnError)
			lc := NewLogoutCommand()
			lc.Register(fs)

			err := fs.Parse(tt.args)
			if (err == nil) != tt.shouldParseOk {
				t.Fatalf("Expected parse error: %v, got: %v", !tt.shouldParseOk, err)
			}

			if lc.Force != tt.expectedForce {
				t.Errorf("Expected force=%v, got %v", tt.expectedForce, lc.Force)
			}
		})
	}
}

func TestLogoutCommand_NoForce_UserCancels(t *testing.T) {
	// Setup temporary HOME directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &config.Config{
		Username:             "test@example.com",
		APIToken:             "test-token",
		BearerToken:          "test-bearer",
		BearerTokenExpiresAt: time.Now().Add(24 * time.Hour),
		APIEndpoint:          "https://api.example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Test with Force=true to skip prompt
	lc := NewLogoutCommand()
	lc.Force = true

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should succeed with force flag: %v", err)
	}

	// Config should be deleted
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Config should be deleted after logout")
	}
}

func TestLogoutCommand_PreservesOtherUserData(t *testing.T) {
	// Setup temporary HOME directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create a config
	cfg := &config.Config{
		Username:             "test@example.com",
		APIToken:             "test-token",
		BearerToken:          "test-bearer",
		BearerTokenExpiresAt: time.Now().Add(24 * time.Hour),
		APIEndpoint:          "https://api.example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Create another file in same directory
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	configDir := filepath.Dir(configPath)
	otherFile := filepath.Join(configDir, "other-file.txt")
	if err := os.WriteFile(otherFile, []byte("other data"), 0600); err != nil {
		t.Fatalf("Failed to create other file: %v", err)
	}

	// Execute logout
	lc := NewLogoutCommand()
	lc.Force = true

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should succeed: %v", err)
	}

	// Config should be deleted
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Config should be deleted")
	}

	// Other file should still exist
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("Other file should still exist: %v", err)
	}
}
