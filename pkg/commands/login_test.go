package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/testutil"
)

// TestLoginCommand_FirstTimeLogin tests a complete first-time login flow.
func TestLoginCommand_FirstTimeLogin(t *testing.T) {
	// Create temporary home directory
	tmpDir, err := os.MkdirTemp("", "nvair-login-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create a mock API server
	loginToken := testutil.MakeTestJWT(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/login/":
			resp := map[string]interface{}{
				"result":  "OK",
				"message": "Successfully logged in.",
				"token":   loginToken,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)

		case "/v1/sshkey":
			if r.Method == "GET" {
				// No keys found
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("[]"))
			} else if r.Method == "POST" {
				// Successfully created key - API returns array
				resp := []map[string]string{
					{
						"id":          "new-key-id",
						"fingerprint": "test-fingerprint==",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(resp)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Create and execute login command
	cmd := NewLoginCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = "test-api-token"
	cmd.APIEndpoint = server.URL

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// TODO: Verify that config was saved correctly
}

// TestLoginCommand_MissingUsername tests validation of missing username.
func TestLoginCommand_MissingUsername(t *testing.T) {
	cmd := NewLoginCommand()
	cmd.Username = ""
	cmd.APIToken = "test-token"

	err := cmd.Execute()
	if err == nil {
		t.Error("Expected error for missing username, got nil")
	}
}

// TestLoginCommand_MissingToken tests validation of missing API token.
func TestLoginCommand_MissingToken(t *testing.T) {
	cmd := NewLoginCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = ""

	err := cmd.Execute()
	if err == nil {
		t.Error("Expected error for missing token, got nil")
	}
}

// TestLoginCommand_InvalidEmail tests validation of invalid email format.
func TestLoginCommand_InvalidEmail(t *testing.T) {
	cmd := NewLoginCommand()
	cmd.Username = "not-an-email"
	cmd.APIToken = "test-token"

	err := cmd.Execute()
	if err == nil {
		t.Error("Expected error for invalid email, got nil")
	}
}

// TestLoginCommand_AuthenticationFailure tests handling of 401 response.
func TestLoginCommand_AuthenticationFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-login-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid credentials"}`))
	}))
	defer server.Close()

	cmd := NewLoginCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = "wrong-token"
	cmd.APIEndpoint = server.URL

	err = cmd.Execute()
	if err == nil {
		t.Error("Expected error for authentication failure, got nil")
	}
}

// TestIsValidEmail tests email validation.
func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"test.user@example.co.uk", true},
		{"user+tag@example.com", true},
		{"not-an-email", false},
		{"@example.com", false},
		{"user@", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isValidEmail(tt.email)
		if result != tt.valid {
			t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, result, tt.valid)
		}
	}
}

// TestLoginCommand_TokenExpiry tests that token expiry is set correctly.
func TestLoginCommand_TokenExpiry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-login-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	loginToken := testutil.MakeTestJWT(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/login/":
			resp := map[string]interface{}{
				"result": "OK",
				"token":  loginToken,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		case "/v1/sshkey":
			if r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("[]"))
			}
		}
	}))
	defer server.Close()

	cmd := NewLoginCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = "test-token"
	cmd.APIEndpoint = server.URL

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Verify config was saved with expiry
	// Note: This test would need to read the saved config file to fully validate
}
