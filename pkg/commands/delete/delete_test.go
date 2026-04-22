package delete

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestExecute_DeleteForwardByTarget(t *testing.T) {
	var deleteCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"},
				{"id":"svc-2","name":"forward-20000->node-gpu-1:6443","simulation":"sim-1","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v3/service/svc-2/" && r.Method == "DELETE":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	dc := NewCommand()
	dc.APIEndpoint = server.URL
	dc.ResourceType = "forward"
	dc.SimulationName = "lab-a"
	dc.TargetNode = "node-gpu-1"
	dc.TargetPort = 6443

	if err := dc.Execute(); err != nil {
		t.Fatalf("execute delete forward failed: %v", err)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE /v3/service/svc-2/ to be called")
	}
	if dc.ResourceName != "forward-20000->node-gpu-1:6443" {
		t.Fatalf("expected deleted resource name to be set, got %q", dc.ResourceName)
	}
}

func TestExecute_DeleteForwardByTarget_CompatibleWithLegacyName(t *testing.T) {
	var deleteCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"forward-ssh-10022->node-gpu-1:22","simulation":"sim-1","dest_port":10022,"src_port":17922,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v3/service/svc-1/" && r.Method == "DELETE":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	dc := NewCommand()
	dc.APIEndpoint = server.URL
	dc.ResourceType = "forward"
	dc.SimulationName = "lab-a"
	dc.TargetNode = "node-gpu-1"
	dc.TargetPort = 22

	if err := dc.Execute(); err != nil {
		t.Fatalf("execute delete forward failed: %v", err)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE /v3/service/svc-1/ to be called")
	}
}

func TestExecute_DeleteForwardNotFoundByTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	dc := NewCommand()
	dc.APIEndpoint = server.URL
	dc.ResourceType = "forward"
	dc.SimulationName = "lab-a"
	dc.TargetNode = "node-gpu-1"
	dc.TargetPort = 6443

	err := dc.Execute()
	if err == nil || !strings.Contains(err.Error(), "forward service not found for target node-gpu-1:6443") {
		t.Fatalf("expected forward not found error, got: %v", err)
	}
}

func TestExecute_DeleteForwardRequiresTargetNode(t *testing.T) {
	dc := NewCommand()
	dc.ResourceType = "forward"
	dc.TargetPort = 6443

	err := dc.Execute()
	if err == nil || !strings.Contains(err.Error(), "--target-node is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_DeleteForwardRequiresTargetPort(t *testing.T) {
	dc := NewCommand()
	dc.ResourceType = "forward"
	dc.TargetNode = "node-gpu-1"

	err := dc.Execute()
	if err == nil || !strings.Contains(err.Error(), "--target-port must be between 1 and 65535") {
		t.Fatalf("unexpected error: %v", err)
	}
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

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path error: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
}
