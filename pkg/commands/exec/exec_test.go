package exec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

func TestResolveCredentials(t *testing.T) {
	ec := NewCommand()

	tests := []struct {
		name       string
		node       api.Node
		wantUser   string
		wantPass   string
		wantDirect bool
	}{
		{
			name: "ubuntu node",
			node: api.Node{
				Name:   "node-gpu-1",
				OSName: "generic/ubuntu2404",
			},
			wantUser: constant.DefaultUbuntuUser,
			wantPass: constant.DefaultUbuntuPassword,
		},
		{
			name: "cumulus switch",
			node: api.Node{
				Name:   "switch-gpu-leaf1",
				OSName: "cumulus-vx-5.15.0",
			},
			wantUser: constant.DefaultCumulusUser,
			wantPass: constant.DefaultCumulusNewPassword,
		},
		{
			name: "oob mgmt server",
			node: api.Node{
				Name: constant.OOBMgmtServerName,
			},
			wantUser:   constant.DefaultUbuntuUser,
			wantPass:   "",
			wantDirect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ec.resolveCredentials(tc.node)
			if got.TargetUser != tc.wantUser {
				t.Fatalf("TargetUser mismatch: got %q want %q", got.TargetUser, tc.wantUser)
			}
			if got.TargetPass != tc.wantPass {
				t.Fatalf("TargetPass mismatch: got %q want %q", got.TargetPass, tc.wantPass)
			}
			if got.DirectBastion != tc.wantDirect {
				t.Fatalf("DirectBastion mismatch: got %v want %v", got.DirectBastion, tc.wantDirect)
			}
		})
	}
}

func TestRegister_SimulationFlagOptional(t *testing.T) {
	ec := NewCommand()
	cmd := &cobra.Command{
		Use:           "exec <node-name>",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ec.Execute(args, cmd.ArgsLenAtDash())
		},
	}
	ec.Register(cmd)
	if flag := cmd.Flags().Lookup("api-endpoint"); flag != nil {
		t.Fatalf("did not expect api-endpoint flag to be registered")
	}
	cmd.SetArgs([]string{"node-gpu-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected command execution error")
	}
	if strings.Contains(err.Error(), "required flag(s) \"simulation\" not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRemoteCommand_PreservesArgumentBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		dashIndex int
		want      string
	}{
		{
			name:      "preserves sh -c command string with spaces",
			args:      []string{"node-gpu-1", "sh", "-c", "exit 99"},
			dashIndex: 1,
			want:      "'sh' '-c' 'exit 99'",
		},
		{
			name:      "escapes single quote safely",
			args:      []string{"node-gpu-1", "echo", "it's-ok"},
			dashIndex: 1,
			want:      "'echo' 'it'\"'\"'s-ok'",
		},
		{
			name:      "preserves empty args",
			args:      []string{"node-gpu-1", "printf", "", "x"},
			dashIndex: 1,
			want:      "'printf' '' 'x'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRemoteCommand(tt.args, tt.dashIndex)
			if err != nil {
				t.Fatalf("parseRemoteCommand() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRemoteCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecute_InteractiveModeWithCommandUsesInteractiveCommandRunner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{
						"id":    "sim-1",
						"title": "simple",
						"state": "RUNNING",
					},
				},
			})
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":           "svc-custom",
					"name":         "gpu-ssh",
					"service_type": "other",
					"host":         "worker01.air.nvidia.com",
					"src_port":     17922,
				},
				{
					"id":           "svc-1",
					"name":         "bastion-ssh",
					"service_type": "ssh",
					"host":         "worker01.air.nvidia.com",
					"src_port":     10022,
				},
			})
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{
						"id":         "node-1",
						"name":       "node-gpu-1",
						"state":      "RUNNING",
						"metadata":   `{"mgmt_ip":"192.168.200.6"}`,
						"os":         "img-ubuntu",
						"simulation": "sim-1",
					},
				},
			})
		case r.URL.Path == "/v2/images" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{"id": "img-ubuntu", "name": "generic/ubuntu2404"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	origDefaultKeyPathFn := defaultKeyPathFn
	origInteractiveCommandViaBastion := interactiveCommandViaBastion
	origInteractiveSessionViaBastion := interactiveSessionViaBastion
	t.Cleanup(func() {
		defaultKeyPathFn = origDefaultKeyPathFn
		interactiveCommandViaBastion = origInteractiveCommandViaBastion
		interactiveSessionViaBastion = origInteractiveSessionViaBastion
	})

	defaultKeyPathFn = func() (string, error) { return "/tmp/mock-key", nil }

	calledInteractiveCommand := false
	calledInteractiveShell := false
	var gotCfg bastion.BastionExecConfig
	interactiveCommandViaBastion = func(cfg bastion.BastionExecConfig) error {
		calledInteractiveCommand = true
		gotCfg = cfg
		return nil
	}
	interactiveSessionViaBastion = func(cfg bastion.BastionExecConfig) error {
		calledInteractiveShell = true
		return nil
	}

	ec := NewCommand()
	ec.APIEndpoint = server.URL
	ec.Stdin = true
	ec.TTY = true

	err := ec.Execute([]string{"node-gpu-1", "bash", "-lc", "echo hi"}, 1)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !calledInteractiveCommand {
		t.Fatalf("expected interactive command runner to be called")
	}
	if calledInteractiveShell {
		t.Fatalf("did not expect interactive shell runner to be called")
	}
	if gotCfg.Command != "'bash' '-lc' 'echo hi'" {
		t.Fatalf("unexpected interactive command: %q", gotCfg.Command)
	}
}

func TestExecute_InteractiveModeWithoutCommandUsesShellRunner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{
						"id":    "sim-1",
						"title": "simple",
						"state": "RUNNING",
					},
				},
			})
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":           "svc-1",
					"name":         "bastion-ssh",
					"service_type": "ssh",
					"host":         "worker01.air.nvidia.com",
					"src_port":     10022,
				},
			})
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{
						"id":         "node-1",
						"name":       "node-gpu-1",
						"state":      "RUNNING",
						"metadata":   `{"mgmt_ip":"192.168.200.6"}`,
						"os":         "img-ubuntu",
						"simulation": "sim-1",
					},
				},
			})
		case r.URL.Path == "/v2/images" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{"id": "img-ubuntu", "name": "generic/ubuntu2404"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	origDefaultKeyPathFn := defaultKeyPathFn
	origInteractiveCommandViaBastion := interactiveCommandViaBastion
	origInteractiveSessionViaBastion := interactiveSessionViaBastion
	t.Cleanup(func() {
		defaultKeyPathFn = origDefaultKeyPathFn
		interactiveCommandViaBastion = origInteractiveCommandViaBastion
		interactiveSessionViaBastion = origInteractiveSessionViaBastion
	})

	defaultKeyPathFn = func() (string, error) { return "/tmp/mock-key", nil }

	calledInteractiveCommand := false
	calledInteractiveShell := false
	interactiveCommandViaBastion = func(cfg bastion.BastionExecConfig) error {
		calledInteractiveCommand = true
		return nil
	}
	interactiveSessionViaBastion = func(cfg bastion.BastionExecConfig) error {
		calledInteractiveShell = true
		return nil
	}

	ec := NewCommand()
	ec.APIEndpoint = server.URL
	ec.Stdin = true
	ec.TTY = true

	err := ec.Execute([]string{"node-gpu-1"}, -1)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if calledInteractiveCommand {
		t.Fatalf("did not expect interactive command runner to be called")
	}
	if !calledInteractiveShell {
		t.Fatalf("expected interactive shell runner to be called")
	}
}

func TestExecute_NodeNotFoundIncludesAvailableNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{
						"id":    "sim-1",
						"title": "simple",
						"state": "RUNNING",
					},
				},
			})
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":           "svc-1",
					"name":         "bastion-ssh",
					"service_type": "ssh",
					"host":         "worker01.air.nvidia.com",
					"src_port":     10022,
				},
			})
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 2,
				"results": []map[string]interface{}{
					{
						"id":         "node-1",
						"name":       "node-gpu-1",
						"state":      "RUNNING",
						"metadata":   `{"mgmt_ip":"192.168.200.6"}`,
						"os":         "img-ubuntu",
						"simulation": "sim-1",
					},
					{
						"id":         "node-2",
						"name":       "switch-gpu-leaf1",
						"state":      "RUNNING",
						"metadata":   `{"mgmt_ip":"192.168.200.111"}`,
						"os":         "img-cumulus",
						"simulation": "sim-1",
					},
				},
			})
		case r.URL.Path == "/v2/images" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 2,
				"results": []map[string]interface{}{
					{"id": "img-ubuntu", "name": "generic/ubuntu2404"},
					{"id": "img-cumulus", "name": "cumulus-vx-5.15.0"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ec := NewCommand()
	ec.APIEndpoint = server.URL
	ec.SimulationName = ""

	err := ec.Execute([]string{"does-not-exist", "hostname"}, 1)
	if err == nil {
		t.Fatalf("expected node-not-found error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "node not found: does-not-exist") {
		t.Fatalf("unexpected error message: %s", msg)
	}
	if !strings.Contains(msg, "node-gpu-1") || !strings.Contains(msg, "switch-gpu-leaf1") {
		t.Fatalf("expected available node names in error, got: %s", msg)
	}
}

func TestExecute_RequiresSimulationWhenMultipleExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/simulations" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 2,
				"results": []map[string]interface{}{
					{"id": "sim-1", "title": "simple", "state": "RUNNING"},
					{"id": "sim-2", "title": "lab-b", "state": "RUNNING"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ec := NewCommand()
	ec.APIEndpoint = server.URL
	ec.SimulationName = ""

	err := ec.Execute([]string{"node-gpu-1", "hostname"}, 1)
	if err == nil {
		t.Fatalf("expected simulation selection validation error")
	}
	if !strings.Contains(err.Error(), "--simulation <name> is required (2 simulations found)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_SimulationNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/simulations" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{"id": "sim-1", "title": "simple", "state": "RUNNING"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ec := NewCommand()
	ec.APIEndpoint = server.URL
	ec.SimulationName = "missing-sim"

	err := ec.Execute([]string{"node-gpu-1", "hostname"}, 1)
	if err == nil {
		t.Fatalf("expected simulation-not-found error")
	}
	if !strings.Contains(err.Error(), "simulation not found: missing-sim") {
		t.Fatalf("unexpected error: %v", err)
	}
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

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path error: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
}
