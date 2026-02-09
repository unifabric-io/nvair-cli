package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

// Client represents an HTTP API client with bearer token authentication and retry logic.
type Client struct {
	baseURL     string
	bearerToken string
	httpClient  *http.Client
	maxRetries  int
}

// NewClient creates a new API client.
// baseURL should be the base API endpoint (e.g., "https://air.nvidia.com/api")
func NewClient(baseURL, bearerToken string) *Client {
	return &Client{
		baseURL:     baseURL,
		bearerToken: bearerToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetries: 3,
	}
}

// AuthLoginRequest represents the request body for POST /v1/login/.
type AuthLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthLoginResponse represents the response body for POST /v1/login/.
type AuthLoginResponse struct {
	Result  string `json:"result"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

// AuthLogin exchanges username and API token for a bearer token.
// Returns the bearer token and expiration time.
func (c *Client) AuthLogin(username, apiToken string) (string, time.Time, error) {
	logging.Verbose("AuthLogin: Starting authentication for user: %s", username)
	logging.Verbose("AuthLogin: API endpoint: %s", c.baseURL)

	reqBody := AuthLoginRequest{
		Username: username,
		Password: apiToken,
	}

	logging.Verbose("AuthLogin: Sending POST request to /v1/login/")
	resp, err := c.doRequest("POST", "/v1/login/", &reqBody, false)
	if err != nil {
		logging.Verbose("AuthLogin: Request failed with error: %v", err)
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logging.Verbose("AuthLogin: Received status code %d with body: %s", resp.StatusCode, string(body))
		return "", time.Time{}, fmt.Errorf("auth login failed: status %d: %s", resp.StatusCode, string(body))
	}

	// Decode response
	var authResp AuthLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		logging.Verbose("AuthLogin: Failed to decode response: %v", err)
		return "", time.Time{}, fmt.Errorf("failed to decode auth response: %w", err)
	}

	// Token expires in 24 hours (based on research.md)
	expiresAt := time.Now().Add(24 * time.Hour)
	logging.Verbose("AuthLogin: Successfully obtained bearer token, expires at: %s", expiresAt)

	return authResp.Token, expiresAt, nil
}

// GetSSHKeyResponse represents a single SSH key object in the list response.
type GetSSHKeyResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Account     string `json:"account"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

// GetSSHKeys retrieves all SSH keys for the authenticated user.
// Returns nil if no keys are found (200 OK with empty list).
// Returns an error for non-2xx responses (including 404).
func (c *Client) GetSSHKeys() ([]GetSSHKeyResponse, error) {
	logging.Verbose("GetSSHKeys: Sending GET request to /v1/sshkey")
	resp, err := c.doRequest("GET", "/v1/sshkey?ordering=-name", nil, true)
	if err != nil {
		logging.Verbose("GetSSHKeys: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Handle 404 as "not found" (return empty list)
	if resp.StatusCode == http.StatusNotFound {
		logging.Verbose("GetSSHKeys: Received 404, returning empty list")
		return []GetSSHKeyResponse{}, nil
	}

	// For other non-2xx responses, return error
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		logging.Verbose("GetSSHKeys: Received status code %d with body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("get ssh keys failed: status %d: %s", resp.StatusCode, string(body))
	}

	// Decode response
	var keys []GetSSHKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		logging.Verbose("GetSSHKeys: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode ssh keys response: %w", err)
	}

	logging.Verbose("GetSSHKeys: Successfully retrieved %d SSH keys", len(keys))
	return keys, nil
}

// CreateSSHKeyRequest represents the request body for POST /v1/sshkey.
type CreateSSHKeyRequest struct {
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`
}

// CreateSSHKeyResponse represents the response body for POST /v1/sshkey.
type CreateSSHKeyResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Account     string `json:"account"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

// CreateSSHKey uploads a new SSH public key to the user's account.
// Returns the created key or an error.
// Note: 409 Conflict (key already exists) is returned as an error - caller should handle it.
func (c *Client) CreateSSHKey(publicKey, name string) (*CreateSSHKeyResponse, error) {
	logging.Verbose("CreateSSHKey: Starting SSH key upload for key name: %s", name)
	publicKey = strings.Trim(publicKey, "\n")
	reqBody := CreateSSHKeyRequest{
		PublicKey: publicKey,
		Name:      name,
	}

	logging.Verbose("CreateSSHKey: Sending POST request to /v1/sshkey")
	resp, err := c.doRequest("POST", "/v1/sshkey/", &reqBody, true)
	if err != nil {
		logging.Verbose("CreateSSHKey: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logging.Verbose("CreateSSHKey: Received status code %d with body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("create ssh key failed: status %d: %s", resp.StatusCode, string(body))
	}

	// Read the full response body for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	logging.Verbose("CreateSSHKey: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))

	// Decode response - API returns a single object
	var keyResp CreateSSHKeyResponse
	if err := json.Unmarshal(bodyBytes, &keyResp); err != nil {
		// If we get a JSON decode error and status code is 201 Created,
		// it might be that the API returns an empty body for successful creation
		if resp.StatusCode == http.StatusCreated {
			logging.Verbose("CreateSSHKey: API returned 201 but empty/invalid response body, treating as success")
			// Return a minimal response - the key was created successfully
			return &CreateSSHKeyResponse{
				Name: name,
			}, nil
		}
		logging.Verbose("CreateSSHKey: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode create ssh key response: %w", err)
	}

	logging.Verbose("CreateSSHKey: Successfully created SSH key with ID: %s", keyResp.ID)
	return &keyResp, nil
}

// doRequest performs an HTTP request with retry logic and bearer token injection.
// useAuth determines whether to include the bearer token in the Authorization header.
func (c *Client) doRequest(method, path string, body interface{}, useAuth bool) (*http.Response, error) {
	// Retry logic
	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		logging.Verbose("doRequest: [Attempt %d/%d] %s %s", attempt+1, c.maxRetries, method, c.baseURL+path)

		// Create a fresh body reader for each attempt
		var reqBodyReader io.Reader
		if body != nil {
			bodyBytes, _ := json.Marshal(body)
			logging.Verbose("doRequest: Request body: %s", string(bodyBytes))
			reqBodyReader = bytes.NewReader(bodyBytes)
		}

		// Create request
		req, err := http.NewRequest(method, c.baseURL+path, reqBodyReader)
		if err != nil {
			logging.Verbose("doRequest: Failed to create request: %v", err)
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		if useAuth && c.bearerToken != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.bearerToken))
			// Log truncated token for security
			truncatedToken := c.bearerToken
			if len(truncatedToken) > 20 {
				truncatedToken = truncatedToken[:20] + "..."
			}
			logging.Verbose("doRequest: Bearer token header set with token: %s", truncatedToken)
		}

		// Perform the request
		startTime := time.Now()
		resp, err := c.httpClient.Do(req)
		duration := time.Since(startTime)

		if err != nil {
			logging.Verbose("doRequest: Network error after %dms: %v", duration.Milliseconds(), err)
			lastErr = err
			// Retry on network errors
			if attempt < c.maxRetries-1 {
				backoff := time.Duration((1<<uint(attempt))*100) * time.Millisecond
				logging.Verbose("doRequest: Retrying in %dms", backoff.Milliseconds())
				time.Sleep(backoff)
				continue
			}
			return nil, fmt.Errorf("request failed after %d retries: %w", c.maxRetries, lastErr)
		}

		logging.Verbose("doRequest: Response status code: %d (received in %dms)", resp.StatusCode, duration.Milliseconds())

		// Check if we should retry based on status code
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
			// Transient error, retry
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			logging.Verbose("doRequest: Transient error (status %d), response: %s", resp.StatusCode, string(body))
			lastErr = fmt.Errorf("transient error: status %d", resp.StatusCode)
			if attempt < c.maxRetries-1 {
				backoff := time.Duration((1<<uint(attempt))*100) * time.Millisecond
				logging.Verbose("doRequest: Retrying in %dms", backoff.Milliseconds())
				time.Sleep(backoff)
				continue
			}
			return nil, fmt.Errorf("transient error after %d retries: status %d", c.maxRetries, resp.StatusCode)
		}

		// Success (or permanent error)
		return resp, nil
	}

	return nil, lastErr
}
