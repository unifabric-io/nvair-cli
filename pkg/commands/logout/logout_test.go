package logout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/config"
)

func TestLogoutCommand_Execute_Success(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &config.Config{
		Username:    "test@example.com",
		APIToken:    "test-token",
		APIEndpoint: "https://api.example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Config file should exist: %v", err)
	}

	lc := NewCommand()

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should succeed: %v", err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Config file should be deleted after logout")
	}
}

func TestLogoutCommand_Execute_NotLoggedIn(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	lc := NewCommand()

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should handle already-logged-out state gracefully: %v", err)
	}
}

func TestLogoutCommand_Execute_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &config.Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	lc := NewCommand()

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should handle empty config gracefully: %v", err)
	}
}

func TestLogoutCommand_NoConfirmationRequired(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &config.Config{
		Username:    "test@example.com",
		APIToken:    "test-token",
		APIEndpoint: "https://api.example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	lc := NewCommand()

	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should succeed without confirmation: %v", err)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Config should be deleted after logout")
	}
}

func TestLogoutCommand_PreservesOtherUserData(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &config.Config{
		Username:    "test@example.com",
		APIToken:    "test-token",
		APIEndpoint: "https://api.example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	configDir := filepath.Dir(configPath)
	otherFile := filepath.Join(configDir, "other-file.txt")
	if err := os.WriteFile(otherFile, []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to create other file: %v", err)
	}

	lc := NewCommand()
	if err := lc.Execute(); err != nil {
		t.Fatalf("Logout should succeed: %v", err)
	}

	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("Other user data should remain untouched: %v", err)
	}
}
