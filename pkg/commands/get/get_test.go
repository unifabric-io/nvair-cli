package get

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/testutil"
)

func TestNormalizeOutputFormat(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", formatDefault, false},
		{"json", formatJSON, false},
		{"yaml", formatYAML, false},
		{"xml", "", true},
	}

	for _, tc := range cases {
		got, err := normalizeOutputFormat(tc.in)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for %q", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("format mismatch for %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractMgmtIP(t *testing.T) {
	cases := []struct {
		name     string
		metadata string
		want     string
	}{
		{name: "valid", metadata: `{"mgmt_ip":"192.168.200.1"}`, want: "192.168.200.1"},
		{name: "empty", metadata: "", want: "-"},
		{name: "invalid json", metadata: "not-json", want: "-"},
		{name: "missing key", metadata: `{"foo":"bar"}`, want: "-"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractMgmtIP(tc.metadata)
			if got != tc.want {
				t.Fatalf("extractMgmtIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetSimulations_JSONResultsOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/simulations":
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case "/v2/simulations/nodes/":
			_, _ = w.Write([]byte(`{"count":2,"results":[{"id":"n1","name":"leaf-1","state":"RUNNING","metadata":null,"os":"img-cumulus","simulation":"sim-1"},{"id":"n2","name":"host-1","state":"RUNNING","metadata":null,"os":"img-ubuntu","simulation":"sim-1"}]}`))
		case "/v2/images":
			_, _ = w.Write([]byte(`{"count":2,"results":[{"id":"img-cumulus","name":"cumulus-linux"},{"id":"img-ubuntu","name":"generic-ubuntu"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))
	out, err := executeGet(t, []string{"simulations", "-o", "json"}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	var simulations []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &simulations); err != nil {
		t.Fatalf("output is not json array: %v; output=%q", err, out)
	}
	if len(simulations) != 1 {
		t.Fatalf("expected 1 simulation, got %d", len(simulations))
	}
	countField, hasCount := simulations[0]["count"]
	if !hasCount {
		t.Fatalf("expected count field in simulation summary, got: %v", simulations[0])
	}
	countMap, ok := countField.(map[string]interface{})
	if !ok {
		t.Fatalf("expected count to be an object, got: %T", countField)
	}
	if countMap["switch"] != float64(1) {
		t.Fatalf("expected count.switch=1, got: %v", countMap["switch"])
	}
	if countMap["host"] != float64(1) {
		t.Fatalf("expected count.host=1, got: %v", countMap["host"])
	}
}

func TestGetNodes_RequiresSimulation(t *testing.T) {
	_, err := executeGet(t, []string{"nodes", "-o", "json"}, "https://air.nvidia.com/api")
	if err == nil || !strings.Contains(err.Error(), "--simulation <name> is required") {
		t.Fatalf("expected missing simulation validation error, got: %v", err)
	}
}

func TestGetNodes_SimulationNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/simulations" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))
	_, err := executeGet(t, []string{"nodes", "--simulation", "does-not-exist"}, server.URL)
	if err == nil || !strings.Contains(err.Error(), "simulation not found") {
		t.Fatalf("expected simulation not found error, got: %v", err)
	}
}

func TestGetNodes_YAMLResultsOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case "/v2/simulations/nodes/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"node-1","name":"leaf-1","state":"RUNNING","metadata":null,"os":"img-cumulus","simulation":"sim-1"}]}`))
		case "/v2/images":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"img-cumulus","name":"cumulus-linux"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))
	out, err := executeGet(t, []string{"node", "--simulation", "lab-a", "-o", "yaml"}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if strings.Contains(out, "count:") || strings.Contains(out, "next:") || strings.Contains(out, "previous:") {
		t.Fatalf("results-only yaml expected, got: %q", out)
	}
	if !strings.Contains(out, "- id: node-1") {
		t.Fatalf("expected node in yaml output, got: %q", out)
	}
	if !strings.Contains(out, "image:") {
		t.Fatalf("expected image struct in yaml output, got: %q", out)
	}
	if !strings.Contains(out, "cumulus-linux") {
		t.Fatalf("expected resolved os name in yaml output, got: %q", out)
	}
}

func TestGetNodes_ShortSimulationFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case "/v2/simulations/nodes/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"node-1","name":"leaf-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.10\"}","os":"img-ubuntu","simulation":"sim-1"}]}`))
		case "/v2/images":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"img-ubuntu","name":"generic-ubuntu"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))
	out, err := executeGet(t, []string{"nodes", "-s", "lab-a"}, server.URL)
	if err != nil {
		t.Fatalf("execute with -s failed: %v", err)
	}

	if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "MGMT_IP") {
		t.Fatalf("expected aligned default table output, got: %q", out)
	}
	if strings.Contains(out, "ID") {
		t.Fatalf("did not expect ID column in default nodes output, got: %q", out)
	}
	if !strings.Contains(out, "leaf-1") {
		t.Fatalf("expected node row in output, got: %q", out)
	}
	if !strings.Contains(out, "192.168.200.10") {
		t.Fatalf("expected mgmt_ip in output, got: %q", out)
	}
}

func TestAliasesEquivalentForSimulationsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/simulations":
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case "/v2/simulations/nodes/":
			_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
		case "/v2/images":
			_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))
	outPlural, err := executeGet(t, []string{"simulations", "-o", "json"}, server.URL)
	if err != nil {
		t.Fatalf("plural execute failed: %v", err)
	}

	outSingular, err := executeGet(t, []string{"simulation", "-o", "json"}, server.URL)
	if err != nil {
		t.Fatalf("singular execute failed: %v", err)
	}

	if outPlural != outSingular {
		t.Fatalf("alias outputs differ\nplural=%q\nsingular=%q", outPlural, outSingular)
	}
}

func executeGet(t *testing.T, args []string, endpoint string) (string, error) {
	t.Helper()
	gc := NewCommand()
	gc.APIEndpoint = endpoint

	cmd := &cobra.Command{Use: "get", SilenceErrors: true, SilenceUsage: true}
	gc.Register(cmd)
	cmd.SetArgs(args)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	return out.String(), err
}

func setupConfig(t *testing.T, endpoint, bearer string, expiresAt time.Time) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := &config.Config{
		Username:             "user@example.com",
		APIToken:             "api-token",
		BearerToken:          bearer,
		BearerTokenExpiresAt: expiresAt,
		APIEndpoint:          endpoint,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Ensure config exists where expected.
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path error: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
}

func TestEnsureAuthenticatedClient_RefreshToken(t *testing.T) {
	jwt := testutil.MakeTestJWT(time.Now().Add(1 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/login/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"OK","message":"ok","token":"` + jwt + `"}`))
	}))
	defer server.Close()

	setupConfig(t, server.URL, "old-token", time.Now().Add(-1*time.Minute))
	client, cfg, err := ensureAuthenticatedClient(server.URL)
	if err != nil {
		t.Fatalf("expected refresh success, got: %v", err)
	}
	if client == nil || cfg == nil {
		t.Fatalf("expected non-nil client/config")
	}
	if cfg.BearerToken == "old-token" {
		t.Fatalf("expected bearer token to be refreshed")
	}
}

func TestEnsureAuthenticatedClient_RefreshTokenSaveFailure(t *testing.T) {
	jwt := testutil.MakeTestJWT(time.Now().Add(1 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/login/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"OK","message":"ok","token":"` + jwt + `"}`))
	}))
	defer server.Close()

	setupConfig(t, server.URL, "old-token", time.Now().Add(-1*time.Minute))

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("failed to get config path: %v", err)
	}

	configDir := filepath.Dir(configPath)
	if err := os.Chmod(configDir, 0500); err != nil {
		t.Fatalf("failed to make config directory read-only: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(configDir, 0700)
	})

	_, _, err = ensureAuthenticatedClient(server.URL)
	if err == nil {
		t.Fatalf("expected save failure error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to persist new token") {
		t.Fatalf("expected persist error, got: %v", err)
	}
}
