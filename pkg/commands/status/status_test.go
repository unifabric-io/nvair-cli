package status

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

func TestStatusCommand_NotLoggedInWithoutConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, err := executeStatusCommandWithIO(t, nil, constant.DefaultAPIEndpoint)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	want := "User          : Not logged in\nEndpoint      : api.dsx-air.nvidia.com\nAccess        : No\n"
	if stdout != want {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestStatusCommand_IncompleteConfigIsNotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		Username:    "user@example.com",
		APIToken:    "",
		APIEndpoint: constant.DefaultAPIEndpoint,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	stdout, _, err := executeStatusCommandWithIO(t, nil, constant.DefaultAPIEndpoint)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	want := "User          : Not logged in\nEndpoint      : api.dsx-air.nvidia.com\nAccess        : No\n"
	if stdout != want {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if strings.Contains(stdout, cfg.Username) {
		t.Fatalf("expected no stale username in output, got: %q", stdout)
	}
}

func TestStatusCommand_IncompleteConfigShowsConfiguredEndpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		Username:    "user@example.com",
		APIToken:    "",
		APIEndpoint: "https://configured.example/api",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	stdout, _, err := executeStatusCommandWithIO(t, nil, "https://fallback.example/api")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	want := "User          : Not logged in\nEndpoint      : configured.example\nAccess        : No\n"
	if stdout != want {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if strings.Contains(stdout, cfg.Username) {
		t.Fatalf("expected no stale username in output, got: %q", stdout)
	}
}

func TestStatusCommand_DoesNotAcceptAPIEndpointFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, _, err := executeStatusCommandWithIO(t, []string{"--api-endpoint", "https://example.com/api"}, constant.DefaultAPIEndpoint)
	if err == nil {
		t.Fatalf("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --api-endpoint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStatusCommand_CanConnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"count": 1,
			"results": []map[string]interface{}{
				{"id": "sim-1", "name": "lab-a", "state": "RUNNING"},
			},
		})
	}))
	defer server.Close()

	setupConfig(t, server.URL, "user@example.com", "api-token")

	stdout, _, err := executeStatusCommandWithIO(t, nil, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	want := "User          : user@example.com\nEndpoint      : " + displayEndpoint(server.URL) + "\nAccess        : Yes\n"
	if stdout != want {
		t.Fatalf("unexpected stdout:\nwant=%q\ngot=%q", want, stdout)
	}
}

func TestStatusCommand_CannotConnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations" {
			http.NotFound(w, r)
			return
		}

		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "user@example.com", "api-token")

	stdout, _, err := executeStatusCommandWithIO(t, nil, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	want := "User          : user@example.com\nEndpoint      : " + displayEndpoint(server.URL) + "\nAccess        : No\n"
	if stdout != want {
		t.Fatalf("unexpected stdout:\nwant=%q\ngot=%q", want, stdout)
	}
}

func TestStatusCommand_MissingAPITokenIsNotLoggedIn(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	setupConfig(t, server.URL, "user@example.com", "")

	stdout, _, err := executeStatusCommandWithIO(t, nil, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	want := "User          : Not logged in\nEndpoint      : " + displayEndpoint(server.URL) + "\nAccess        : No\n"
	if stdout != want {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func executeStatusCommandWithIO(t *testing.T, args []string, endpoint string) (string, string, error) {
	t.Helper()

	sc := NewCommand()
	sc.APIEndpoint = endpoint

	cmd := &cobra.Command{
		Use:           "status",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return sc.Execute(cmd)
		},
	}
	sc.Register(cmd)
	if args != nil {
		cmd.SetArgs(args)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func setupConfig(t *testing.T, endpoint, username, apiToken string) string {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := &config.Config{
		Username:    username,
		APIToken:    apiToken,
		APIEndpoint: endpoint,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path error: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("config file missing: %v", err)
	}

	return homeDir
}
