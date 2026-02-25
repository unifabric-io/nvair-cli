package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

// TestCreateCommand_ValidTopology tests creating a simulation with a valid topology
func TestCreateCommand_ValidTopology(t *testing.T) {
	// Create a CreateCommand instance
	createCmd := NewCreateCommand()
	createCmd.Directory = "../topology"
	createCmd.DryRun = true
	createCmd.Verbose = false

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute the command
	err := createCmd.Execute()

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	output, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	outputStr := string(output)
	if !bytes.Contains(output, []byte("Topology validation passed")) {
		t.Errorf("Expected success message, got: %s", outputStr)
	}
}

// TestCreateCommand_MissingDirectory tests with non-existent directory
func TestCreateCommand_MissingDirectory(t *testing.T) {
	createCmd := NewCreateCommand()
	createCmd.Directory = "/nonexistent/path"
	createCmd.DryRun = true

	err := createCmd.Execute()
	if err == nil {
		t.Fatalf("Expected error for missing directory, got nil")
	}
}

// TestCreateCommand_NoDirectoryFlag tests without required directory flag
func TestCreateCommand_NoDirectoryFlag(t *testing.T) {
	createCmd := NewCreateCommand()
	createCmd.Directory = ""
	createCmd.DryRun = true

	err := createCmd.Execute()
	if err == nil {
		t.Fatalf("Expected error for missing directory flag, got nil")
	}
}

// TestCreateCommand_DryRun tests that dry-run doesn't create simulation
func TestCreateCommand_DryRun(t *testing.T) {
	createCmd := NewCreateCommand()
	createCmd.Directory = "../topology"
	createCmd.DryRun = true

	err := createCmd.Execute()
	if err != nil {
		t.Fatalf("Dry-run should not fail, got: %v", err)
	}
}

// TestDeleteCommand_InvalidResourceType tests delete command with invalid resource type
func TestDeleteCommand_InvalidResourceType(t *testing.T) {
	deleteCmd := NewDeleteCommand()

	err := deleteCmd.Execute([]string{"invalid", "name"})
	if err == nil {
		t.Fatalf("Expected error for invalid resource type, got nil")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("invalid resource type")) {
		t.Errorf("Expected 'invalid resource type' error message, got: %v", err)
	}
}

// TestDeleteCommand_MissingArgs tests delete command with missing arguments
func TestDeleteCommand_MissingArgs(t *testing.T) {
	deleteCmd := NewDeleteCommand()

	err := deleteCmd.Execute([]string{})
	if err == nil {
		t.Fatalf("Expected error for missing arguments, got nil")
	}
}

// TestTopologyLoading_Integration tests topology loading in integration context
func TestTopologyLoading_Integration(t *testing.T) {
	// Load topology file
	topo, err := topology.LoadTopologyFromPath("../topology/valid_topology.json")
	if err != nil {
		t.Fatalf("Failed to load topology: %v", err)
	}

	// Validate it
	result := topology.ValidateTopology(topo)
	if !result.Valid {
		t.Fatalf("Valid topology failed validation: %v", result.Errors)
	}

	// Check structure - RawTopology uses Title and Content fields
	if topo.Title == "" {
		t.Errorf("Expected non-empty topology title")
	}

	if len(topo.Content.Nodes) == 0 {
		t.Errorf("Expected at least one node in content")
	}
}

// TestConfiguration_TokenExpired tests the token expiration check
func TestConfiguration_TokenExpired(t *testing.T) {
	// Create a config with expired token
	expiredCfg := &config.Config{
		Username:             "test@example.com",
		BearerToken:          "test-token",
		BearerTokenExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}

	// Check that the token is marked as expired
	if !expiredCfg.IsTokenExpired(time.Now()) {
		t.Errorf("Expected token to be expired")
	}

	// Create a config with valid token
	validCfg := &config.Config{
		Username:             "test@example.com",
		BearerToken:          "test-token",
		BearerTokenExpiresAt: time.Now().Add(23 * time.Hour), // Valid for 23 more hours
	}

	// Check that the token is NOT marked as expired
	if validCfg.IsTokenExpired(time.Now()) {
		t.Errorf("Expected token to be valid")
	}
}

// TestConfiguration_TokenRefreshScenario tests token refresh scenarios
func TestConfiguration_TokenRefreshScenario(t *testing.T) {
	// Simulate a config that would need refresh
	expiredCfg := &config.Config{
		Username:             "test@example.com",
		APIToken:             "saved-api-token", // This would be used for refresh
		BearerToken:          "old-bearer-token",
		BearerTokenExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
	}

	// Check that token is expired
	if !expiredCfg.IsTokenExpired(time.Now()) {
		t.Fatalf("Test setup error: token should be expired")
	}

	// Check that we have the necessary fields for refresh
	if expiredCfg.APIToken == "" {
		t.Errorf("Expected API token to be set for refresh")
	}

	if expiredCfg.Username == "" {
		t.Errorf("Expected username to be set for refresh")
	}

	// Simulate successful refresh - update token and expiration
	newBearerToken := "new-bearer-token"
	newExpiresAt := time.Now().Add(24 * time.Hour)

	expiredCfg.BearerToken = newBearerToken
	expiredCfg.BearerTokenExpiresAt = newExpiresAt

	// Verify the token is now valid
	if expiredCfg.IsTokenExpired(time.Now()) {
		t.Errorf("Expected token to be valid after refresh")
	}

	// Verify the token was updated
	if expiredCfg.BearerToken != newBearerToken {
		t.Errorf("Expected bearer token to be updated")
	}
}

// TestDeleteCommand_BasicValidation tests basic validation of the delete command
func TestDeleteCommand_BasicValidation(t *testing.T) {
	deleteCmd := NewDeleteCommand()

	// Test that missing args returns error
	err := deleteCmd.Execute([]string{})
	if err == nil {
		t.Fatalf("Expected error for missing arguments, got nil")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("usage")) {
		t.Errorf("Expected 'usage' in error message, got: %v", err)
	}

	// Test that invalid resource type returns error
	err = deleteCmd.Execute([]string{"invalid", "name"})
	if err == nil {
		t.Fatalf("Expected error for invalid resource type, got nil")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("invalid resource type")) {
		t.Errorf("Expected 'invalid resource type' in error message, got: %v", err)
	}
}

// TestWaitForJobs_AllComplete tests waiting for jobs that all complete successfully.
func TestWaitForJobs_AllComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/jobs/job-1" && r.Method == "GET" {
			resp := api.Job{
				ID:    "job-1",
				State: "COMPLETE",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/v2/jobs/job-2" && r.Method == "GET" {
			resp := api.Job{
				ID:    "job-2",
				State: "COMPLETE",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	createCmd := NewCreateCommand()
	createCmd.APIEndpoint = server.URL

	apiClient := api.NewClient(server.URL, "test-token")
	err := createCmd.WaitForJobs(apiClient, []string{"job-1", "job-2"})

	if err != nil {
		t.Fatalf("WaitForJobs should succeed when all jobs complete, got error: %v", err)
	}
}

// TestWaitForJobs_JobFailed tests waiting for jobs when one fails.
func TestWaitForJobs_JobFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/jobs/job-1" && r.Method == "GET" {
			resp := api.Job{
				ID:    "job-1",
				State: "FAILED",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	createCmd := NewCreateCommand()
	createCmd.APIEndpoint = server.URL

	apiClient := api.NewClient(server.URL, "test-token")
	err := createCmd.WaitForJobs(apiClient, []string{"job-1"})

	if err == nil {
		t.Fatalf("WaitForJobs should fail when a job fails")
	}

	if !bytes.Contains([]byte(err.Error()), []byte("failed")) {
		t.Errorf("Error should mention job failure, got: %v", err)
	}
}

// TestWaitForJobs_EmptyJobList tests waiting with empty job list.
func TestWaitForJobs_EmptyJobList(t *testing.T) {
	createCmd := NewCreateCommand()
	apiClient := api.NewClient("http://localhost:8000", "test-token")

	err := createCmd.WaitForJobs(apiClient, []string{})

	if err != nil {
		t.Fatalf("WaitForJobs should succeed with empty job list, got error: %v", err)
	}
}
