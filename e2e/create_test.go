//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/config"
)

// TestIntegration_RealAPI_Create exercises the real NVIDIA Air API using nvair.
// It requires NV_AIR_USER and NV_AIR_TOKEN to be set and skips otherwise.
func TestIntegration_RealAPI_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	user := os.Getenv("NV_AIR_USER")
	token := os.Getenv("NV_AIR_TOKEN")
	if user == "" || token == "" {
		t.Skip("NV_AIR_USER and NV_AIR_TOKEN must be set for real API e2e test")
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	env := withHomeEnv(homeDir)

	loginResult := runCommand(t, env, "login", "-u", user, "-p", token)
	logCommandOutput(t, loginResult, "TestIntegration_RealAPI_Create_login")
	if loginResult.ExitCode != 0 {
		t.Fatalf("nvair login failed with exit code %d: %v", loginResult.ExitCode, loginResult.Err)
	}
	if !strings.Contains(loginResult.Stdout+loginResult.Stderr, "Login successful") {
		t.Fatalf("unexpected login output: stdout=%q stderr=%q", loginResult.Stdout, loginResult.Stderr)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("could not determine config path: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created after login: %v", err)
	}

	simName := "simple"
	topoDir := filepath.Join("..", "examples", "simple")

	t.Logf("Creating simulation `%s` from topology in `%s`", simName, topoDir)

	t.Cleanup(func() {
		deleteResult := runCommand(t, env, "delete", "simulation", simName)
		logCommandOutput(t, deleteResult, "TestIntegration_RealAPI_Create_cleanup_delete")
	})

	createResult := runCommand(t, env, "create", "--delete-if-exists", "-v", "-d", topoDir)
	logCommandOutput(t, createResult, "TestIntegration_RealAPI_Create_create")
	if createResult.ExitCode != 0 {
		t.Fatalf("nvair create failed with exit code %d: %v", createResult.ExitCode, createResult.Err)
	}
	if !strings.Contains(createResult.Stdout+createResult.Stderr, "Simulation created successfully") {
		t.Fatalf("unexpected create output: stdout=%q stderr=%q", createResult.Stdout, createResult.Stderr)
	}
}

func TestIntegration_Create_DryRunNegativeScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	tests := []struct {
		name             string
		setup            func(t *testing.T) string
		args             []string
		expectedContains []string
		notContains      []string
	}{
		{
			name: "missing directory flag",
			args: []string{"create", "--dry-run"},
			expectedContains: []string{
				"directory flag is required (-d or --directory)",
				"Directory flag is required",
			},
			notContains: []string{
				"Ready to create",
			},
		},
		{
			name: "directory does not exist",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			args: []string{"create", "--dry-run"},
			expectedContains: []string{
				"directory not found",
			},
			notContains: []string{
				"Ready to create",
			},
		},
		{
			name: "invalid topology file structure",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeE2ETestFile(t, filepath.Join(dir, "topology.json"), "{\n  \"format\": \"JSON\",\n")
				return dir
			},
			args: []string{"create", "--dry-run"},
			expectedContains: []string{
				"failed to load topology: failed to parse topology:",
				"Failed to load topology:",
			},
			notContains: []string{
				"Ready to create",
			},
		},
		{
			name: "topology validation failure",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeE2ETestFile(t, filepath.Join(dir, "topology.json"), `{
					"format": "JSON",
					"content": {
						"nodes": {},
						"links": []
					}
				}`)
				return dir
			},
			args: []string{"create", "--dry-run"},
			expectedContains: []string{
				"✗ Topology validation failed:",
				"title: title field is required and cannot be empty",
				"content.nodes: nodes must have at least one node",
				"topology validation failed",
			},
			notContains: []string{
				"Ready to create",
			},
		},
		{
			name: "invalid netplan configuration",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeE2ETestFile(t, filepath.Join(dir, "topology.json"), `{
					"format": "JSON",
					"title": "invalid-netplan",
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
				writeE2ETestFile(t, filepath.Join(dir, "node-generic-1.yaml"), "network:\n  renderer: networkd\n")
				return dir
			},
			args: []string{"create", "--dry-run"},
			expectedContains: []string{
				"invalid netplan config for generic node node-generic-1",
				"missing network.version",
			},
			notContains: []string{
				"Ready to create",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			env := withHomeEnv(homeDir)

			args := append([]string{}, tt.args...)
			if tt.setup != nil {
				args = append(args, "-d", tt.setup(t))
			}

			result := runCommand(t, env, args...)
			logCommandOutput(t, result, "TestIntegration_Create_DryRunNegativeScenarios_"+strings.ReplaceAll(tt.name, " ", "_"))

			if result.ExitCode == 0 {
				t.Fatalf("expected command to fail, stdout=%q stderr=%q", result.Stdout, result.Stderr)
			}

			combinedOutput := result.Stdout + result.Stderr
			for _, expected := range tt.expectedContains {
				if !strings.Contains(combinedOutput, expected) {
					t.Fatalf("expected output to contain %q, stdout=%q stderr=%q", expected, result.Stdout, result.Stderr)
				}
			}

			for _, unexpected := range tt.notContains {
				if strings.Contains(combinedOutput, unexpected) {
					t.Fatalf("expected output not to contain %q, stdout=%q stderr=%q", unexpected, result.Stdout, result.Stderr)
				}
			}
		})
	}
}

func withHomeEnv(home string) []string {
	env := []string{"HOME=" + home}
	env = append(env, os.Environ()...)
	return env
}

func writeE2ETestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
