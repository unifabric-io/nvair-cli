package add

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

	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

func TestRegister_ForwardExposesTargetNodeAndTargetPortOnly(t *testing.T) {
	ac := NewCommand()
	cmd := &cobra.Command{Use: "add", SilenceErrors: true, SilenceUsage: true}
	ac.Register(cmd)

	forwardCmd, _, err := cmd.Find([]string{"forward"})
	if err != nil {
		t.Fatalf("find forward command: %v", err)
	}

	targetPortFlag := forwardCmd.Flags().Lookup("target-port")
	if targetPortFlag == nil {
		t.Fatalf("expected target-port flag to be registered")
	}
	if targetPortFlag.Shorthand != "" {
		t.Fatalf("expected no shorthand for target-port, got %q", targetPortFlag.Shorthand)
	}

	targetNodeFlag := forwardCmd.Flags().Lookup("target-node")
	if targetNodeFlag == nil {
		t.Fatalf("expected target-node flag to be registered")
	}
	if targetNodeFlag.Shorthand != "" {
		t.Fatalf("expected no shorthand for target-node, got %q", targetNodeFlag.Shorthand)
	}

	if flag := forwardCmd.Flags().Lookup("listen-port"); flag != nil {
		t.Fatalf("did not expect listen-port flag to be registered")
	}
	if flag := forwardCmd.Flags().Lookup("target-host"); flag != nil {
		t.Fatalf("did not expect target-host flag to be registered")
	}
	if flag := forwardCmd.Flags().Lookup("type"); flag != nil {
		t.Fatalf("did not expect type flag to be registered")
	}
}

func TestExecuteForward_SuccessAllocatesFirstAutoPort(t *testing.T) {
	var gotCreateBody map[string]interface{}
	stubBastionExecution(t, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-ssh","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","interface":"if-out","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v3/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v3/service/" && r.Method == "POST":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-1","name":"forward-20000->node-gpu-1:6443","simulation":"sim-1","interface":"if-out","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	if err := ac.executeForward(); err != nil {
		t.Fatalf("executeForward failed: %v", err)
	}

	if gotCreateBody["dest_port"] != float64(20000) {
		t.Fatalf("expected dest_port=20000, got: %v", gotCreateBody["dest_port"])
	}
	if gotCreateBody["service_type"] != "other" {
		t.Fatalf("expected service_type=other, got: %v", gotCreateBody["service_type"])
	}
	if gotCreateBody["name"] != "forward-20000->node-gpu-1:6443" {
		t.Fatalf("expected encoded forward name, got: %v", gotCreateBody["name"])
	}
}

func TestExecuteForward_SkipsUsedPortsWhenAllocating(t *testing.T) {
	var gotCreateBody map[string]interface{}
	stubBastionExecution(t, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-ssh","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"},
				{"id":"svc-1","name":"forward-20000->node-gpu-9:80","simulation":"sim-1","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"},
				{"id":"svc-2","name":"k8s-api-server","simulation":"sim-1","dest_port":20001,"src_port":17923,"service_type":"other","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v3/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v3/service/" && r.Method == "POST":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-3","name":"forward-20002->node-gpu-1:6443","simulation":"sim-1","interface":"if-out","dest_port":20002,"src_port":17924,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	if err := ac.executeForward(); err != nil {
		t.Fatalf("executeForward failed: %v", err)
	}

	if gotCreateBody["dest_port"] != float64(20002) {
		t.Fatalf("expected dest_port=20002, got: %v", gotCreateBody["dest_port"])
	}
	if gotCreateBody["name"] != "forward-20002->node-gpu-1:6443" {
		t.Fatalf("expected encoded forward name, got: %v", gotCreateBody["name"])
	}
}

func TestExecuteForward_ReusesExistingForwardForSameDestination(t *testing.T) {
	var createCalls int
	stubBastionExecution(t, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"forward-20005->node-gpu-1:6443","simulation":"sim-1","dest_port":20005,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"},
				{"id":"svc-ssh","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v3/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v3/service/" && r.Method == "POST":
			createCalls++
			http.Error(w, "unexpected create", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	if err := ac.executeForward(); err != nil {
		t.Fatalf("expected idempotent reuse, got: %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("expected no create call for existing forward, got %d", createCalls)
	}
}

func TestExecuteForward_CreatesDirectBastionForwardWithoutIPTables(t *testing.T) {
	var gotCreateBody map[string]interface{}
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		t.Fatalf("did not expect iptables setup for direct bastion forward")
		return nil, nil
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","name":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/v3/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v3/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v3/service/" && r.Method == "POST":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-ssh","name":"forward-22->oob-mgmt-server:22","simulation":"sim-1","interface":"if-out","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "api-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.TargetPort = 22
	ac.TargetNode = constant.OOBMgmtServerName

	if err := ac.executeForward(); err != nil {
		t.Fatalf("executeForward failed: %v", err)
	}

	if gotCreateBody["dest_port"] != float64(22) {
		t.Fatalf("expected dest_port=22, got: %v", gotCreateBody["dest_port"])
	}
	if gotCreateBody["service_type"] != "ssh" {
		t.Fatalf("expected service_type=ssh, got: %v", gotCreateBody["service_type"])
	}
	if gotCreateBody["name"] != "forward-22->oob-mgmt-server:22" {
		t.Fatalf("expected bastion forward name, got: %v", gotCreateBody["name"])
	}
}

func TestExecuteForward_TargetNodeRequired(t *testing.T) {
	ac := NewCommand()
	ac.TargetPort = 6443

	err := ac.executeForward()
	if err == nil || !strings.Contains(err.Error(), "--target-node is required") {
		t.Fatalf("expected missing target node error, got: %v", err)
	}
}

func TestExecuteForward_InvalidTargetPort(t *testing.T) {
	ac := NewCommand()
	ac.TargetPort = 70000
	ac.TargetNode = "node-gpu-1"

	err := ac.executeForward()
	if err == nil || !strings.Contains(err.Error(), "--target-port must be between 1 and 65535") {
		t.Fatalf("expected invalid target port error, got: %v", err)
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

func stubBastionExecution(t *testing.T, execFn func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error)) {
	t.Helper()

	oldKeyPathFn := defaultKeyPathFn
	oldExecFn := execCommandOnBastionFn

	defaultKeyPathFn = func() (string, error) {
		return "/tmp/nvair-test-key", nil
	}
	if execFn == nil {
		execFn = func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
			return &bastion.ExecResult{ExitCode: 0}, nil
		}
	}
	execCommandOnBastionFn = execFn

	t.Cleanup(func() {
		defaultKeyPathFn = oldKeyPathFn
		execCommandOnBastionFn = oldExecFn
	})
}
