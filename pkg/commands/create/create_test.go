package create

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

func TestCreateCommand_ValidTopology(t *testing.T) {
	createCmd := NewCommand()
	createCmd.Directory = "../../../examples/simple"
	createCmd.DryRun = true

	err := createCmd.Execute()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestCreateCommand_MissingDirectory(t *testing.T) {
	createCmd := NewCommand()
	createCmd.Directory = "/tmp/nonexistent/path"
	createCmd.DryRun = true

	if err := createCmd.Execute(); err == nil {
		t.Fatalf("Expected error for missing directory, got nil")
	}
}

func TestCreateCommand_NoDirectoryFlag(t *testing.T) {
	createCmd := NewCommand()
	createCmd.Directory = ""
	createCmd.DryRun = true

	if err := createCmd.Execute(); err == nil {
		t.Fatalf("Expected error for missing directory flag, got nil")
	}
}

func TestCreateCommand_DoesNotAcceptAPIEndpointFlag(t *testing.T) {
	createCmd := NewCommand()
	cmd := &cobra.Command{Use: "create", SilenceErrors: true, SilenceUsage: true}
	createCmd.Register(cmd)
	cmd.SetArgs([]string{"--api-endpoint", "https://example.com/api"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --api-endpoint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTopologyLoading_Integration(t *testing.T) {
	topo, err := topology.LoadTopologyFromPath("../../topology/valid_topology.json")
	if err != nil {
		t.Fatalf("Failed to load topology: %v", err)
	}

	result := topology.ValidateTopology(topo)
	if !result.Valid {
		t.Fatalf("Valid topology failed validation: %v", result.Errors)
	}

	if topo.Title == "" {
		t.Errorf("Expected non-empty topology title")
	}

	if len(topo.Content.Nodes) == 0 {
		t.Errorf("Expected at least one node in content")
	}
}

func TestConfiguration_TokenExpired(t *testing.T) {
	expiredCfg := &config.Config{
		Username:             "test@example.com",
		BearerToken:          "test-token",
		BearerTokenExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	if !expiredCfg.IsTokenExpired(time.Now()) {
		t.Errorf("Expected token to be expired")
	}

	validCfg := &config.Config{
		Username:             "test@example.com",
		BearerToken:          "test-token",
		BearerTokenExpiresAt: time.Now().Add(23 * time.Hour),
	}

	if validCfg.IsTokenExpired(time.Now()) {
		t.Errorf("Expected token to be valid")
	}
}

func TestConfiguration_TokenRefreshScenario(t *testing.T) {
	expiredCfg := &config.Config{
		Username:             "test@example.com",
		APIToken:             "saved-api-token",
		BearerToken:          "old-bearer-token",
		BearerTokenExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	if !expiredCfg.IsTokenExpired(time.Now()) {
		t.Fatalf("Test setup error: token should be expired")
	}

	if expiredCfg.APIToken == "" {
		t.Errorf("Expected API token to be set for refresh")
	}

	if expiredCfg.Username == "" {
		t.Errorf("Expected username to be set for refresh")
	}

	newBearerToken := "new-bearer-token"
	newExpiresAt := time.Now().Add(24 * time.Hour)

	expiredCfg.BearerToken = newBearerToken
	expiredCfg.BearerTokenExpiresAt = newExpiresAt

	if expiredCfg.IsTokenExpired(time.Now()) {
		t.Errorf("Expected token to be valid after refresh")
	}

	if expiredCfg.BearerToken != newBearerToken {
		t.Errorf("Expected bearer token to be updated")
	}
}

func TestCreateCommand_GenericNodeNetplanValidated(t *testing.T) {
	dir := t.TempDir()
	writeTestTopology(t, dir, `{
		"format": "JSON",
		"title": "generic-netplan-valid",
		"content": {
			"nodes": {
				"node-generic-1": {
					"name": "node-generic-1",
					"os": "generic/ubuntu2404"
				}
			},
			"links": []
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "node-generic-1.yaml"), `network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: true
`)

	createCmd := NewCommand()
	createCmd.Directory = dir
	createCmd.DryRun = true

	if err := createCmd.Execute(); err != nil {
		t.Fatalf("Expected valid generic netplan, got: %v", err)
	}
}

func TestCreateCommand_GenericNodeInvalidNetplanFails(t *testing.T) {
	dir := t.TempDir()
	writeTestTopology(t, dir, `{
		"format": "JSON",
		"title": "generic-netplan-invalid",
		"content": {
			"nodes": {
				"node-generic-1": {
					"name": "node-generic-1",
					"os": "generic/ubuntu2404"
				}
			},
			"links": []
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "node-generic-1.yaml"), `network:
  version: two
`)

	createCmd := NewCommand()
	createCmd.Directory = dir
	createCmd.DryRun = true

	err := createCmd.Execute()
	if err == nil {
		t.Fatalf("Expected invalid generic netplan to fail")
	}
	if !strings.Contains(err.Error(), "invalid netplan config for generic node node-generic-1") {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestCreateCommand_GenericNodeMissingNetplanFails(t *testing.T) {
	dir := t.TempDir()
	writeTestTopology(t, dir, `{
		"format": "JSON",
		"title": "generic-netplan-missing",
		"content": {
			"nodes": {
				"node-generic-1": {
					"name": "node-generic-1",
					"os": "generic/ubuntu2404"
				}
			},
			"links": []
		}
	}`)

	createCmd := NewCommand()
	createCmd.Directory = dir
	createCmd.DryRun = true

	err := createCmd.Execute()
	if err == nil {
		t.Fatalf("Expected missing generic netplan to fail")
	}
	if !strings.Contains(err.Error(), "invalid netplan config for generic node node-generic-1") {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestResolveNodeImageNames(t *testing.T) {
	nodes := []api.Node{
		{ID: "1", Name: "switch-1", OS: "img-switch"},
		{ID: "2", Name: "node-1", OS: "img-generic"},
		{ID: "3", Name: "unknown-1", OS: "raw-os-id"},
	}
	images := []api.ImageInfo{
		{ID: "img-switch", Name: "cumulus-vx-5.10"},
		{ID: "img-generic", Name: "generic/ubuntu2404"},
	}

	resolved := resolveNodeImageNames(nodes, images)

	if resolved[0].OSName != "cumulus-vx-5.10" {
		t.Fatalf("Expected switch OS name to resolve, got %q", resolved[0].OSName)
	}
	if resolved[1].OSName != "generic/ubuntu2404" {
		t.Fatalf("Expected generic OS name to resolve, got %q", resolved[1].OSName)
	}
	if resolved[2].OSName != "raw-os-id" {
		t.Fatalf("Expected unresolved OS to fall back to raw ID, got %q", resolved[2].OSName)
	}

	switches := filterCumulusSwitchNodes(resolved)
	if len(switches) != 1 || switches[0].Name != "switch-1" {
		t.Fatalf("Expected one cumulus switch, got %+v", switches)
	}

	genericNodes := filterGenericUbuntuNodes(resolved)
	if len(genericNodes) != 1 || genericNodes[0].Name != "node-1" {
		t.Fatalf("Expected one generic node, got %+v", genericNodes)
	}
}

func TestJoinErrors(t *testing.T) {
	errCh := make(chan error, 3)
	err1 := errors.New("node-1 failed")
	err2 := errors.New("node-2 failed")
	errCh <- err1
	errCh <- nil
	errCh <- err2
	close(errCh)

	err := joinErrors(errCh)
	if err == nil {
		t.Fatal("Expected joined error, got nil")
	}
	if !errors.Is(err, err1) {
		t.Fatal("Expected joined error to contain err1")
	}
	if !errors.Is(err, err2) {
		t.Fatal("Expected joined error to contain err2")
	}
}

func TestFormatBastionSSHCommand(t *testing.T) {
	got := formatBastionSSHCommand("worker01.air.nvidia.com", 16821, "/tmp/my key")
	want := "ssh -i '/tmp/my key' -p 16821 ubuntu@worker01.air.nvidia.com"
	if got != want {
		t.Fatalf("formatBastionSSHCommand() = %q, want %q", got, want)
	}
}

func writeTestTopology(t *testing.T, dir, content string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "topology.json"), content)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
