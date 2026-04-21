package delete

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
)

func TestValidateArgs_InvalidResourceType(t *testing.T) {
	err := ValidateArgs([]string{"invalid", "name"})
	if err == nil {
		t.Fatalf("expected error for invalid resource type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArgs_ServiceNotSupported(t *testing.T) {
	err := ValidateArgs([]string{"service", "name"})
	if err == nil {
		t.Fatalf("expected error for unsupported service resource type, got nil")
	}
	if !strings.Contains(err.Error(), "Must be 'simulation'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArgs_MissingArgs(t *testing.T) {
	err := ValidateArgs([]string{})
	if err == nil {
		t.Fatalf("expected error for missing arguments, got nil")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg(s), received 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_MissingMappedFields(t *testing.T) {
	dc := NewCommand()
	err := dc.Execute()
	if err == nil {
		t.Fatalf("expected error when required fields are not mapped")
	}
	if !strings.Contains(err.Error(), "usage: nvair delete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_DeleteForwardByName(t *testing.T) {
	var deleteCalled bool
	var bastionAddr string
	var cleanupCommand string
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		bastionAddr = cfg.BastionAddr
		cleanupCommand = cfg.Command
		return &bastion.ExecResult{ExitCode: 0}, nil
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"2001:db8::1","link":"","node_name":"oob-mgmt-server"},
				{"id":"svc-2","name":"gpu-api","simulation":"sim-1","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v1/service/svc-2/" && r.Method == "DELETE":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	dc := NewCommand()
	dc.APIEndpoint = server.URL
	dc.ResourceType = "forward"
	dc.ResourceName = "gpu-api"
	dc.SimulationName = "lab-a"

	if err := dc.Execute(); err != nil {
		t.Fatalf("execute delete forward failed: %v", err)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE /v1/service/svc-2/ to be called")
	}
	if dc.ResourceName != "gpu-api" {
		t.Fatalf("expected deleted resource name to be set, got %q", dc.ResourceName)
	}
	if bastionAddr != "[2001:db8::1]:16821" {
		t.Fatalf("expected IPv6 bastion address to use net.JoinHostPort formatting, got %q", bastionAddr)
	}
	if !strings.Contains(cleanupCommand, "nvair cli port: 20000") {
		t.Fatalf("expected cleanup command to target nvair port comment, got: %s", cleanupCommand)
	}
	if strings.Contains(cleanupCommand, "nl -w1") || strings.Contains(cleanupCommand, "line_number") {
		t.Fatalf("expected cleanup command not to delete by iptables -S line number, got: %s", cleanupCommand)
	}
	if strings.Contains(cleanupCommand, "eval") {
		t.Fatalf("expected cleanup command not to use eval, got: %s", cleanupCommand)
	}
	if !strings.Contains(cleanupCommand, `sed -n "s/^-A $chain /-D $chain /p"`) {
		t.Fatalf("expected cleanup command to transform matching -A rules into -D rules, got: %s", cleanupCommand)
	}
	if !strings.Contains(cleanupCommand, `xargs -r sudo iptables -t nat`) {
		t.Fatalf("expected cleanup command to execute transformed rules without eval, got: %s", cleanupCommand)
	}
}

func TestExecute_DeleteForwardByName_NonManagedPortSkipsIPTablesCleanup(t *testing.T) {
	var deleteCalled bool
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		t.Fatalf("did not expect iptables cleanup for non-managed listen port")
		return nil, nil
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v1/service/svc-1/" && r.Method == "DELETE":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	dc := NewCommand()
	dc.APIEndpoint = server.URL
	dc.ResourceType = "forward"
	dc.ResourceName = "bastion-ssh"
	dc.SimulationName = "lab-a"

	if err := dc.Execute(); err != nil {
		t.Fatalf("execute delete forward failed: %v", err)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE /v1/service/svc-1/ to be called")
	}
}

func TestExecute_DeleteForwardNotFoundByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	dc := NewCommand()
	dc.APIEndpoint = server.URL
	dc.ResourceType = "forward"
	dc.ResourceName = "gpu-api"
	dc.SimulationName = "lab-a"

	err := dc.Execute()
	if err == nil || !strings.Contains(err.Error(), `forward service "gpu-api" not found`) {
		t.Fatalf("expected forward not found error, got: %v", err)
	}
}

func TestExecute_DeleteForwardRequiresName(t *testing.T) {
	dc := NewCommand()
	dc.ResourceType = "forward"

	err := dc.Execute()
	if err == nil || !strings.Contains(err.Error(), "forward name is required") {
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

func stubBastionExecution(t *testing.T, execFn func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error)) {
	t.Helper()

	oldKeyPathFn := defaultKeyPathFn
	oldExecFn := execCommandOnBastionFn

	defaultKeyPathFn = func() (string, error) {
		return "/tmp/nvair-test-key", nil
	}
	execCommandOnBastionFn = execFn

	t.Cleanup(func() {
		defaultKeyPathFn = oldKeyPathFn
		execCommandOnBastionFn = oldExecFn
	})
}
