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
		case "/v3/simulations":
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/":
			_, _ = w.Write([]byte(`{"count":2,"results":[{"id":"n1","name":"leaf-1","state":"RUNNING","metadata":null,"os":"img-cumulus","simulation":"sim-1"},{"id":"n2","name":"host-1","state":"RUNNING","metadata":null,"os":"img-ubuntu","simulation":"sim-1"}]}`))
		case "/v3/images":
			_, _ = w.Write([]byte(`{"count":2,"results":[{"id":"img-cumulus","name":"cumulus-linux"},{"id":"img-ubuntu","name":"generic-ubuntu"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
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

func TestGetNodes_AutoSelectSingleSimulation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"node-1","name":"leaf-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.10\"}","os":"img-ubuntu","simulation":"sim-1"}]}`))
		case "/v3/images":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"img-ubuntu","name":"generic-ubuntu"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	stdout, stderr, err := executeGetWithIO(t, []string{"nodes", "-o", "json"}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(stdout, "\"name\": \"leaf-1\"") {
		t.Fatalf("expected node output, got: %q", stdout)
	}
	if !strings.Contains(stderr, `Using simulation "lab-a" by default.`) {
		t.Fatalf("expected auto-selected simulation notice, got stderr=%q", stderr)
	}
}

func TestGetNodes_RequiresSimulationWhenMultipleExist(t *testing.T) {
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
	_, err := executeGet(t, []string{"nodes", "-o", "json"}, server.URL)
	if err == nil || !strings.Contains(err.Error(), "--simulation <name> is required (2 simulations found)") {
		t.Fatalf("expected missing simulation validation error, got: %v", err)
	}
}

func TestGetNodes_SimulationNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	_, err := executeGet(t, []string{"nodes", "--simulation", "does-not-exist"}, server.URL)
	if err == nil || !strings.Contains(err.Error(), "simulation not found") {
		t.Fatalf("expected simulation not found error, got: %v", err)
	}
}

func TestGetNodes_YAMLResultsOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"node-1","name":"leaf-1","state":"RUNNING","metadata":null,"os":"img-cumulus","simulation":"sim-1"}]}`))
		case "/v3/images":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"img-cumulus","name":"cumulus-linux"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
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
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"node-1","name":"leaf-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.10\"}","os":"img-ubuntu","simulation":"sim-1"}]}`))
		case "/v3/images":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"img-ubuntu","name":"generic-ubuntu"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
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
		case "/v3/simulations":
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/":
			_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
		case "/v3/images":
			_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
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

func TestGetForward_DefaultOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/interfaces/services/":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("simulation") != "sim-1" || r.URL.Query().Get("limit") != "25" {
				http.Error(w, "invalid query", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[{"id":"svc-1","name":"oob-mgmt-server SSH","node_port":22,"worker_port":23626,"worker_fqdn":"dc5d2f73.workers.ngc.air.nvidia.com","interface":"if-out","service_type":"SSH"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	stdout, stderr, err := executeGetWithIO(t, []string{"forward"}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !strings.Contains(stdout, "NAME") || !strings.Contains(stdout, "EXTERNAL") || !strings.Contains(stdout, "TARGET") {
		t.Fatalf("expected table header in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "oob-mgmt-server SSH") {
		t.Fatalf("expected ssh forward row, got: %q", stdout)
	}
	if !strings.Contains(stdout, "dc5d2f73.workers.ngc.air.nvidia.com:23626") {
		t.Fatalf("expected worker endpoint in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "oob-mgmt-server:22") {
		t.Fatalf("expected bastion destination in output, got: %q", stdout)
	}
	if !strings.Contains(stderr, `Using simulation "lab-a" by default.`) {
		t.Fatalf("expected auto-selected simulation notice, got stderr=%q", stderr)
	}
}

func TestGetForward_JSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/interfaces/services/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"next":null,"previous":null,"results":[
				{"id":"svc-1","name":"oob-mgmt-server SSH","node_port":22,"worker_port":16821,"worker_fqdn":"worker01.air.nvidia.com","interface":"if-out","service_type":"SSH"},
				{"id":"svc-2","name":"forward-20000->node-gpu-1:22","node_port":20000,"worker_port":17922,"worker_fqdn":"worker01.air.nvidia.com","interface":"if-out","service_type":"OTHER"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	out, err := executeGet(t, []string{"forward", "-o", "json"}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	var forwards []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &forwards); err != nil {
		t.Fatalf("output is not json array: %v; output=%q", err, out)
	}
	if len(forwards) != 2 {
		t.Fatalf("expected 2 forwards, got %d", len(forwards))
	}
	if forwards[0]["name"] != "forward-20000->node-gpu-1:22" {
		t.Fatalf("expected sorted first forward by name, got: %v", forwards[0]["name"])
	}
	if forwards[0]["target_host"] != "node-gpu-1" {
		t.Fatalf("expected parsed target host, got: %v", forwards[0]["target_host"])
	}
	if forwards[0]["target_port"] != float64(22) {
		t.Fatalf("expected parsed target port, got: %v", forwards[0]["target_port"])
	}
	if forwards[1]["address"] != "worker01.air.nvidia.com:16821" {
		t.Fatalf("expected normalized worker address, got: %v", forwards[1]["address"])
	}
}

func TestGetForward_YAMLOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/simulations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case "/v3/simulations/nodes/interfaces/services/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[{"id":"svc-1","name":"forward-20000->node-gpu-1:22","node_port":20000,"worker_port":17922,"worker_fqdn":"worker01.air.nvidia.com","interface":"if-out","service_type":"OTHER"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))
	out, err := executeGet(t, []string{"forwards", "-o", "yaml"}, server.URL)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !strings.Contains(out, "- id: svc-1") {
		t.Fatalf("expected forward in yaml output, got: %q", out)
	}
	if !strings.Contains(out, "address: worker01.air.nvidia.com:17922") {
		t.Fatalf("expected resolved address in yaml output, got: %q", out)
	}
	if !strings.Contains(out, "target_host: node-gpu-1") {
		t.Fatalf("expected parsed target host in yaml output, got: %q", out)
	}
	if !strings.Contains(out, "target_port: 22") {
		t.Fatalf("expected parsed target port in yaml output, got: %q", out)
	}
}

func executeGet(t *testing.T, args []string, endpoint string) (string, error) {
	t.Helper()
	stdout, _, err := executeGetWithIO(t, args, endpoint)
	return stdout, err
}

func executeGetWithIO(t *testing.T, args []string, endpoint string) (string, string, error) {
	t.Helper()
	gc := NewCommand()
	gc.APIEndpoint = endpoint

	cmd := &cobra.Command{Use: "get", SilenceErrors: true, SilenceUsage: true}
	gc.Register(cmd)
	cmd.SetArgs(args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func setupConfig(t *testing.T, endpoint, apiToken string, _ time.Time) {
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

	// Ensure config exists where expected.
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path error: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
}

func TestEnsureAuthenticatedClient_UsesSavedAPITokenDirectly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/login/" {
			t.Fatalf("login endpoint should not be called")
		}
		if r.URL.Path != "/v3/simulations" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer saved-token" {
			t.Fatalf("Authorization header mismatch: got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
	}))
	defer server.Close()

	setupConfig(t, server.URL, "saved-token", time.Now().Add(-1*time.Minute))
	client, cfg, err := ensureAuthenticatedClient(server.URL)
	if err == nil {
		_, err = client.GetSimulations()
	}
	if err != nil {
		t.Fatalf("expected saved API token to be usable directly, got: %v", err)
	}
	if cfg == nil || cfg.APIToken != "saved-token" {
		t.Fatalf("expected saved API token to remain unchanged, got %#v", cfg)
	}
}
