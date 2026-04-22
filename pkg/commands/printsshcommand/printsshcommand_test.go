package printsshcommand

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

func TestPrintSSHCommand_AutoSelectSingleSimulation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/interfaces/services/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"forward-20000->node-gpu-1:22","simulation":"sim-1","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"},
				{"id":"svc-2","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	homeDir := setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	stdout, stderr, err := executePrintSSHCommandWithIO(t, []string{}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	want := fmt.Sprintf(
		"ssh -i '%s' -p 16821 ubuntu@worker01.air.nvidia.com\n",
		filepath.Join(homeDir, ".ssh", constant.DefaultKeyName),
	)
	if stdout != want {
		t.Fatalf("unexpected output:\nwant=%q\ngot=%q", want, stdout)
	}
	if !strings.Contains(stderr, `Using simulation "lab-a" by default.`) {
		t.Fatalf("expected auto-selected simulation notice, got stderr=%q", stderr)
	}
}

func TestPrintSSHCommand_RequiresSimulationWhenMultipleExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"},{"id":"sim-2","name":"lab-b","state":"RUNNING"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	_, err := executePrintSSHCommand(t, []string{}, server.URL)
	if err == nil || !strings.Contains(err.Error(), "--simulation <name> is required (2 simulations found)") {
		t.Fatalf("expected missing simulation validation error, got: %v", err)
	}
}

func executePrintSSHCommand(t *testing.T, args []string, endpoint string) (string, error) {
	t.Helper()
	stdout, _, err := executePrintSSHCommandWithIO(t, args, endpoint)
	return stdout, err
}

func executePrintSSHCommandWithIO(t *testing.T, args []string, endpoint string) (string, string, error) {
	t.Helper()
	pc := NewCommand()
	pc.APIEndpoint = endpoint

	cmd := &cobra.Command{
		Use:           "print-ssh-command",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pc.Execute(cmd)
		},
	}
	pc.Register(cmd)
	cmd.SetArgs(args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func setupConfig(t *testing.T, endpoint, apiToken string, _ time.Time) string {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := &config.Config{
		Username:    "user@example.com",
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
