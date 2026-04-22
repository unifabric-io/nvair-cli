package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	forwardutil "github.com/unifabric-io/nvair-cli/pkg/forward"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

const (
	DefaultBaseURL = "https://api.dsx-air.nvidia.com/api"

	apiVersionPath = "/v3"

	sshKeysEndpoint           = apiVersionPath + "/users/ssh-keys"
	serviceEndpoint           = apiVersionPath + "/service"
	servicesEndpoint          = apiVersionPath + "/services"
	simulationEndpoint        = apiVersionPath + "/simulation"
	simulationsEndpoint       = apiVersionPath + "/simulations"
	simulationsImportEndpoint = simulationsEndpoint + "/import/"
	imagesEndpoint            = apiVersionPath + "/images"
	jobsEndpoint              = apiVersionPath + "/jobs"
	nodesEndpoint             = simulationsEndpoint + "/nodes/"
	nodeInterfacesEndpoint    = simulationsEndpoint + "/nodes/interfaces"
	interfaceServicesEndpoint = nodeInterfacesEndpoint + "/services/"

	DefaultSimulationsEndpoint = DefaultBaseURL + simulationsEndpoint
)

// Client represents an HTTP API client with API token authentication and retry logic.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
	maxRetries int
}

// NewClient creates a new API client.
// baseURL should be the base API endpoint (e.g., DefaultBaseURL).
func NewClient(baseURL, apiToken string) *Client {
	return &Client{
		baseURL:  baseURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetries: 3,
	}
}

// GetSSHKeyResponse represents a single SSH key object in the list response.
type GetSSHKeyResponse struct {
	Created     string `json:"created"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

// listSSHKeysResponse represents the paginated response body for GET /v3/users/ssh-keys/.
type listSSHKeysResponse struct {
	Count    int                 `json:"count"`
	Next     interface{}         `json:"next"`
	Previous interface{}         `json:"previous"`
	Results  []GetSSHKeyResponse `json:"results"`
}

// GetSSHKeys retrieves all SSH keys for the authenticated user.
// Returns nil if no keys are found (200 OK with empty list).
// Returns an error for non-2xx responses (including 404).
func (c *Client) GetSSHKeys() ([]GetSSHKeyResponse, error) {
	endpoint := sshKeysEndpoint + "/?limit="
	logging.Verbose("GetSSHKeys: Sending GET request to %s", endpoint)
	resp, err := c.doRequest("GET", endpoint, nil, true)
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

	var listResp listSSHKeysResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		logging.Verbose("GetSSHKeys: Failed to decode response: %v", err)
		return nil, fmt.Errorf("failed to decode ssh keys response: %w", err)
	}

	logging.Verbose("GetSSHKeys: Successfully retrieved %d SSH keys", len(listResp.Results))
	return listResp.Results, nil
}

// CreateSSHKeyRequest represents the request body for POST /v3/users/ssh-keys/.
type CreateSSHKeyRequest struct {
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`
}

// CreateSSHKeyResponse represents the response body for POST /v3/users/ssh-keys/.
type CreateSSHKeyResponse struct {
	Created     string `json:"created"`
	ID          string `json:"id"`
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

	endpoint := sshKeysEndpoint + "/"
	logging.Verbose("CreateSSHKey: Sending POST request to %s", endpoint)
	resp, err := c.doRequest("POST", endpoint, &reqBody, true)
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

	endpoint := fmt.Sprintf("%s/%s/", sshKeysEndpoint, keyID)
	logging.Verbose("DeleteSSHKey: Sending DELETE request to %s", endpoint)
	resp, err := c.doRequest("DELETE", endpoint, nil, true)
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

// EnableSSHResponse represents a service/forward response.
type EnableSSHResponse struct {
	URL               string `json:"url"`
	ID                string `json:"id"`
	Created           string `json:"created"`
	Modified          string `json:"modified"`
	Name              string `json:"name"`
	Simulation        string `json:"simulation"`
	Interface         string `json:"interface"`
	DestPort          int    `json:"dest_port"`
	SrcPort           int    `json:"src_port"`
	NodePort          int    `json:"node_port"`
	WorkerPort        int    `json:"worker_port"`
	WorkerFQDN        string `json:"worker_fqdn"`
	Link              string `json:"link"`
	ServiceType       string `json:"service_type"`
	NodeName          string `json:"node_name"`
	InterfaceName     string `json:"interface_name"`
	Host              string `json:"host"`
	OSDefaultUsername string `json:"os_default_username"`
}

func (s *EnableSSHResponse) normalize() {
	if s.DestPort == 0 {
		s.DestPort = s.NodePort
	}
	if s.SrcPort == 0 {
		s.SrcPort = s.WorkerPort
	}
	if s.Host == "" {
		s.Host = s.WorkerFQDN
	}
}

type listServicesResponse struct {
	Count    int                 `json:"count"`
	Next     interface{}         `json:"next"`
	Previous interface{}         `json:"previous"`
	Results  []EnableSSHResponse `json:"results"`
}

// CreateService creates a service for a simulation interface.
// serviceName: the name of the service (e.g., "forward-22->oob-mgmt-server:22", "k8s-api-server")
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

	endpoint := serviceEndpoint + "/"
	logging.Verbose("CreateService: Sending POST request to %s", endpoint)
	resp, err := c.doRequest("POST", endpoint, &reqBody, true)
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
	svcResp.normalize()

	logging.Verbose("CreateService: Successfully created %s service with ID: %s, SrcPort: %d, Host: %s", serviceType, svcResp.ID, svcResp.SrcPort, svcResp.Host)
	return &svcResp, nil
}

// CreateSSHService creates the default bastion SSH service for a simulation interface.
// Returns the service details including the host and port information.
func (c *Client) CreateSSHService(simulationID, interfaceID string) (*EnableSSHResponse, error) {
	return c.CreateService(simulationID, interfaceID, forwardutil.BuildBastionSSHServiceName(), 22, "ssh")
}

// CreateKubernetesAPIService creates a Kubernetes API service for a simulation interface.
// Returns the service details including the host and port information.
func (c *Client) CreateKubernetesAPIService(simulationID, interfaceID string) (*EnableSSHResponse, error) {
	return c.CreateService(simulationID, interfaceID, "k8s-api-server", 6443, "other")
}

// GetServices lists interface services for a simulation.
func (c *Client) GetServices(simulationID string) ([]EnableSSHResponse, error) {
	logging.Verbose("GetServices: Fetching services for simulation: %s", simulationID)

	query := url.Values{}
	query.Set("simulation", simulationID)
	query.Set("limit", "25")
	path := fmt.Sprintf("%s?%s", interfaceServicesEndpoint, query.Encode())

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

	var listResp listServicesResponse
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		var services []EnableSSHResponse
		if arrayErr := json.Unmarshal(bodyBytes, &services); arrayErr != nil {
			logging.Verbose("GetServices: Failed to decode response: %v", err)
			return nil, fmt.Errorf("failed to decode get services response: %w", err)
		}
		for i := range services {
			services[i].normalize()
		}
		logging.Verbose("GetServices: Successfully retrieved %d services", len(services))
		return services, nil
	}

	for i := range listResp.Results {
		listResp.Results[i].normalize()
	}
	logging.Verbose("GetServices: Successfully retrieved %d services", len(listResp.Results))
	return listResp.Results, nil
}

// doRequest performs an HTTP request with retry logic and Authorization header injection.
// useAuth determines whether to include the API token in the Authorization header.
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
		if useAuth && c.apiToken != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
			// Log truncated token for security
			truncatedToken := c.apiToken
			if len(truncatedToken) > 20 {
				truncatedToken = truncatedToken[:20] + "..."
			}
			logging.Verbose("doRequest: Authorization header set with API token: %s", truncatedToken)
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

// CreateSimulationRequest represents the request body for POST /v3/simulations
type CreateSimulationRequest struct {
	Topology interface{} `json:"topology"`
}

// CreateSimulationResponse represents the response body for POST /v3/simulations/import/
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

	logging.Verbose("CreateSimulation: Sending POST request to %s", simulationsImportEndpoint)
	resp, err := c.doRequest("POST", simulationsImportEndpoint, &reqBody, true)
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

// ControlSimulationResponse represents the response body for POST /v3/simulation/{id}/control/
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

	endpoint := fmt.Sprintf("%s/%s/control/", simulationEndpoint, simulationID)
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
	Name    string `json:"name"`
	State   string `json:"state"`
	Created string `json:"created"`
}

// ListSimulationsResponse represents the response body for GET /v3/simulations
type ListSimulationsResponse struct {
	Count    int              `json:"count"`
	Next     interface{}      `json:"next"`
	Previous interface{}      `json:"previous"`
	Results  []SimulationInfo `json:"results"`
}

// ImageInfo represents a single image returned by GET /v3/images.
type ImageInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListImagesResponse represents the response body for GET /v3/images.
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

	path := fmt.Sprintf("%s?%s", imagesEndpoint, query.Encode())
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
	logging.Verbose("GetSimulations: Sending GET request to %s", simulationsEndpoint)
	resp, err := c.doRequest("GET", simulationsEndpoint, nil, true)
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
// It first retrieves all simulations, finds the one matching the given name,
// and then deletes it using its ID.
func (c *Client) DeleteSimulation(name string) error {
	logging.Verbose("DeleteSimulation: Starting deletion of simulation: %s", name)

	// Get all simulations
	simulations, err := c.GetSimulations()
	if err != nil {
		logging.Verbose("DeleteSimulation: Failed to get simulations list: %v", err)
		return fmt.Errorf("failed to list simulations: %w", err)
	}

	// Find the simulation with matching name
	var simulationID string
	for _, sim := range simulations {
		if sim.Name == name {
			simulationID = sim.ID
			break
		}
	}

	if simulationID == "" {
		logging.Verbose("DeleteSimulation: Simulation with name '%s' not found", name)
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

	endpoint := fmt.Sprintf("%s/%s/", simulationsEndpoint, simulationID)
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

	endpoint := fmt.Sprintf("%s/%s", servicesEndpoint, name)
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

	endpoint := fmt.Sprintf("%s/%s/", serviceEndpoint, serviceID)
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

	endpoint := fmt.Sprintf("%s/%s", jobsEndpoint, jobID)
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

// nodeListResponse represents the response body for GET /v3/simulations/nodes/
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

	path := fmt.Sprintf("%s?%s", nodesEndpoint, query.Encode())
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

	path := fmt.Sprintf("%s?%s", nodesEndpoint, query.Encode())
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

// interfaceListResponse represents the response body for GET /v3/simulations/nodes/interfaces
type interfaceListResponse struct {
	Results []Interface `json:"results"`
}

// GetNodeInterfaces retrieves interfaces for a specific node in a simulation
func (c *Client) GetNodeInterfaces(simulationID, nodeID string) ([]Interface, error) {
	logging.Verbose("GetNodeInterfaces: Fetching interfaces for node %s in simulation %s", nodeID, simulationID)

	endpoint := fmt.Sprintf("%s?simulation=%s&node=%s", nodeInterfacesEndpoint, simulationID, nodeID)
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
