package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAuthLogin_Success tests successful authentication.
func TestAuthLogin_Success(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/login/" {
			http.NotFound(w, r)
			return
		}

		resp := AuthLoginResponse{
			Result:  "OK",
			Message: "Successfully logged in.",
			Token:   "test-bearer-token",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	token, expiresAt, err := client.AuthLogin("user@example.com", "test-api-token")

	if err != nil {
		t.Fatalf("AuthLogin failed: %v", err)
	}

	if token != "test-bearer-token" {
		t.Errorf("Token mismatch: got %q, want %q", token, "test-bearer-token")
	}

	// Check that expiry is roughly 24 hours from now
	now := time.Now()
	expectedTime := now.Add(24 * time.Hour)
	diff := expiresAt.Sub(expectedTime).Abs()
	if diff > 1*time.Minute {
		t.Errorf("ExpiresAt time is too far off: expected ~%v, got %v (diff: %v)", expectedTime, expiresAt, diff)
	}
}

// TestAuthLogin_InvalidCredentials tests 401 response.
func TestAuthLogin_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid credentials"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	_, _, err := client.AuthLogin("user@example.com", "wrong-token")

	if err == nil {
		t.Error("Expected error for invalid credentials, got nil")
	}
}

// TestGetSSHKeys_Success tests successful retrieval of SSH keys.
func TestGetSSHKeys_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify bearer token is present
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		keys := []GetSSHKeyResponse{
			{
				ID:          "key-1",
				Name:        "my-key",
				Fingerprint: "abc123==",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(keys)
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
		w.Write([]byte("[]"))
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
		resp := AuthLoginResponse{Result: "OK", Token: "success"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	token, _, err := client.AuthLogin("user@example.com", "token")

	if err != nil {
		t.Fatalf("AuthLogin failed after retries: %v", err)
	}

	if token != "success" {
		t.Errorf("Token mismatch: got %q, want %q", token, "success")
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

	client := NewClient(server.URL, "")
	_, _, err := client.AuthLogin("user@example.com", "token")

	if err == nil {
		t.Error("Expected error for bad request, got nil")
	}

	if attemptCount != 1 {
		t.Errorf("Expected 1 attempt (no retry for 4xx), got %d", attemptCount)
	}
}

// TestBearerTokenInHeader tests that bearer token is properly included in requests.
func TestBearerTokenInHeader(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-test-token")
	client.GetSSHKeys()

	expected := "Bearer my-test-token"
	if capturedAuth != expected {
		t.Errorf("Authorization header mismatch: got %q, want %q", capturedAuth, expected)
	}
}

// TestNoAuthOnLoginEndpoint tests that login endpoint doesn't use bearer token.
func TestNoAuthOnLoginEndpoint(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		resp := AuthLoginResponse{Result: "OK", Token: "new-token"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "irrelevant-token")
	client.AuthLogin("user@example.com", "api-token")

	if capturedAuth != "" {
		t.Errorf("AuthLogin should not include bearer token, but got %q", capturedAuth)
	}
}
