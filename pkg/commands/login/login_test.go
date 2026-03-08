package login

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/testutil"
)

func TestLoginCommand_FirstTimeLogin(t *testing.T) {
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
				"result":  "OK",
				"message": "Successfully logged in.",
				"token":   loginToken,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)

		case "/v1/sshkey", "/v1/sshkey/":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("[]"))
			case http.MethodPost:
				resp := map[string]string{
					"id":          "new-key-id",
					"name":        "nvair-cli",
					"fingerprint": "test-fingerprint==",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(resp)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := NewCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = "test-api-token"
	cmd.APIEndpoint = server.URL

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
}

func TestLoginCommand_MissingUsername(t *testing.T) {
	cmd := NewCommand()
	cmd.Username = ""
	cmd.APIToken = "test-token"

	if err := cmd.Execute(); err == nil {
		t.Error("Expected error for missing username, got nil")
	}
}

func TestLoginCommand_MissingToken(t *testing.T) {
	cmd := NewCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = ""

	if err := cmd.Execute(); err == nil {
		t.Error("Expected error for missing token, got nil")
	}
}

func TestLoginCommand_InvalidEmail(t *testing.T) {
	cmd := NewCommand()
	cmd.Username = "not-an-email"
	cmd.APIToken = "test-token"

	if err := cmd.Execute(); err == nil {
		t.Error("Expected error for invalid email, got nil")
	}
}

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

	cmd := NewCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = "wrong-token"
	cmd.APIEndpoint = server.URL

	if err := cmd.Execute(); err == nil {
		t.Error("Expected error for authentication failure, got nil")
	}
}

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
		if result := isValidEmail(tt.email); result != tt.valid {
			t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, result, tt.valid)
		}
	}
}

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
		case "/v1/sshkey", "/v1/sshkey/":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("[]"))
			case http.MethodPost:
				resp := map[string]string{
					"id":          "new-key-id",
					"name":        "nvair-cli",
					"fingerprint": "test-fingerprint==",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(resp)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := NewCommand()
	cmd.Username = "test@example.com"
	cmd.APIToken = "test-api-token"
	cmd.APIEndpoint = server.URL

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Login should succeed: %v", err)
	}
}
