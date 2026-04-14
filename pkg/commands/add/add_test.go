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

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

func TestRegister_ForwardRequiresNameAndExposesTargetNodeAndTargetPortOnly(t *testing.T) {
	ac := NewCommand()
	cmd := &cobra.Command{Use: "add", SilenceErrors: true, SilenceUsage: true}
	ac.Register(cmd)

	if flag := cmd.PersistentFlags().Lookup("api-endpoint"); flag != nil {
		t.Fatalf("did not expect api-endpoint flag to be registered")
	}

	forwardCmd, _, err := cmd.Find([]string{"forward"})
	if err != nil {
		t.Fatalf("find forward command: %v", err)
	}
	if forwardCmd.Use != "forward <forward-name>" {
		t.Fatalf("expected forward command to require name, got use %q", forwardCmd.Use)
	}
	if _, _, err := cmd.Find([]string{"forwards", "gpu-api", "--target-node", "node-gpu-1", "--target-port", "6443"}); err != nil {
		t.Fatalf("expected forwards alias to resolve: %v", err)
	}
	if err := forwardCmd.Args(forwardCmd, []string{"gpu-api"}); err != nil {
		t.Fatalf("expected one forward name arg to be valid: %v", err)
	}
	if err := forwardCmd.Args(forwardCmd, nil); err == nil {
		t.Fatalf("expected missing forward name arg to be rejected")
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
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-ssh","name":"bastion-ssh","simulation":"sim-1","interface":"if-out","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v2/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v1/service/" && r.Method == "POST":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-1","name":"gpu-api","simulation":"sim-1","interface":"if-out","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.ForwardName = "gpu-api"
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
	if gotCreateBody["name"] != "gpu-api" {
		t.Fatalf("expected explicit forward name, got: %v", gotCreateBody["name"])
	}
}

func TestExecuteForward_SkipsUsedPortsFromIPTablesComments(t *testing.T) {
	var gotCreateBody map[string]interface{}
	var setupCommand string
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		if strings.Contains(cfg.Command, "iptables-save") {
			return &bastion.ExecResult{
				ExitCode: 0,
				Stdout: `-A PREROUTING -p tcp --dport 20000 -m comment --comment "nvair cli port: 20000" -j DNAT --to-destination 192.168.200.5:80
-A OUTPUT -p tcp --dport 20001 -m comment --comment "nvair cli port: 20001" -j DNAT --to-destination 192.168.200.6:443`,
			}, nil
		}
		setupCommand = cfg.Command
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
				{"id":"svc-ssh","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v2/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v1/service/" && r.Method == "POST":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-3","name":"gpu-api","simulation":"sim-1","interface":"if-out","dest_port":20002,"src_port":17924,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.ForwardName = "gpu-api"
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	if err := ac.executeForward(); err != nil {
		t.Fatalf("executeForward failed: %v", err)
	}

	if gotCreateBody["dest_port"] != float64(20002) {
		t.Fatalf("expected dest_port=20002, got: %v", gotCreateBody["dest_port"])
	}
	if gotCreateBody["name"] != "gpu-api" {
		t.Fatalf("expected explicit forward name, got: %v", gotCreateBody["name"])
	}
	if !strings.Contains(setupCommand, "--comment 'nvair cli port: 20002'") {
		t.Fatalf("expected iptables setup to include nvair port comment, got: %s", setupCommand)
	}
}

func TestExecuteForward_DoesNotReuseParsedForwardNameForSameDestination(t *testing.T) {
	var createCalls int
	var gotCreateBody map[string]interface{}
	stubBastionExecution(t, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"sim-1","title":"lab-a","state":"RUNNING"}]}`))
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id":"svc-1","name":"old-style-gpu-api","simulation":"sim-1","dest_port":20005,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"},
				{"id":"svc-ssh","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v2/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v1/service/" && r.Method == "POST":
			createCalls++
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-2","name":"gpu-api","simulation":"sim-1","interface":"if-out","dest_port":20000,"src_port":17924,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.ForwardName = "gpu-api"
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	if err := ac.executeForward(); err != nil {
		t.Fatalf("executeForward failed: %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("expected create call for explicit forward name, got %d", createCalls)
	}
	if gotCreateBody["dest_port"] != float64(20000) {
		t.Fatalf("expected dest_port=20000, got: %v", gotCreateBody["dest_port"])
	}
	if gotCreateBody["name"] != "gpu-api" {
		t.Fatalf("expected explicit forward name, got: %v", gotCreateBody["name"])
	}
}

func TestExecuteForward_RejectsExistingForwardNameForSameTarget(t *testing.T) {
	var createCalls int
	var setupCalls int
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		if strings.Contains(cfg.Command, "iptables-save") {
			return &bastion.ExecResult{
				ExitCode: 0,
				Stdout:   `-A PREROUTING -p tcp --dport 20005 -m comment --comment "nvair cli port: 20005" -j DNAT --to-destination 192.168.200.6:6443`,
			}, nil
		}
		setupCalls++
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
				{"id":"svc-1","name":"gpu-api","simulation":"sim-1","dest_port":20005,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"},
				{"id":"svc-ssh","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":2,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v1/service/" && r.Method == "POST":
			createCalls++
			http.Error(w, "unexpected create", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.ForwardName = "gpu-api"
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	err := ac.executeForward()
	if err == nil {
		t.Fatalf("expected name conflict error")
	}
	if !strings.Contains(err.Error(), `Forward name "gpu-api" already used for node-gpu-1:6443`) {
		t.Fatalf("expected existing target in conflict error, got: %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("expected no create call for existing forward, got %d", createCalls)
	}
	if setupCalls != 0 {
		t.Fatalf("expected no iptables setup for existing forward name, got %d", setupCalls)
	}
}

func TestExecuteForward_RejectsExistingForwardNameWithDifferentTarget(t *testing.T) {
	var createCalls int
	var setupCalls int
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		if strings.Contains(cfg.Command, "iptables-save") {
			return &bastion.ExecResult{
				ExitCode: 0,
				Stdout:   `-A PREROUTING -p tcp --dport 20005 -m comment --comment "nvair cli port: 20005" -j DNAT --to-destination 192.168.200.7:6445`,
			}, nil
		}
		setupCalls++
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
				{"id":"svc-1","name":"gpu-api","simulation":"sim-1","dest_port":20005,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"},
				{"id":"svc-ssh","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","link":"ssh://ubuntu@worker01.air.nvidia.com:16821","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":3,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-2","name":"node-gpu-1","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.6\"}","os":"img-ubuntu","simulation":"sim-1"},
				{"id":"node-3","name":"node-gpu-2","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.7\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v1/service/" && r.Method == "POST":
			createCalls++
			http.Error(w, "unexpected create", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.ForwardName = "gpu-api"
	ac.TargetPort = 6446
	ac.TargetNode = "node-gpu-2"

	err := ac.executeForward()
	if err == nil {
		t.Fatalf("expected name conflict error")
	}
	if !strings.Contains(err.Error(), `Forward name "gpu-api" already used for node-gpu-2:6445`) {
		t.Fatalf("expected existing target in conflict error, got: %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("expected no create call for conflicting forward name, got %d", createCalls)
	}
	if setupCalls != 0 {
		t.Fatalf("expected no iptables setup for conflicting forward name, got %d", setupCalls)
	}
}

func TestExecuteForward_TargetBastionSSHUsesManagedForwardPath(t *testing.T) {
	var gotCreateBody map[string]interface{}
	var setupCommand string
	stubBastionExecution(t, func(cfg bastion.BastionExecConfig) (*bastion.ExecResult, error) {
		if strings.Contains(cfg.Command, "iptables-save") {
			return &bastion.ExecResult{ExitCode: 0}, nil
		}
		setupCommand = cfg.Command
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
				{"id":"svc-ssh","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"}
			]`))
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[
				{"id":"node-1","name":"oob-mgmt-server","state":"RUNNING","metadata":"{\"mgmt_ip\":\"192.168.200.2\"}","os":"img-ubuntu","simulation":"sim-1"}
			]}`))
		case r.URL.Path == "/v2/simulations/nodes/interfaces" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"results":[{"id":"if-out","name":"eth0","interface_type":"ETHERNET","mac_address":"","link_up":true,"internal_ipv4":"","full_ipv6":"","prefix_ipv6":"","port_number":0,"node":"node-1","simulation":"sim-1","outbound":true,"link":""}]}`))
		case r.URL.Path == "/v1/service/" && r.Method == "POST":
			if err := json.NewDecoder(r.Body).Decode(&gotCreateBody); err != nil {
				t.Fatalf("failed to decode create service request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"svc-1","name":"bastion-ssh-forward","simulation":"sim-1","interface":"if-out","dest_port":20000,"src_port":17922,"service_type":"other","host":"worker01.air.nvidia.com","link":"","node_name":"oob-mgmt-server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	ac := NewCommand()
	ac.APIEndpoint = server.URL
	ac.SimulationName = "lab-a"
	ac.ForwardName = "bastion-ssh-forward"
	ac.TargetPort = 22
	ac.TargetNode = constant.OOBMgmtServerName

	if err := ac.executeForward(); err != nil {
		t.Fatalf("executeForward failed: %v", err)
	}

	if gotCreateBody["dest_port"] != float64(20000) {
		t.Fatalf("expected dest_port=20000, got: %v", gotCreateBody["dest_port"])
	}
	if gotCreateBody["service_type"] != "other" {
		t.Fatalf("expected service_type=other, got: %v", gotCreateBody["service_type"])
	}
	if gotCreateBody["name"] != "bastion-ssh-forward" {
		t.Fatalf("expected explicit bastion forward name, got: %v", gotCreateBody["name"])
	}
	if !strings.Contains(setupCommand, "--comment 'nvair cli port: 20000'") {
		t.Fatalf("expected iptables setup to include nvair port comment, got: %s", setupCommand)
	}
	if !strings.Contains(setupCommand, "--to-destination '192.168.200.2:22'") {
		t.Fatalf("expected iptables setup to target bastion SSH via NAT, got: %s", setupCommand)
	}
}

func TestExecuteForward_ForwardNameRequired(t *testing.T) {
	ac := NewCommand()
	ac.TargetPort = 6443
	ac.TargetNode = "node-gpu-1"

	err := ac.executeForward()
	if err == nil || !strings.Contains(err.Error(), "forward name is required") {
		t.Fatalf("expected missing forward name error, got: %v", err)
	}
}

func TestExecuteForward_TargetNodeRequired(t *testing.T) {
	ac := NewCommand()
	ac.ForwardName = "gpu-api"
	ac.TargetPort = 6443

	err := ac.executeForward()
	if err == nil || !strings.Contains(err.Error(), "--target-node is required") {
		t.Fatalf("expected missing target node error, got: %v", err)
	}
}

func TestExecuteForward_InvalidTargetPort(t *testing.T) {
	ac := NewCommand()
	ac.ForwardName = "gpu-api"
	ac.TargetPort = 70000
	ac.TargetNode = "node-gpu-1"

	err := ac.executeForward()
	if err == nil || !strings.Contains(err.Error(), "--target-port must be between 1 and 65535") {
		t.Fatalf("expected invalid target port error, got: %v", err)
	}
}

func TestFindSSHService_UsesOOBMgmtServerService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/service" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"svc-node","name":"node-ssh","simulation":"sim-1","dest_port":22,"src_port":19999,"service_type":"ssh","host":"node-worker.air.nvidia.com","node_name":"node-gpu-1"},
			{"id":"svc-bastion","name":"bastion-ssh","simulation":"sim-1","dest_port":22,"src_port":16821,"service_type":"ssh","host":"worker01.air.nvidia.com","node_name":"oob-mgmt-server"}
		]`))
	}))
	defer server.Close()

	ac := NewCommand()
	host, port, err := ac.findSSHService(api.NewClient(server.URL, "bearer-token"), "sim-1")
	if err != nil {
		t.Fatalf("findSSHService failed: %v", err)
	}
	if host != "worker01.air.nvidia.com" || port != 16821 {
		t.Fatalf("expected oob-mgmt-server SSH service, got %s:%d", host, port)
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
