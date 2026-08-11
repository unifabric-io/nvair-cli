package create

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	nodeutil "github.com/unifabric-io/nvair-cli/pkg/node"
)

func TestWaitForSimulationState_Success(t *testing.T) {
	origPollInterval := waitForSimulationStatePollInterval
	origMaxWaitTime := waitForSimulationStateMaxWaitTime
	waitForSimulationStatePollInterval = 10 * time.Millisecond
	waitForSimulationStateMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForSimulationStatePollInterval = origPollInterval
		waitForSimulationStateMaxWaitTime = origMaxWaitTime
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/sim-123/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		state := "REQUESTING"
		if atomic.AddInt32(&calls, 1) >= 2 {
			state = "ACTIVE"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      "sim-123",
			"name":    "demo",
			"state":   state,
			"created": "2026-05-09T00:00:00Z",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	if err := cmd.WaitForSimulationState(client, "sim-123", "ACTIVE"); err != nil {
		t.Fatalf("WaitForSimulationState failed: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForNodeManagementIPs_WaitsUntilAssigned(t *testing.T) {
	origPollInterval := waitForNodeManagementIPsPollInterval
	origMaxWaitTime := waitForNodeManagementIPsMaxWaitTime
	waitForNodeManagementIPsPollInterval = 10 * time.Millisecond
	waitForNodeManagementIPsMaxWaitTime = 500 * time.Millisecond
	defer func() {
		waitForNodeManagementIPsPollInterval = origPollInterval
		waitForNodeManagementIPsMaxWaitTime = origMaxWaitTime
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v3/simulations/nodes/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		mgmtIP := ""
		if atomic.AddInt32(&calls, 1) >= 3 {
			mgmtIP = "10.0.0.5"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{
					"id":            "switch-1",
					"name":          "switch-gpu-leaf1",
					"state":         "ACTIVE",
					"image":         "cumulus-vx",
					"management_ip": mgmtIP,
				},
			},
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	initialNodes := []api.Node{
		{ID: "switch-1", Name: "switch-gpu-leaf1", Image: "cumulus-vx", ManagementIP: ""},
	}

	nodes, err := cmd.WaitForNodeManagementIPs(client, "sim-123", initialNodes)
	if err != nil {
		t.Fatalf("WaitForNodeManagementIPs failed: %v", err)
	}

	if len(nodes) != 1 || nodes[0].ManagementIP != "10.0.0.5" {
		t.Fatalf("expected refreshed node with management IP, got %+v", nodes)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForNodeManagementIPs_WaitsForGPUNodesRegardlessOfImageName(t *testing.T) {
	// Regression test: GPU/compute nodes resolve to image names that don't
	// contain "cumulus" or "generic" (e.g. "ubuntu-24.04"), so the wait must
	// not be scoped to those substrings or it will return prematurely.
	origPollInterval := waitForNodeManagementIPsPollInterval
	origMaxWaitTime := waitForNodeManagementIPsMaxWaitTime
	waitForNodeManagementIPsPollInterval = 10 * time.Millisecond
	waitForNodeManagementIPsMaxWaitTime = 500 * time.Millisecond
	defer func() {
		waitForNodeManagementIPsPollInterval = origPollInterval
		waitForNodeManagementIPsMaxWaitTime = origMaxWaitTime
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v3/simulations/nodes/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		mgmtIP := ""
		if atomic.AddInt32(&calls, 1) >= 3 {
			mgmtIP = "10.0.0.9"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{
					"id":            "node-gpu-1",
					"name":          "node-gpu-1",
					"state":         "ACTIVE",
					"image":         "ubuntu-24.04",
					"management_ip": mgmtIP,
				},
				{
					"id":            "oob-1",
					"name":          "oob-mgmt-server",
					"state":         "ACTIVE",
					"image":         "ubuntu-24.04",
					"management_ip": "",
				},
			},
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	initialNodes := []api.Node{
		{ID: "node-gpu-1", Name: "node-gpu-1", Image: "ubuntu-24.04", ManagementIP: ""},
		{ID: "oob-1", Name: "oob-mgmt-server", Image: "ubuntu-24.04", ManagementIP: ""},
	}

	nodes, err := cmd.WaitForNodeManagementIPs(client, "sim-123", initialNodes)
	if err != nil {
		t.Fatalf("WaitForNodeManagementIPs failed: %v", err)
	}

	var gpuNode *api.Node
	for i := range nodes {
		if nodes[i].Name == "node-gpu-1" {
			gpuNode = &nodes[i]
		}
	}
	if gpuNode == nil || gpuNode.ManagementIP != "10.0.0.9" {
		t.Fatalf("expected node-gpu-1 to have a resolved management IP, got %+v", nodes)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

// TestWaitForNodeManagementIPs_ResolvesFromManagementInterfaces is a
// regression test for the real API response shape observed in production,
// where nodes have no top-level "management_ip" and a null "metadata", and
// instead expose their management IP only under
// management_interfaces.<ifname>.ip (e.g. {"eth0": {"ip": "192.168.200.1"}}).
// Without support for this shape, ResolveMgmtIP always returns "" and the
// wait times out even though the backend has already assigned IPs.
func TestWaitForNodeManagementIPs_ResolvesFromManagementInterfaces(t *testing.T) {
	origPollInterval := waitForNodeManagementIPsPollInterval
	origMaxWaitTime := waitForNodeManagementIPsMaxWaitTime
	waitForNodeManagementIPsPollInterval = 10 * time.Millisecond
	waitForNodeManagementIPsMaxWaitTime = 500 * time.Millisecond
	defer func() {
		waitForNodeManagementIPsPollInterval = origPollInterval
		waitForNodeManagementIPsMaxWaitTime = origMaxWaitTime
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v3/simulations/nodes/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"switch-1","name":"switch-gpu-leaf1","state":"ACTIVE","image":"cumulus-vx","metadata":null,"management_interfaces":{"eth0":{"ip":"192.168.200.111","mac_address":"48:B0:2D:00:00:00"}}},
			{"id":"oob-1","name":"oob-mgmt-server","state":"ACTIVE","image":"ubuntu-24.04","metadata":null,"management_interfaces":{"eth1":{"ip":"192.168.200.1"}}}
		]}`))
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	initialNodes := []api.Node{
		{ID: "switch-1", Name: "switch-gpu-leaf1", Image: "cumulus-vx"},
		{ID: "oob-1", Name: "oob-mgmt-server", Image: "ubuntu-24.04"},
	}

	nodes, err := cmd.WaitForNodeManagementIPs(client, "sim-123", initialNodes)
	if err != nil {
		t.Fatalf("WaitForNodeManagementIPs failed: %v", err)
	}

	var switchNode *api.Node
	for i := range nodes {
		if nodes[i].Name == "switch-gpu-leaf1" {
			switchNode = &nodes[i]
		}
	}
	if switchNode == nil {
		t.Fatalf("expected switch-gpu-leaf1 in returned nodes, got %+v", nodes)
	}
	gotIP, err := nodeutil.ResolveMgmtIP(*switchNode)
	if err != nil {
		t.Fatalf("ResolveMgmtIP failed: %v", err)
	}
	if gotIP != "192.168.200.111" {
		t.Fatalf("expected management IP resolved from management_interfaces, got %q", gotIP)
	}
}

func TestWaitForNodeManagementIPs_SkipsOOBMgmtSwitchNodes(t *testing.T) {
	// Regression test: oob-mgmt-switch* nodes (e.g. "oob-mgmt-switch-leaf-1")
	// commonly have no management IP assigned (management_interfaces.eth0.ip
	// is null in real API responses), so the wait must not require them to
	// have one, or it will time out even though every other node is ready.
	origPollInterval := waitForNodeManagementIPsPollInterval
	origMaxWaitTime := waitForNodeManagementIPsMaxWaitTime
	waitForNodeManagementIPsPollInterval = 10 * time.Millisecond
	waitForNodeManagementIPsMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForNodeManagementIPsPollInterval = origPollInterval
		waitForNodeManagementIPsMaxWaitTime = origMaxWaitTime
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v3/simulations/nodes/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"switch-1","name":"switch-gpu-leaf1","state":"ACTIVE","image":"cumulus-vx","metadata":null,"management_interfaces":{"eth0":{"ip":"192.168.200.111"}}},
			{"id":"oob-1","name":"oob-mgmt-server","state":"ACTIVE","image":"ubuntu-24.04","metadata":null,"management_interfaces":{"eth1":{"ip":"192.168.200.1"}}},
			{"id":"oob-2","name":"oob-mgmt-switch-leaf-1","state":"ACTIVE","image":"cumulus-vx","metadata":null,"management_interfaces":{"eth0":{"ip":null}}}
		]}`))
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	initialNodes := []api.Node{
		{ID: "switch-1", Name: "switch-gpu-leaf1", Image: "cumulus-vx"},
		{ID: "oob-1", Name: "oob-mgmt-server", Image: "ubuntu-24.04"},
		{ID: "oob-2", Name: "oob-mgmt-switch-leaf-1", Image: "cumulus-vx"},
	}

	if _, err := cmd.WaitForNodeManagementIPs(client, "sim-123", initialNodes); err != nil {
		t.Fatalf("expected wait to succeed while oob-mgmt-switch-leaf-1 has no management IP, got: %v", err)
	}
}

func TestWaitForNodeManagementIPs_Timeout(t *testing.T) {
	origPollInterval := waitForNodeManagementIPsPollInterval
	origMaxWaitTime := waitForNodeManagementIPsMaxWaitTime
	waitForNodeManagementIPsPollInterval = 5 * time.Millisecond
	waitForNodeManagementIPsMaxWaitTime = 30 * time.Millisecond
	defer func() {
		waitForNodeManagementIPsPollInterval = origPollInterval
		waitForNodeManagementIPsMaxWaitTime = origMaxWaitTime
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{
					"id":            "switch-1",
					"name":          "switch-gpu-leaf1",
					"state":         "ACTIVE",
					"image":         "cumulus-vx",
					"management_ip": "",
				},
			},
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	initialNodes := []api.Node{
		{ID: "switch-1", Name: "switch-gpu-leaf1", Image: "cumulus-vx", ManagementIP: ""},
	}

	_, err := cmd.WaitForNodeManagementIPs(client, "sim-123", initialNodes)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout waiting for management IPs") {
		t.Fatalf("expected timeout error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "switch-gpu-leaf1") {
		t.Fatalf("expected error to mention missing node name, got: %v", err)
	}
}

func TestWaitForSimulationState_ReachesInactive(t *testing.T) {
	origPollInterval := waitForSimulationStatePollInterval
	origMaxWaitTime := waitForSimulationStateMaxWaitTime
	waitForSimulationStatePollInterval = 10 * time.Millisecond
	waitForSimulationStateMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForSimulationStatePollInterval = origPollInterval
		waitForSimulationStateMaxWaitTime = origMaxWaitTime
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/sim-123/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		state := "IMPORTING"
		if atomic.AddInt32(&calls, 1) >= 2 {
			state = "INACTIVE"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      "sim-123",
			"name":    "demo",
			"state":   state,
			"created": "2026-05-09T00:00:00Z",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	if err := cmd.WaitForSimulationState(client, "sim-123", "INACTIVE"); err != nil {
		t.Fatalf("WaitForSimulationState failed: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForSimulationState_InvalidStateFails(t *testing.T) {
	origPollInterval := waitForSimulationStatePollInterval
	origMaxWaitTime := waitForSimulationStateMaxWaitTime
	waitForSimulationStatePollInterval = 10 * time.Millisecond
	waitForSimulationStateMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForSimulationStatePollInterval = origPollInterval
		waitForSimulationStateMaxWaitTime = origMaxWaitTime
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/sim-123/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      "sim-123",
			"name":    "demo",
			"state":   "INVALID",
			"created": "2026-05-09T00:00:00Z",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	err := cmd.WaitForSimulationState(client, "sim-123", "ACTIVE")
	if err == nil {
		t.Fatal("expected WaitForSimulationState to fail for INVALID state")
	}
	if !strings.Contains(err.Error(), "INVALID") {
		t.Fatalf("expected INVALID state error, got %v", err)
	}
}
