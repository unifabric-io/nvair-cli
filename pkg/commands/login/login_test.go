package login

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/config"
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/login/":
			t.Errorf("login endpoint should not be called when API key is used directly")
			http.NotFound(w, r)

		case "/v3/users/ssh-keys", "/v3/users/ssh-keys/":
			if got := r.Header.Get("Authorization"); got != "Bearer test-api-token" {
				t.Errorf("Authorization header mismatch: got %q", got)
			}
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
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

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if cfg.APIToken != "test-api-token" {
		t.Fatalf("expected login key to be saved as api key, got %q", cfg.APIToken)
	}
	assertNoLegacyTokenFields(t)
}

func assertNoLegacyTokenFields(t *testing.T) {
	t.Helper()

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("failed to get config path: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to parse saved config: %v", err)
	}
	for _, field := range []string{"bearerToken", "bearerTokenExpiresAt"} {
		if _, exists := raw[field]; exists {
			t.Fatalf("legacy field %q should not be saved", field)
		}
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

func TestLoginCommand_DirectKeyHasNoTokenExchange(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-login-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/login/":
			t.Errorf("login endpoint should not be called when API key is used directly")
			http.NotFound(w, r)
		case "/v3/users/ssh-keys", "/v3/users/ssh-keys/":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
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

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if cfg.APIToken != "test-api-token" {
		t.Fatalf("expected direct API key to be saved as api key, got %q", cfg.APIToken)
	}
	assertNoLegacyTokenFields(t)
}
