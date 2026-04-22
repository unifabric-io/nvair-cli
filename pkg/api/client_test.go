package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetSSHKeys_Success tests successful retrieval of SSH keys.
func TestGetSSHKeys_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/ssh-keys/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.RawQuery != "limit=" {
			t.Errorf("query mismatch: got %q, want %q", r.URL.RawQuery, "limit=")
		}

		// Verify API token is present
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		resp := listSSHKeysResponse{
			Count: 1,
			Results: []GetSSHKeyResponse{
				{
					Created:     "2026-04-22T07:08:35.573313Z",
					ID:          "key-1",
					Name:        "my-key",
					Fingerprint: "abc123==",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	keys, err := client.GetSSHKeys()

	if err != nil {
		t.Fatalf("GetSSHKeys failed: %v", err)
	}

	if len(keys) != 1 {
		t.Errorf("Expected 1 key, got %d", len(keys))
	}

	if keys[0].ID != "key-1" {
		t.Errorf("Key ID mismatch: got %q, want %q", keys[0].ID, "key-1")
	}
}

// TestGetSSHKeys_NotFound tests 404 response (no keys).
func TestGetSSHKeys_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	keys, err := client.GetSSHKeys()

	if err != nil {
		t.Fatalf("GetSSHKeys failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("Expected 0 keys for 404, got %d", len(keys))
	}
}

// TestGetSSHKeys_Empty tests successful retrieval with empty list.
func TestGetSSHKeys_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	keys, err := client.GetSSHKeys()

	if err != nil {
		t.Fatalf("GetSSHKeys failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("Expected 0 keys, got %d", len(keys))
	}
}

// TestCreateSSHKey_Success tests successful key creation.
func TestCreateSSHKey_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/ssh-keys/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request
		var req CreateSSHKeyRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.PublicKey == "" || req.Name == "" {
			http.Error(w, "Missing fields", http.StatusBadRequest)
			return
		}

		resp := CreateSSHKeyResponse{
			ID:          "new-key-id",
			Name:        req.Name,
			Fingerprint: "xyz789==",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	keyResp, err := client.CreateSSHKey("ssh-ed25519 AAAA...", "my-key")

	if err != nil {
		t.Fatalf("CreateSSHKey failed: %v", err)
	}

	if keyResp.ID != "new-key-id" {
		t.Errorf("Key ID mismatch: got %q, want %q", keyResp.ID, "new-key-id")
	}
}

// TestCreateSSHKey_Conflict tests 409 Conflict response.
func TestCreateSSHKey_Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error": "Key already exists"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.CreateSSHKey("ssh-ed25519 AAAA...", "existing-key")

	if err == nil {
		t.Error("Expected error for conflict, got nil")
	}
}

// TestCreateSSHKey_CreatedEmptyBody tests 201 Created without a response body.
func TestCreateSSHKey_CreatedEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/ssh-keys/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	keyResp, err := client.CreateSSHKey("ssh-ed25519 AAAA...", "my-key")

	if err != nil {
		t.Fatalf("CreateSSHKey failed: %v", err)
	}
	if keyResp.Name != "my-key" {
		t.Errorf("Key name mismatch: got %q, want %q", keyResp.Name, "my-key")
	}
}

// TestDeleteSSHKey_Success tests the v3 users SSH key delete endpoint.
func TestDeleteSSHKey_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/ssh-keys/key-1/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	if err := client.DeleteSSHKey("key-1"); err != nil {
		t.Fatalf("DeleteSSHKey failed: %v", err)
	}
}

// TestRetryLogic_TransientFailure tests retry on 5xx errors.
func TestRetryLogic_TransientFailure(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.GetSSHKeys()

	if err != nil {
		t.Fatalf("GetSSHKeys failed after retries: %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}
}

// TestRetryLogic_PermanentFailure tests no retry on 4xx errors.
func TestRetryLogic_PermanentFailure(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.GetSSHKeys()

	if err == nil {
		t.Error("Expected error for bad request, got nil")
	}

	if attemptCount != 1 {
		t.Errorf("Expected 1 attempt (no retry for 4xx), got %d", attemptCount)
	}
}

// TestAPITokenInHeader tests that API token is properly included in requests.
func TestAPITokenInHeader(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-test-token")
	client.GetSSHKeys()

	expected := "Bearer my-test-token"
	if capturedAuth != expected {
		t.Errorf("Authorization header mismatch: got %q, want %q", capturedAuth, expected)
	}
}

// TestGetSimulations_Success tests retrieving simulations list.
func TestGetSimulations_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations" {
			http.NotFound(w, r)
			return
		}

		resp := ListSimulationsResponse{
			Count: 2,
			Results: []SimulationInfo{
				{
					ID:    "sim-1",
					Name:  "simple",
					State: "NEW",
				},
				{
					ID:    "sim-2",
					Name:  "complex",
					State: "READY",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	sims, err := client.GetSimulations()

	if err != nil {
		t.Fatalf("GetSimulations failed: %v", err)
	}

	if len(sims) != 2 {
		t.Errorf("Expected 2 simulations, got %d", len(sims))
	}

	if sims[0].Name != "simple" || sims[0].ID != "sim-1" {
		t.Errorf("First simulation mismatch: got %v", sims[0])
	}

	if sims[1].Name != "complex" || sims[1].ID != "sim-2" {
		t.Errorf("Second simulation mismatch: got %v", sims[1])
	}
}

// TestDeleteSimulation_ByName tests deleting a simulation by name.
// The deletion flow: list simulations -> find by name -> delete by ID
func TestDeleteSimulation_ByName(t *testing.T) {
	requestLog := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestLog = append(requestLog, r.Method+" "+r.URL.Path)

		if r.URL.Path == "/v3/simulations" && r.Method == "GET" {
			// List simulations response
			resp := ListSimulationsResponse{
				Count: 1,
				Results: []SimulationInfo{
					{
						ID:    "c51aed7b-febf-45fd-881d-83c373f9282f",
						Name:  "simple",
						State: "NEW",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/v3/simulations/c51aed7b-febf-45fd-881d-83c373f9282f/" && r.Method == "DELETE" {
			// Delete response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.DeleteSimulation("simple")

	if err != nil {
		t.Fatalf("DeleteSimulation failed: %v", err)
	}

	// Verify the request sequence
	if len(requestLog) != 2 {
		t.Errorf("Expected 2 requests (list then delete), got %d: %v", len(requestLog), requestLog)
	}

	if len(requestLog) >= 1 && requestLog[0] != "GET /v3/simulations" {
		t.Errorf("First request should be GET /v3/simulations, got %s", requestLog[0])
	}

	if len(requestLog) >= 2 && requestLog[1] != "DELETE /v3/simulations/c51aed7b-febf-45fd-881d-83c373f9282f/" {
		t.Errorf("Second request should be DELETE /v3/simulations/{id}/, got %s", requestLog[1])
	}
}

// TestDeleteSimulation_NotFound tests deletion when simulation doesn't exist.
func TestDeleteSimulation_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations" && r.Method == "GET" {
			// Empty list response
			resp := ListSimulationsResponse{
				Count:   0,
				Results: []SimulationInfo{},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.DeleteSimulation("nonexistent")

	if err == nil {
		t.Fatal("Expected error for nonexistent simulation, got nil")
	}

	if err.Error() != "simulation 'nonexistent' not found" {
		t.Errorf("Wrong error message: %v", err)
	}
}

// TestControlSimulation_Success tests setting simulation state.
func TestControlSimulation_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulation/sim-id-123/control/" && r.Method == "POST" {
			// Verify request body
			var reqBody map[string]string
			json.NewDecoder(r.Body).Decode(&reqBody)

			if reqBody["action"] != "load" {
				t.Errorf("Expected action 'load', got %q", reqBody["action"])
			}

			resp := ControlSimulationResponse{
				Result: "OK",
				Jobs:   []string{"job-1", "job-2"},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	resp, err := client.ControlSimulation("sim-id-123", "load")

	if err != nil {
		t.Fatalf("ControlSimulation failed: %v", err)
	}

	if resp.Result != "OK" {
		t.Errorf("Expected result 'OK', got %q", resp.Result)
	}

	if len(resp.Jobs) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(resp.Jobs))
	}

	if resp.Jobs[0] != "job-1" || resp.Jobs[1] != "job-2" {
		t.Errorf("Job list mismatch: got %v", resp.Jobs)
	}
}

// TestControlSimulation_Error tests error handling for control simulation.
func TestControlSimulation_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulation/invalid-id/control/" && r.Method == "POST" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "simulation not found"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.ControlSimulation("invalid-id", "load")

	if err == nil {
		t.Fatal("Expected error for invalid simulation, got nil")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("404")) {
		t.Errorf("Expected 404 error, got: %v", err)
	}
}

func TestGetServices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/nodes/interfaces/services/" || r.Method != "GET" {
			http.NotFound(w, r)
			return
		}

		if r.URL.Query().Get("simulation") != "sim-123" {
			http.Error(w, "invalid simulation query", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("limit") != "25" {
			http.Error(w, "invalid limit query", http.StatusBadRequest)
			return
		}

		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		resp := map[string]interface{}{
			"count":    2,
			"next":     nil,
			"previous": nil,
			"results": []map[string]interface{}{
				{
					"id":           "svc-1",
					"name":         "oob-mgmt-server SSH",
					"service_type": "SSH",
					"worker_fqdn":  "worker01.air.nvidia.com",
					"worker_port":  16821,
					"node_port":    22,
					"interface":    "if-out",
				},
				{
					"id":           "svc-2",
					"name":         "k8s-default-my-web-app-30080",
					"service_type": "TCP",
					"worker_fqdn":  "worker01.air.nvidia.com",
					"worker_port":  17922,
					"node_port":    30080,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	services, err := client.GetServices("sim-123")
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	if services[0].ServiceType != "SSH" {
		t.Fatalf("expected first service_type SSH, got %q", services[0].ServiceType)
	}
	if services[0].SrcPort != 16821 {
		t.Fatalf("expected first src_port 16821, got %d", services[0].SrcPort)
	}
	if services[0].DestPort != 22 {
		t.Fatalf("expected first dest_port 22, got %d", services[0].DestPort)
	}
	if services[0].Host != "worker01.air.nvidia.com" {
		t.Fatalf("expected first host to be normalized, got %q", services[0].Host)
	}
}

func TestGetServices_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations/nodes/interfaces/services/" && r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"simulation not found"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.GetServices("sim-404")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("404")) {
		t.Errorf("Expected 404 error, got: %v", err)
	}
}

func TestDeleteServiceByID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/service/svc-123/" && r.Method == "DELETE" {
			if r.Header.Get("Authorization") != "Bearer test-token" {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	if err := client.DeleteServiceByID("svc-123"); err != nil {
		t.Fatalf("DeleteServiceByID failed: %v", err)
	}
}

func TestDeleteServiceByID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/service/svc-404/" && r.Method == "DELETE" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"service not found"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.DeleteServiceByID("svc-404")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("not found")) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// TestGetNodes_Success tests retrieving nodes list.
func TestGetNodes_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations/nodes/" && r.Method == "GET" {
			resp := nodeListResponse{
				Results: []Node{
					{
						ID:    "node-1",
						Name:  "oob-mgmt-server",
						State: "UP",
					},
					{
						ID:    "node-2",
						Name:  "leaf01",
						State: "UP",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	nodes, err := client.GetNodes("test-simulation-id")

	if err != nil {
		t.Fatalf("GetNodes failed: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}

	if nodes[0].Name != "oob-mgmt-server" {
		t.Errorf("Expected first node to be 'oob-mgmt-server', got %q", nodes[0].Name)
	}

	if nodes[1].Name != "leaf01" {
		t.Errorf("Expected second node to be 'leaf01', got %q", nodes[1].Name)
	}
}

// TestGetNodeInterfaces_Success tests retrieving node interfaces.
func TestGetNodeInterfaces_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations/nodes/interfaces" && r.Method == "GET" {
			// Check query parameters
			q := r.URL.Query()
			if q.Get("simulation") != "sim-123" || q.Get("node") != "node-1" {
				http.Error(w, "Invalid query parameters", http.StatusBadRequest)
				return
			}

			interfaces := []Interface{
				{
					ID:         "intf-1",
					Name:       "eth0",
					Outbound:   true,
					LinkUp:     true,
					MacAddress: "aa:bb:cc:dd:ee:ff",
				},
				{
					ID:         "intf-2",
					Name:       "eth1",
					Outbound:   false,
					LinkUp:     true,
					MacAddress: "aa:bb:cc:dd:ee:00",
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(interfaceListResponse{Results: interfaces})
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	interfaces, err := client.GetNodeInterfaces("sim-123", "node-1")

	if err != nil {
		t.Fatalf("GetNodeInterfaces failed: %v", err)
	}

	if len(interfaces) != 2 {
		t.Errorf("Expected 2 interfaces, got %d", len(interfaces))
	}

	if !interfaces[0].Outbound {
		t.Errorf("Expected first interface to be outbound")
	}

	if interfaces[1].Outbound {
		t.Errorf("Expected second interface to not be outbound")
	}
}

// TestGetNodeInterfaces_Error tests error handling for node interfaces.
func TestGetNodeInterfaces_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/simulations/nodes/interfaces" && r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "node not found"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.GetNodeInterfaces("sim-123", "invalid-node")

	if err == nil {
		t.Fatal("Expected error for invalid node, got nil")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("404")) {
		t.Errorf("Expected 404 error, got: %v", err)
	}
}

// TestGetJob_Success tests retrieving job status.
func TestGetJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/jobs/job-123" && r.Method == "GET" {
			resp := Job{
				ID:          "job-123",
				Category:    "LOAD",
				State:       "COMPLETE",
				Created:     "2026-02-26T08:00:00Z",
				LastUpdated: "2026-02-26T08:05:00Z",
				Simulation:  "sim-456",
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	job, err := client.GetJob("job-123")

	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}

	if job.ID != "job-123" {
		t.Errorf("Expected job ID 'job-123', got %q", job.ID)
	}

	if job.State != "COMPLETE" {
		t.Errorf("Expected job state 'COMPLETE', got %q", job.State)
	}

	if job.Category != "LOAD" {
		t.Errorf("Expected job category 'LOAD', got %q", job.Category)
	}
}

// TestGetJob_Error tests error handling for missing job.
func TestGetJob_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/jobs/nonexistent" && r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "job not found"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.GetJob("nonexistent")

	if err == nil {
		t.Fatal("Expected error for nonexistent job, got nil")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("404")) {
		t.Errorf("Expected 404 error, got: %v", err)
	}
}
