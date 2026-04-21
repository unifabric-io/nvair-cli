package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

// Client represents an HTTP API client with bearer token authentication and retry logic.
type Client struct {
	baseURL     string
	bearerToken string
	httpClient  *http.Client
	maxRetries  int
}

// TokenExpireTime parses a JWT token and extracts the expiration time from the exp claim.
// Returns the expiration time or an error if the token is invalid.
func TokenExpireTime(token string) (time.Time, error) {
	if token == "" {
		return time.Time{}, fmt.Errorf("token is empty")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to decode token payload: %w", err)
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("failed to unmarshal token claims: %w", err)
	}

	if claims.Exp <= 0 {
		return time.Time{}, fmt.Errorf("invalid exp claim: %d", claims.Exp)
	}

	return time.Unix(claims.Exp, 0), nil
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

	// Parse the JWT token to extract the actual expiration time
	expiresAt, err := TokenExpireTime(authResp.Token)
	if err != nil {
		logging.Verbose("AuthLogin: Failed to parse token expiration: %v", err)
		return "", time.Time{}, fmt.Errorf("failed to parse bearer token: %w", err)
	}
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

// DeleteSSHKey deletes an SSH key by its ID.
// Returns an error if the deletion fails.
func (c *Client) DeleteSSHKey(keyID string) error {
	logging.Verbose("DeleteSSHKey: Starting SSH key deletion for key ID: %s", keyID)

	logging.Verbose("DeleteSSHKey: Sending DELETE request to /v1/sshkey/%s", keyID)
	resp, err := c.doRequest("DELETE", fmt.Sprintf("/v1/sshkey/%s/", keyID), nil, true)
	if err != nil {
		logging.Verbose("DeleteSSHKey: Request failed with error: %v", err)
		return err
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logging.Verbose("DeleteSSHKey: Received status code %d with body: %s", resp.StatusCode, string(body))
		return fmt.Errorf("delete ssh key failed: status %d: %s", resp.StatusCode, string(body))
	}

	logging.Verbose("DeleteSSHKey: Successfully deleted SSH key with ID: %s", keyID)
	return nil
}

// EnableSSHResponse represents the response body for POST /v1/service/
type EnableSSHResponse struct {
	URL               string `json:"url"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Simulation        string `json:"simulation"`
	Interface         string `json:"interface"`
	DestPort          int    `json:"dest_port"`
	SrcPort           int    `json:"src_port"`
	Link              string `json:"link"`
	ServiceType       string `json:"service_type"`
	NodeName          string `json:"node_name"`
	InterfaceName     string `json:"interface_name"`
	Host              string `json:"host"`
	OSDefaultUsername string `json:"os_default_username"`
}

// CreateService creates a service for a simulation interface.
// serviceName: the name of the service (e.g., "bastion-ssh", "k8s-api-server")
// destPort: the destination port on the target interface (e.g., 22 for SSH, 6443 for Kubernetes API)
// serviceType: the type of service (e.g., "ssh", "kubernetes")
// Returns the service details including the host and port information.
func (c *Client) CreateService(simulationID, interfaceID, serviceName string, destPort int, serviceType string) (*EnableSSHResponse, error) {
	logging.Verbose("CreateService: Creating %s service for simulation: %s, interface: %s, destPort: %d", serviceType, simulationID, interfaceID, destPort)

	reqBody := map[string]interface{}{
		"name":         serviceName,
		"simulation":   simulationID,
		"interface":    interfaceID,
		"dest_port":    destPort,
		"service_type": serviceType,
	}

	logging.Verbose("CreateService: Sending POST request to /v1/service/")
	resp, err := c.doRequest("POST", "/v1/service/", &reqBody, true)
	if err != nil {
		logging.Verbose("CreateService: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("CreateService: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("create %s service failed: status %d: %s", serviceType, resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var svcResp EnableSSHResponse
	if err := json.Unmarshal(bodyBytes, &svcResp); err != nil {
		logging.Verbose("CreateService: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode create %s service response: %w", serviceType, err)
	}

	logging.Verbose("CreateService: Successfully created %s service with ID: %s, SrcPort: %d, Host: %s", serviceType, svcResp.ID, svcResp.SrcPort, svcResp.Host)
	return &svcResp, nil
}

// CreateSSHService creates the default bastion SSH service for a simulation interface.
// Returns the service details including the host and port information.
func (c *Client) CreateSSHService(simulationID, interfaceID string) (*EnableSSHResponse, error) {
	return c.CreateService(simulationID, interfaceID, constant.DefaultBastionSSHServiceName, 22, "ssh")
}

// CreateKubernetesAPIService creates a Kubernetes API service for a simulation interface.
// Returns the service details including the host and port information.
func (c *Client) CreateKubernetesAPIService(simulationID, interfaceID string) (*EnableSSHResponse, error) {
	return c.CreateService(simulationID, interfaceID, "k8s-api-server", 6443, "other")
}

// GetServices lists services for a simulation.
func (c *Client) GetServices(simulationID string) ([]EnableSSHResponse, error) {
	logging.Verbose("GetServices: Fetching services for simulation: %s", simulationID)

	query := url.Values{}
	query.Set("simulation", simulationID)
	path := fmt.Sprintf("/v1/service?%s", query.Encode())

	resp, err := c.doRequest("GET", path, nil, true)
	if err != nil {
		logging.Verbose("GetServices: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetServices: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get services failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var services []EnableSSHResponse
	if err := json.Unmarshal(bodyBytes, &services); err != nil {
		logging.Verbose("GetServices: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode get services response: %w", err)
	}

	logging.Verbose("GetServices: Successfully retrieved %d services", len(services))
	return services, nil
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
			// logging.Verbose("doRequest: Request body: %s", string(bodyBytes))
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

// CreateSimulationRequest represents the request body for POST /v1/simulations
type CreateSimulationRequest struct {
	Topology interface{} `json:"topology"`
}

// CreateSimulationResponse represents the response body for POST /v2/simulations/import/
type CreateSimulationResponse struct {
	ID               string      `json:"id"`
	Title            string      `json:"title"`
	Organization     interface{} `json:"organization"`
	OrganizationName interface{} `json:"organization_name"`
}

// CreateSimulation creates a new simulation from a topology
func (c *Client) CreateSimulation(topo *topology.RawTopology) (*CreateSimulationResponse, error) {
	logging.Verbose("CreateSimulation: Starting simulation creation with topology: %s", topo.Title)

	reqBody := topo

	logging.Verbose("CreateSimulation: Sending POST request to /api/v2/simulations/import/")
	resp, err := c.doRequest("POST", "/v2/simulations/import/", &reqBody, true)
	if err != nil {
		logging.Verbose("CreateSimulation: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("CreateSimulation: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("create simulation failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var simResp CreateSimulationResponse
	if err := json.Unmarshal(bodyBytes, &simResp); err != nil {
		logging.Verbose("CreateSimulation: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode create simulation response: %w", err)
	}

	logging.Verbose("CreateSimulation: Successfully created simulation with ID: %s, Title: %s", simResp.ID, simResp.Title)
	return &simResp, nil
}

// ControlSimulationResponse represents the response body for POST /v1/simulation/{id}/control/
type ControlSimulationResponse struct {
	Result string   `json:"result"`
	Jobs   []string `json:"jobs"`
}

// ControlSimulation sets the control state of a simulation (e.g., "load", "play", "stop")
func (c *Client) ControlSimulation(simulationID, state string) (*ControlSimulationResponse, error) {
	logging.Verbose("ControlSimulation: Setting simulation %s to state: %s", simulationID, state)

	payload, err := json.Marshal(map[string]string{"action": state})
	if err != nil {
		logging.Verbose("ControlSimulation: Failed to marshal request body: %v", err)
		return nil, fmt.Errorf("failed to marshal control request: %w", err)
	}

	endpoint := fmt.Sprintf("/v1/simulation/%s/control/", simulationID)
	logging.Verbose("ControlSimulation: Sending POST request to %s with payload: %s", endpoint, string(payload))

	resp, err := c.doRequest("POST", endpoint, map[string]string{"action": state}, true)
	if err != nil {
		logging.Verbose("ControlSimulation: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("ControlSimulation: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("control simulation failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var ctrlResp ControlSimulationResponse
	if err := json.Unmarshal(bodyBytes, &ctrlResp); err != nil {
		logging.Verbose("ControlSimulation: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode control simulation response: %w", err)
	}

	logging.Verbose("ControlSimulation: Successfully set simulation %s to state %s, result: %s", simulationID, state, ctrlResp.Result)
	return &ctrlResp, nil
}

// SimulationInfo represents a single simulation in the list response
type SimulationInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Created string `json:"created"`
}

// ListSimulationsResponse represents the response body for GET /v2/simulations
type ListSimulationsResponse struct {
	Count    int              `json:"count"`
	Next     interface{}      `json:"next"`
	Previous interface{}      `json:"previous"`
	Results  []SimulationInfo `json:"results"`
}

// ImageInfo represents a single image returned by GET /v2/images.
type ImageInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListImagesResponse represents the response body for GET /v2/images.
type ListImagesResponse struct {
	Count    int         `json:"count"`
	Next     interface{} `json:"next"`
	Previous interface{} `json:"previous"`
	Results  []ImageInfo `json:"results"`
}

// GetImages retrieves images from the authenticated user's catalog.
func (c *Client) GetImages() ([]ImageInfo, error) {
	query := url.Values{}
	query.Set("limit", strconv.FormatInt(math.MaxInt64, 10))

	path := fmt.Sprintf("/v2/images?%s", query.Encode())
	logging.Verbose("GetImages: Sending GET request to %s", path)
	resp, err := c.doRequest("GET", path, nil, true)
	if err != nil {
		logging.Verbose("GetImages: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetImages: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get images failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var listResp ListImagesResponse
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		logging.Verbose("GetImages: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode images list response: %w", err)
	}

	logging.Verbose("GetImages: Successfully retrieved %d images", len(listResp.Results))
	return listResp.Results, nil
}

// GetSimulations retrieves all simulations for the authenticated user
func (c *Client) GetSimulations() ([]SimulationInfo, error) {
	logging.Verbose("GetSimulations: Sending GET request to /v2/simulations")
	resp, err := c.doRequest("GET", "/v2/simulations", nil, true)
	if err != nil {
		logging.Verbose("GetSimulations: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetSimulations: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get simulations failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var listResp ListSimulationsResponse
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		logging.Verbose("GetSimulations: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode simulations list response: %w", err)
	}

	logging.Verbose("GetSimulations: Successfully retrieved %d simulations", len(listResp.Results))
	return listResp.Results, nil
}

// DeleteSimulation deletes a simulation by name
// It first retrieves all simulations, finds the one matching the given name (title),
// and then deletes it using its ID.
func (c *Client) DeleteSimulation(name string) error {
	logging.Verbose("DeleteSimulation: Starting deletion of simulation: %s", name)

	// Get all simulations
	simulations, err := c.GetSimulations()
	if err != nil {
		logging.Verbose("DeleteSimulation: Failed to get simulations list: %v", err)
		return fmt.Errorf("failed to list simulations: %w", err)
	}

	// Find the simulation with matching title
	var simulationID string
	for _, sim := range simulations {
		if sim.Title == name {
			simulationID = sim.ID
			break
		}
	}

	if simulationID == "" {
		logging.Verbose("DeleteSimulation: Simulation with title '%s' not found", name)
		return fmt.Errorf("simulation '%s' not found", name)
	}

	// Delete the simulation using its ID
	logging.Verbose("DeleteSimulation: Found simulation ID: %s, deleting", simulationID)

	if err := c.DeleteSimulationByID(simulationID); err != nil {
		return err
	}

	logging.Verbose("DeleteSimulation: Successfully deleted simulation: %s (ID: %s)", name, simulationID)
	return nil
}

// DeleteSimulationByID deletes a simulation by its ID.
// Returns an error if the deletion fails.
func (c *Client) DeleteSimulationByID(simulationID string) error {
	logging.Verbose("DeleteSimulationByID: Starting deletion of simulation ID: %s", simulationID)

	endpoint := fmt.Sprintf("/v2/simulations/%s/", simulationID)
	logging.Verbose("DeleteSimulationByID: Sending DELETE request to %s", endpoint)

	resp, err := c.doRequest("DELETE", endpoint, nil, true)
	if err != nil {
		logging.Verbose("DeleteSimulationByID: Request failed with error: %v", err)
		return err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("DeleteSimulationByID: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("delete simulation failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logging.Verbose("DeleteSimulationByID: Successfully deleted simulation ID: %s", simulationID)
	return nil
}

// DeleteService deletes a service by name
func (c *Client) DeleteService(name string) error {
	logging.Verbose("DeleteService: Starting deletion of service: %s", name)

	endpoint := fmt.Sprintf("/v1/services/%s", name)
	logging.Verbose("DeleteService: Sending DELETE request to %s", endpoint)

	resp, err := c.doRequest("DELETE", endpoint, nil, true)
	if err != nil {
		logging.Verbose("DeleteService: Request failed with error: %v", err)
		return err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Handle 404 specifically
	if resp.StatusCode == http.StatusNotFound {
		logging.Verbose("DeleteService: Service not found (404)")
		return fmt.Errorf("service '%s' not found", name)
	}

	// Check for other errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("DeleteService: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("delete service failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logging.Verbose("DeleteService: Successfully deleted service: %s", name)
	return nil
}

// DeleteServiceByID deletes a service by its ID.
func (c *Client) DeleteServiceByID(serviceID string) error {
	logging.Verbose("DeleteServiceByID: Starting deletion of service ID: %s", serviceID)

	endpoint := fmt.Sprintf("/v1/service/%s/", serviceID)
	logging.Verbose("DeleteServiceByID: Sending DELETE request to %s", endpoint)

	resp, err := c.doRequest("DELETE", endpoint, nil, true)
	if err != nil {
		logging.Verbose("DeleteServiceByID: Request failed with error: %v", err)
		return err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		logging.Verbose("DeleteServiceByID: Service not found (404)")
		return fmt.Errorf("service '%s' not found", serviceID)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("DeleteServiceByID: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("delete service failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logging.Verbose("DeleteServiceByID: Successfully deleted service ID: %s", serviceID)
	return nil
}

// Job represents a job status from the API
type Job struct {
	Category    string `json:"category"`
	Created     string `json:"created"`
	ID          string `json:"id"`
	LastUpdated string `json:"last_updated"`
	Simulation  string `json:"simulation"`
	State       string `json:"state"`
}

// GetJob retrieves the status of a specific job
func (c *Client) GetJob(jobID string) (*Job, error) {
	logging.Verbose("GetJob: Fetching job status for job ID: %s", jobID)

	endpoint := fmt.Sprintf("/v2/jobs/%s", jobID)
	resp, err := c.doRequest("GET", endpoint, nil, true)
	if err != nil {
		logging.Verbose("GetJob: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetJob: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get job failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var job Job
	if err := json.Unmarshal(bodyBytes, &job); err != nil {
		logging.Verbose("GetJob: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode job response: %w", err)
	}

	logging.Verbose("GetJob: Successfully retrieved job %s, state: %s", jobID, job.State)
	return &job, nil
}

// Node represents a simulation node
type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Metadata   string `json:"metadata"`
	OS         string `json:"os"`
	OSName     string `json:"-"`
	Simulation string `json:"simulation"`
}

// nodeListResponse represents the response body for GET /v2/simulations/nodes/
type nodeListResponse struct {
	Results []Node `json:"results"`
}

type rawNode struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	State      string      `json:"state"`
	Metadata   interface{} `json:"metadata"`
	OS         string      `json:"os"`
	Simulation string      `json:"simulation"`
}

type rawNodeListResponse struct {
	Results []rawNode `json:"results"`
}

// GetNodes retrieves all nodes for a simulation
func (c *Client) GetNodes(simulationID string) ([]Node, error) {
	logging.Verbose("GetNodes: Fetching nodes for simulation")

	query := url.Values{}
	query.Set("simulation", simulationID)
	query.Set("ordering", "os")
	query.Set("limit", strconv.FormatInt(math.MaxInt64, 10))

	path := fmt.Sprintf("/v2/simulations/nodes/?%s", query.Encode())
	resp, err := c.doRequest("GET", path, nil, true)
	if err != nil {
		logging.Verbose("GetNodes: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetNodes: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get nodes failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response with null-tolerant metadata handling.
	var rawResp rawNodeListResponse
	if err := json.Unmarshal(bodyBytes, &rawResp); err != nil {
		logging.Verbose("GetNodes: Failed to decode response: %v, body: %s", err, string(bodyBytes))
		return nil, fmt.Errorf("failed to decode nodes response: %w", err)
	}

	listResp := nodeListResponse{Results: make([]Node, 0, len(rawResp.Results))}
	for _, n := range rawResp.Results {
		metadata := ""
		if s, ok := n.Metadata.(string); ok {
			metadata = s
		}
		listResp.Results = append(listResp.Results, Node{
			ID:         n.ID,
			Name:       n.Name,
			State:      n.State,
			Metadata:   metadata,
			OS:         n.OS,
			Simulation: n.Simulation,
		})
	}

	logging.Verbose("GetNodes: Successfully retrieved %d nodes", len(listResp.Results))
	return listResp.Results, nil
}

// GetAllNodes retrieves all nodes across all simulations for the authenticated user.
// Each returned Node has a Simulation field indicating which simulation it belongs to.
func (c *Client) GetAllNodes() ([]Node, error) {
	logging.Verbose("GetAllNodes: Fetching all nodes across all simulations")

	query := url.Values{}
	query.Set("ordering", "os")
	query.Set("limit", strconv.FormatInt(math.MaxInt64, 10))

	path := fmt.Sprintf("/v2/simulations/nodes/?%s", query.Encode())
	resp, err := c.doRequest("GET", path, nil, true)
	if err != nil {
		logging.Verbose("GetAllNodes: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetAllNodes: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get all nodes failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var rawResp rawNodeListResponse
	if err := json.Unmarshal(bodyBytes, &rawResp); err != nil {
		logging.Verbose("GetAllNodes: Failed to decode response: %v, body: %s", err, string(bodyBytes))
		return nil, fmt.Errorf("failed to decode all nodes response: %w", err)
	}

	nodes := make([]Node, 0, len(rawResp.Results))
	for _, n := range rawResp.Results {
		metadata := ""
		if s, ok := n.Metadata.(string); ok {
			metadata = s
		}
		nodes = append(nodes, Node{
			ID:         n.ID,
			Name:       n.Name,
			State:      n.State,
			Metadata:   metadata,
			OS:         n.OS,
			Simulation: n.Simulation,
		})
	}

	logging.Verbose("GetAllNodes: Successfully retrieved %d nodes", len(nodes))
	return nodes, nil
}

// Interface represents a simulation node interface
type Interface struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	InterfaceType string `json:"interface_type"`
	MacAddress    string `json:"mac_address"`
	LinkUp        bool   `json:"link_up"`
	InternalIPv4  string `json:"internal_ipv4"`
	FullIPv6      string `json:"full_ipv6"`
	PrefixIPv6    string `json:"prefix_ipv6"`
	PortNumber    int    `json:"port_number"`
	Node          string `json:"node"`
	Simulation    string `json:"simulation"`
	Outbound      bool   `json:"outbound"`
	Link          string `json:"link"`
}

// interfaceListResponse represents the response body for GET /v2/simulations/nodes/interfaces
type interfaceListResponse struct {
	Results []Interface `json:"results"`
}

// GetNodeInterfaces retrieves interfaces for a specific node in a simulation
func (c *Client) GetNodeInterfaces(simulationID, nodeID string) ([]Interface, error) {
	logging.Verbose("GetNodeInterfaces: Fetching interfaces for node %s in simulation %s", nodeID, simulationID)

	endpoint := fmt.Sprintf("/v2/simulations/nodes/interfaces?simulation=%s&node=%s", simulationID, nodeID)
	resp, err := c.doRequest("GET", endpoint, nil, true)
	if err != nil {
		logging.Verbose("GetNodeInterfaces: Request failed with error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Verbose("GetNodeInterfaces: Received status code %d with body: %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("get node interfaces failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var listResp interfaceListResponse
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		logging.Verbose("GetNodeInterfaces: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode interfaces response: %w", err)
	}

	logging.Verbose("GetNodeInterfaces: Successfully retrieved %d interfaces for node %s", len(listResp.Results), nodeID)
	return listResp.Results, nil
}
