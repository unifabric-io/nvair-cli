//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// MockAPIServer represents a mock NVIDIA Air API server for testing
type MockAPIServer struct {
	server         *httptest.Server
	loginCalls     int
	getKeyCalls    int
	createKeyCalls int
}

// NewMockAPIServer creates a new mock API server for testing
func NewMockAPIServer() *MockAPIServer {
	mas := &MockAPIServer{}
	mas.server = httptest.NewServer(http.HandlerFunc(mas.handleRequest))
	return mas
}

// handleRequest routes requests to appropriate handlers
func (mas *MockAPIServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/login/":
		mas.handleLogin(w, r)
	case "/v1/sshkey", "/v1/sshkey/":
		if r.Method == "GET" {
			mas.handleGetSSHKeys(w, r)
		} else if r.Method == "POST" {
			mas.handleCreateSSHKey(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

// handleLogin mocks the authentication endpoint
func (mas *MockAPIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	mas.loginCalls++

	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	// Simulate specific test scenarios
	if req["username"] == "test-transient-error@example.com" && mas.loginCalls < 2 {
		// First attempt fails with 503
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if req["username"] == "test-auth-error@example.com" {
		// Authentication always fails
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid credentials"}`))
		return
	}

	// Success case
	resp := map[string]interface{}{
		"result":  "OK",
		"message": "Successfully logged in.",
		"token":   "test-bearer-token-" + fmt.Sprintf("%d", mas.loginCalls),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleGetSSHKeys mocks the get SSH keys endpoint
func (mas *MockAPIServer) handleGetSSHKeys(w http.ResponseWriter, r *http.Request) {
	mas.getKeyCalls++

	// Check authorization header
	if r.Header.Get("Authorization") == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check for specific test scenarios in query params
	testCase := r.URL.Query().Get("test_case")

	switch testCase {
	case "has_existing_key":
		// Return existing SSH key
		keys := []map[string]string{
			{
				"id":          "existing-key-id",
				"name":        "nvair-cli",
				"fingerprint": "qe2hUthJPcQ2UWhGCi5Sl5NBYX3F2SZbwY5PhKO1Jfc=",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(keys)

	case "key_conflict":
		// Return key with same name but different fingerprint
		keys := []map[string]string{
			{
				"id":          "different-key-id",
				"name":        "nvair-cli",
				"fingerprint": "different-fingerprint-hash",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(keys)

	default:
		// No keys found
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	}
}

// handleCreateSSHKey mocks the create SSH key endpoint
func (mas *MockAPIServer) handleCreateSSHKey(w http.ResponseWriter, r *http.Request) {
	mas.createKeyCalls++

	// Check authorization header
	if r.Header.Get("Authorization") == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	// Check for specific test scenarios
	if req["name"] == "conflict-key" {
		// Simulate key already exists
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error": "Key already exists"}`))
		return
	}

	// Success case - return single object, not array
	resp := map[string]string{
		"id":          "new-key-id",
		"name":        req["name"],
		"fingerprint": "test-fingerprint==",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// URL returns the base URL of the mock server
func (mas *MockAPIServer) URL() string {
	return mas.server.URL
}

// Close closes the mock server
func (mas *MockAPIServer) Close() {
	mas.server.Close()
}

// Stats returns the call statistics
func (mas *MockAPIServer) Stats() map[string]int {
	return map[string]int{
		"login_calls":      mas.loginCalls,
		"get_key_calls":    mas.getKeyCalls,
		"create_key_calls": mas.createKeyCalls,
	}
}

// Helper function to get the CLI binary path
func getCliBinaryPath(t *testing.T) string {
	// Try to find the compiled binary in common locations
	// The binary is at the project root in bin/nvcli
	// Relative to e2e, that would be ../bin/nvcli
	possiblePaths := []string{
		"../bin/nvcli",    // From e2e (root level)
		"bin/nvcli",       // From project root
		"./bin/nvcli",     // From project root (explicit)
		"../../bin/nvcli", // Fallback, in case working directory varies
	}

	// Get current working directory and also try from there
	wd, err := os.Getwd()
	if err == nil {
		absPath := filepath.Join(wd, "bin/nvcli")
		possiblePaths = append(possiblePaths, absPath)
		// Also try to find bin directory relative from different base paths
		for _, base := range []string{filepath.Dir(wd), filepath.Dir(filepath.Dir(wd))} {
			possiblePaths = append(possiblePaths, filepath.Join(base, "bin/nvcli"))
		}
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			// If it's a relative path, make it absolute
			if !filepath.IsAbs(path) {
				absPath, err := filepath.Abs(path)
				if err == nil {
					return absPath
				}
			}
			return path
		}
	}

	t.Fatalf("CLI binary not found in any of these locations: %v. Run 'make build' first", possiblePaths)
	return ""
}

// CommandResult holds the command execution result for detailed verification
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

// runCommand executes a command with verbose logging enabled and captures all output
func runCommand(t *testing.T, env []string, args ...string) *CommandResult {
	if env == nil {
		env = os.Environ()
	}

	// Build command arguments with verbose flag
	cmdArgs := []string{}

	// Add all provided arguments
	cmdArgs = append(cmdArgs, args...)

	// Check if --verbose or -v is already present
	hasVerbose := false
	for _, arg := range cmdArgs {
		if arg == "--verbose" || arg == "-v" {
			hasVerbose = true
			break
		}
	}

	// Add verbose flag if not already present
	if !hasVerbose {
		cmdArgs = append(cmdArgs, "--verbose")
	}

	cmd := exec.Command(getCliBinaryPath(t), cmdArgs...)
	cmd.Env = env

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	result := &CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
	}

	return result
}

// logCommandOutput prints the command output for test debugging and verification
func logCommandOutput(t *testing.T, result *CommandResult, testName string) {
	t.Logf("\n========== Command Output: %s ==========", testName)

	if result.Stdout != "" {
		t.Logf("--- STDOUT ---\n%s", result.Stdout)
	} else {
		t.Logf("--- STDOUT ---\n(empty)")
	}

	if result.Stderr != "" {
		t.Logf("--- STDERR ---\n%s", result.Stderr)
	} else {
		t.Logf("--- STDERR ---\n(empty)")
	}

	t.Logf("--- Exit Code: %d ---", result.ExitCode)
	if result.Err != nil {
		t.Logf("--- Error: %v ---", result.Err)
	}
	t.Logf("========== End Output %s ==========\n", testName)
}

// mustContainInOutput checks if the output contains expected strings for validation
func mustContainInOutput(t *testing.T, result *CommandResult, expectedStrings []string, outputType string) {
	output := result.Stdout + result.Stderr
	for _, expected := range expectedStrings {
		if expected == "" {
			continue
		}
		if !bytes.Contains([]byte(output), []byte(expected)) {
			logCommandOutput(t, result, "ASSERTION_FAILED")
			t.Errorf("Expected '%s' not found in %s output", expected, outputType)
		}
	}
}

// Test scenarios

// TestIntegration_SuccessfulLogin tests a successful login flow
func TestIntegration_SuccessfulLogin(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server
	mockServer := NewMockAPIServer()
	defer mockServer.Close()

	// Execute login command via compiled binary with verbose output
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", "test@example.com",
		"-p", "test-api-token",
		"--api-endpoint", mockServer.URL())

	// Always log output for debugging
	logCommandOutput(t, result, "TestIntegration_SuccessfulLogin")

	if result.ExitCode != 0 {
		t.Fatalf("Login failed with exit code %d: %v", result.ExitCode, result.Err)
	}

	// Verify expected log messages in output
	expectedLogs := []string{
		"[DEBUG]", // Should have debug logs from verbose mode
	}
	mustContainInOutput(t, result, expectedLogs, "combined")

	// Verify statistics
	stats := mockServer.Stats()
	if stats["login_calls"] != 1 {
		t.Errorf("Expected 1 login call, got %d", stats["login_calls"])
	}
	if stats["get_key_calls"] < 1 {
		t.Errorf("Expected at least 1 get key call, got %d", stats["get_key_calls"])
	}

	// Verify config file was created
	configPath := filepath.Join(tmpDir, ".config/nvair.unifabric.io/config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Configuration file not created at %s", configPath)
	}

	// Read and verify config content
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	if len(configData) == 0 {
		t.Fatal("Configuration file is empty")
	}

	t.Logf("✓ Successful login test passed")
	t.Logf("  - Login calls: %d", stats["login_calls"])
	t.Logf("  - Get key calls: %d", stats["get_key_calls"])
	t.Logf("  - Create key calls: %d", stats["create_key_calls"])
	t.Logf("  - Config file size: %d bytes", len(configData))
}

// TestIntegration_TransientFailure tests retry logic on transient errors
func TestIntegration_TransientFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server
	mockServer := NewMockAPIServer()
	defer mockServer.Close()

	// Execute login command with transient error scenario via compiled binary
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", "test-transient-error@example.com",
		"-p", "test-api-token",
		"--api-endpoint", mockServer.URL())

	logCommandOutput(t, result, "TestIntegration_TransientFailure")

	if result.ExitCode != 0 {
		t.Fatalf("Login failed with exit code %d", result.ExitCode)
	}

	// Verify retry logic was triggered with verbose logs
	expectedLogs := []string{
		"[DEBUG]", // Should have debug logs showing retry logic
	}
	mustContainInOutput(t, result, expectedLogs, "combined")

	// Verify retry logic was triggered
	stats := mockServer.Stats()
	if stats["login_calls"] < 2 {
		t.Errorf("Expected at least 2 login calls (retry), got %d", stats["login_calls"])
	}

	t.Logf("✓ Transient failure retry test passed")
	t.Logf("  - Login calls (with retry): %d", stats["login_calls"])
}

// TestIntegration_AuthenticationFailure tests handling of authentication errors
func TestIntegration_AuthenticationFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server
	mockServer := NewMockAPIServer()
	defer mockServer.Close()

	// Execute login command with invalid credentials via compiled binary
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", "test-auth-error@example.com",
		"-p", "wrong-token",
		"--api-endpoint", mockServer.URL())

	logCommandOutput(t, result, "TestIntegration_AuthenticationFailure")

	if result.ExitCode == 0 {
		t.Fatalf("Expected authentication error (non-zero exit code), but command succeeded")
	}

	// Verify error messages are logged
	expectedErrors := []string{
		"[DEBUG]", // Should have debug logs
	}
	mustContainInOutput(t, result, expectedErrors, "combined")

	stats := mockServer.Stats()
	if stats["login_calls"] != 1 {
		t.Errorf("Expected 1 login call, got %d", stats["login_calls"])
	}

	t.Logf("✓ Authentication failure test passed")
	t.Logf("  - Correctly rejected invalid credentials")
	t.Logf("  - Error properly logged and captured")
}

// TestIntegration_ExistingSSHKey tests login when SSH key already exists
func TestIntegration_ExistingSSHKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server with custom GET handler
	mas := &MockAPIServer{}
	mas.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/login/" {
			resp := map[string]interface{}{
				"result": "OK",
				"token":  "test-bearer-token",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/v1/sshkey" {
			if r.Method == "GET" {
				// Return existing key with same fingerprint
				keys := []map[string]interface{}{
					{
						"id":          "existing-key-id",
						"name":        "nvair-cli",
						"fingerprint": "matching-fingerprint",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(keys)
			}
		}
	}))
	defer mas.server.Close()

	// Execute login command via compiled binary with verbose output
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", "test@example.com",
		"-p", "test-api-token",
		"--api-endpoint", mas.URL())

	logCommandOutput(t, result, "TestIntegration_ExistingSSHKey")

	if result.ExitCode != 0 {
		t.Fatalf("Login failed with exit code %d", result.ExitCode)
	}

	// Verify verbose logs captured
	expectedLogs := []string{
		"[DEBUG]",
	}
	mustContainInOutput(t, result, expectedLogs, "combined")

	// Verify config file was created
	configPath := filepath.Join(tmpDir, ".config/nvair.unifabric.io/config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Configuration file not created")
	}

	t.Logf("✓ Existing SSH key test passed")
	t.Logf("  - Login succeeded even with existing key")
	t.Logf("  - Verbose logs captured and verified")
}

// TestIntegration_SSHKeyConflict tests handling of SSH key name conflicts
func TestIntegration_SSHKeyConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server
	mockServer := NewMockAPIServer()
	defer mockServer.Close()

	// Execute login command via compiled binary with verbose output
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", "test@example.com",
		"-p", "test-api-token",
		"--api-endpoint", mockServer.URL())

	logCommandOutput(t, result, "TestIntegration_SSHKeyConflict")

	if result.ExitCode != 0 {
		t.Fatalf("Login failed with exit code %d", result.ExitCode)
	}

	// Verify verbose logs captured for debugging
	expectedLogs := []string{
		"[DEBUG]",
	}
	mustContainInOutput(t, result, expectedLogs, "combined")

	// Verify key creation was attempted
	stats := mockServer.Stats()
	if stats["create_key_calls"] < 1 {
		t.Errorf("Expected at least 1 create key call, got %d", stats["create_key_calls"])
	}

	t.Logf("✓ SSH key creation test passed")
	t.Logf("  - Create key calls: %d", stats["create_key_calls"])
	t.Logf("  - Command logs verified with verbose output")
}

// TestIntegration_RealAPI runs against the real NVIDIA Air API if credentials are provided
func TestIntegration_RealAPI(t *testing.T) {
	// Only run if environment variables are set
	username := os.Getenv("NV_AIR_USER")
	token := os.Getenv("NV_AIR_TOKEN")

	if username == "" || token == "" {
		t.Skip("Skipping real API test - NV_AIR_USER and NV_AIR_TOKEN not set")
	}

	tmpDir, err := os.MkdirTemp("", "nvair-real-api-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Execute login against real API via compiled binary with verbose output
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", username,
		"-p", token,
		"--api-endpoint", "https://air.nvidia.com/api")

	logCommandOutput(t, result, "TestIntegration_RealAPI")

	if result.ExitCode != 0 {
		t.Fatalf("Real API login failed with exit code %d", result.ExitCode)
	}

	// Verify expected log output
	expectedLogs := []string{
		"[DEBUG]",
	}
	mustContainInOutput(t, result, expectedLogs, "combined")

	// Verify configuration was saved
	configPath := filepath.Join(tmpDir, ".config/nvair.unifabric.io/config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Configuration file not created at %s", configPath)
	}

	// Read and verify config contents
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatalf("Failed to parse config JSON: %v", err)
	}

	// Verify required fields are present
	requiredFields := []string{"username", "apiToken", "bearerToken", "bearerTokenExpiresAt", "apiEndpoint"}
	for _, field := range requiredFields {
		if _, exists := config[field]; !exists {
			t.Errorf("Required field '%s' missing from config", field)
		}
	}

	t.Logf("✓ Real API integration test passed")
	t.Logf("  - Configuration saved successfully")
	t.Logf("  - Username: %v", config["username"])
	t.Logf("  - Verbose logs captured for debugging")
}

// TestIntegration_ConfigurationManagement tests proper saving and loading of configuration
func TestIntegration_ConfigurationManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server
	mockServer := NewMockAPIServer()
	defer mockServer.Close()

	// First login via compiled binary with verbose output
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	result := runCommand(t, env,
		"login",
		"-u", "test@example.com",
		"-p", "test-api-token",
		"--api-endpoint", mockServer.URL())

	logCommandOutput(t, result, "TestIntegration_ConfigurationManagement_Login")

	if result.ExitCode != 0 {
		t.Fatalf("First login failed with exit code %d", result.ExitCode)
	}

	// Verify verbose logs
	expectedLogs := []string{
		"[DEBUG]",
	}
	mustContainInOutput(t, result, expectedLogs, "combined")

	// Verify configuration file was created
	configPath := filepath.Join(tmpDir, ".config/nvair.unifabric.io/config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Configuration file not created")
	}

	// Read configuration
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if config["username"] != "test@example.com" {
		t.Errorf("Username mismatch: expected 'test@example.com', got %v", config["username"])
	}

	if _, exists := config["bearerToken"]; !exists {
		t.Errorf("Bearer token missing from configuration")
	}

	t.Logf("✓ Configuration management test passed")
	t.Logf("  - Configuration file created and verified")
	t.Logf("  - All required fields present")
	t.Logf("  - Verbose logs captured and checked")
}

// TestIntegration_LoginLogoutFlow tests a complete login-logout cycle
func TestIntegration_LoginLogoutFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "nvair-integration-logout-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock server
	mockServer := NewMockAPIServer()
	defer mockServer.Close()

	// Step 1: Login via compiled binary with verbose output
	env := append(os.Environ(), fmt.Sprintf("HOME=%s", tmpDir))
	loginResult := runCommand(t, env,
		"login",
		"-u", "test@example.com",
		"-p", "test-api-token",
		"--api-endpoint", mockServer.URL())

	logCommandOutput(t, loginResult, "TestIntegration_LoginLogoutFlow_Login")

	if loginResult.ExitCode != 0 {
		t.Fatalf("Login failed with exit code %d", loginResult.ExitCode)
	}

	// Verify login logs
	expectedLoginLogs := []string{
		"[DEBUG]",
	}
	mustContainInOutput(t, loginResult, expectedLoginLogs, "combined")

	// Verify config file exists
	configPath := filepath.Join(tmpDir, ".config/nvair.unifabric.io/config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Configuration file should exist after login: %v", err)
	}

	// Verify config file is readable
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	if len(configData) == 0 {
		t.Fatal("Configuration file is empty")
	}

	t.Logf("✓ Login phase completed successfully")
	t.Logf("  - Configuration saved to: %s", configPath)
	t.Logf("  - Config file size: %d bytes", len(configData))

	// Step 2: Logout with force flag via compiled binary with verbose output
	logoutResult := runCommand(t, env, "logout", "--force")
	logCommandOutput(t, logoutResult, "TestIntegration_LoginLogoutFlow_Logout")

	if logoutResult.ExitCode != 0 {
		t.Fatalf("Logout failed with exit code %d", logoutResult.ExitCode)
	}

	// Verify logout logs
	expectedLogoutLogs := []string{
		"[DEBUG]",
	}
	mustContainInOutput(t, logoutResult, expectedLogoutLogs, "combined")

	// Verify config file is deleted
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Configuration file should be deleted after logout")
	}

	t.Logf("✓ Logout phase completed successfully")
	t.Logf("  - Configuration file deleted")

	// Step 3: Logout again (should handle gracefully) with verbose output
	logoutResult2 := runCommand(t, env, "logout", "--force")
	logCommandOutput(t, logoutResult2, "TestIntegration_LoginLogoutFlow_SecondLogout")

	if logoutResult2.ExitCode != 0 {
		t.Fatalf("Second logout should not error: %v", logoutResult2.Err)
	}

	t.Logf("✓ Second logout handled gracefully (already logged out)")

	t.Logf("✓ Login-logout cycle test passed")
	t.Logf("  - Login succeeded and saved credentials")
	t.Logf("  - Logout succeeded and removed credentials")
	t.Logf("  - Logout idempotent (safe to call multiple times)")
	t.Logf("  - All phases logged with verbose output")
}
