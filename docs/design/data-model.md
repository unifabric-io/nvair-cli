# Phase 1: Data Model

**Status**: Complete  
**Generated**: January 9, 2026

This document defines all data structures, entities, and their relationships used in the nvair CLI.

## JSON Naming Conventions

- **Local Configuration Files** (stored on user's machine): Use **camelCase** (e.g., `bearerToken`, `apiEndpoint`)
- **API Requests/Responses** (from nvair platform): Use **snake_case** (e.g., `bearer_token`, `api_endpoint`)

This follows common conventions:
- Local files use JavaScript/TypeScript style (camelCase)
- API follows Python/REST style (snake_case)

## Core Entities

### User (Implicit - Authentication Identity)

Represents an authenticated user accessing the platform (implicit, only persisted in configuration).

```go
type User struct {
    Username string // login username
    APIToken string // API token issued by platform
}
```

---

### Configuration

Represents authentication configuration stored on user's local machine.

```go
type Configuration struct {
    Username              string    `json:"username"`
    APIToken              string    `json:"apiToken"`              // Platform API token for refresh
    BearerToken           string    `json:"bearerToken"`
    BearerTokenExpiresAt  time.Time `json:"bearerTokenExpiresAt"`
    APIEndpoint           string    `json:"apiEndpoint"` // default: https://nvair.unifabric.io/api
}

func (c Configuration) IsTokenExpired(now time.Time) bool {
    return !c.BearerTokenExpiresAt.After(now)
}
```

**Storage Format (JSON)**:
```json
{
  "username": "user@example.com",
  "apiToken": "nvair_token_abc123...",
  "bearerToken": "eyJhbGc...",
  "bearerTokenExpiresAt": "2026-01-10T12:00:00Z",
  "apiEndpoint": "https://nvair.unifabric.io/api"
}
```

---

### UserSSHKey

Represents user's SSH key pair for platform authentication.

```go
type UserSSHKey struct {
    PrivateKeyPath string // $HOME/.ssh/nvair.unifabric.io
    PublicKeyPath  string // $HOME/.ssh/nvair.unifabric.io.pub
    Fingerprint    string // SHA256 fingerprint
    KeyType        string // "ed25519"
}

func (k UserSSHKey) Exists() bool {
    return fileExists(k.PrivateKeyPath) && fileExists(k.PublicKeyPath)
}

func (k UserSSHKey) ReadPublicKey() (string, error) {
    bytes, err := os.ReadFile(k.PublicKeyPath)
    return string(bytes), err
}
```

---

### SSHPublicKey

Represents an SSH public key registered in user's nvidia account.

```go
type SSHPublicKey struct {
    ID          string    `json:"id"`
    Fingerprint string    `json:"fingerprint"`
    PublicKey   string    `json:"public_key"`
    Label       string    `json:"label"`
    CreatedAt   time.Time `json:"created_at"`
    LastUsed    time.Time `json:"last_used,omitempty"`
}
```

---

### Simulation

Represents a network simulation environment.

```go
type Simulation struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    State     string    `json:"state"`  // STORED, LOADED, etc. (read-only, managed by platform)
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
    NodeCount int       `json:"node_count"`
}

func (s Simulation) IsExpired(now time.Time) bool {
    return !s.ExpiresAt.After(now)
}
```

**API Response Example**:
```json
{
  "id": "abc-def-123",
  "name": "demo",
  "state": "LOADED",
  "created_at": "2026-01-06T08:06:46.262185Z",
  "expires_at": "2026-01-20T08:06:46.261668Z",
  "node_count": 3
}
```

---

### Node

Represents a virtual machine/container node in the simulation.

```go
type Node struct {
    ID            string   `json:"id"`
    SimulationID  string   `json:"simulation_id"`
    Name          string   `json:"name"`
    State         string   `json:"state"`         // RUNNING, STOPPED, etc. (read-only)
    ManagementIP  string   `json:"management_ip"`
    Roles         []string `json:"roles"`
}

func (n Node) IsAccessible() bool {
    return n.State == "RUNNING" && n.ManagementIP != ""
}
```

**API Response Example**:
```json
{
  "id": "2584a1a2-0cad-4da6-8ac6-7c043cada39d",
  "simulation_id": "abc-def-123",
  "name": "node2",
  "state": "RUNNING",
  "management_ip": "192.168.200.7",
  "roles": ["worker"]
}
```

---

### RemoteCommand

Represents a remote command to be executed on a node.

```go
type RemoteCommand struct {
    Node          Node
    Command       string
    TimeoutSec    int // default: 300
}

type RemoteCommandResult struct {
    ExitCode      int
    Stdout        string
    Stderr        string
    ElapsedSec    float64
    TimedOut      bool
}
```

---

### BastionHost

Represents the jump host/bastion server for SSH access to simulation nodes.

```go
type BastionHost struct {
    SimulationID  string `json:"simulation_id"`
    Hostname      string `json:"hostname"`
    Port          int    `json:"port"`
    Username      string `json:"username"`        // typically "ubuntu" or "cumulus"
    PrivateKey    string `json:"-"`               // SSH private key (not serialized)
    Password      string `json:"-"`               // User-set password (not serialized)
}
```

---

### SSHConfig

Represents SSH configuration for accessing nodes through bastion.

```go
type SSHConfig struct {
    SimulationID     string
    BastionHost      BastionHost
    TargetNode       Node
    PrivateKeyPath   string        // Path to cached SSH private key
    Timeout          time.Duration // Default: 30s
}
```

---

### PasswordResetTask

Represents a bastion host password reset operation.

```go
type PasswordResetTask struct {
    Bastion     BastionHost
    OldPassword string  // Initial/default password from platform
    NewPassword string  // User-specified new password
    Timeout     time.Duration
}

type PasswordResetResult struct {
    Success bool
    Message string
    Error   error
}
```

---

### InstallationTask

Represents a software installation task executed on a group of nodes.

```go
type SoftwareType string

const (
    SoftwareDocker              SoftwareType = "docker"
    SoftwareKubeadm             SoftwareType = "kubeadm"
)

type InstallationTask struct {
    Software         SoftwareType      `json:"software"`
    Nodes            []Node            `json:"-"`
    SimulationID     string            `json:"simulation_id"`
    Version          string            `json:"version,omitempty"`          // Software version to install
    AdditionalConfig map[string]any    `json:"config,omitempty"`
}

type InstallationStatus string

const (
    InstallSuccess InstallationStatus = "SUCCESS"
    InstallFailed  InstallationStatus = "FAILED"
    InstallPartial InstallationStatus = "PARTIAL"
)

type InstallationResult struct {
    Software  SoftwareType       `json:"software"`
    Node      Node               `json:"-"`
    Status    InstallationStatus `json:"status"`
    Message   string             `json:"message"`
    ElapsedSec float64           `json:"elapsed_seconds"`
}
```

---

### K8sNodePortService

Represents a Kubernetes NodePort service to be synced.

```go
type K8sNodePortService struct {
    Namespace   string            `json:"namespace"`
    Name        string            `json:"name"`
    NodePort    int               `json:"nodePort"`
    TargetPort  int               `json:"targetPort"`
    Protocol    string            `json:"protocol"` // TCP or UDP
    Selector    map[string]string `json:"selector"`
    Labels      map[string]string `json:"labels"`
}
```

---

### AirService

Represents an NVIDIA Air service forwarding rule.

```go
type AirService struct {
    ID           string `json:"id"`
    Name         string `json:"name"`              // Format: "k8s-{namespace}-{service}-{nodeport}"
    SimulationID string `json:"simulation"`
    InterfaceID  string `json:"interface"`
    DestPort     int    `json:"dest_port"`
    SrcPort      int    `json:"src_port,omitempty"` // Assigned by Air
    ServiceType  string `json:"service_type"`        // "ssh", "tcp", "udp"
    Link         string `json:"link,omitempty"`      // External access URL
    NodeName     string `json:"node_name,omitempty"`
}
```

**Note**: Services synced from K8s always have `k8s-` prefix to avoid naming conflicts.

---

### ForwardSyncTask

Represents a service port forwarding sync operation.

```go
type ForwardSyncTask struct {
    SimulationID     string
    K8sServices      []K8sNodePortService
    ExistingAirSvcs  []AirService
}

type ForwardSyncResult struct {
    Created  []AirService
    Updated  []AirService
    Deleted  []AirService
    Failed   []ForwardSyncError
    Summary  string
}

type ForwardSyncError struct {
    ServiceName string
    Error       error
    Message     string
}
```

---

## Relationships

```
User (1) ──────── (1) Configuration
              (stored locally)

User (1) ──────── (∞) Simulation
              (owns)

Simulation (1) ──────── (∞) Node
                    (contains)

Node (∞) ──────── (1) Simulation
              (belongs to)

RemoteCommand (∞) ──────── (1) Node
                      (targets)

InstallationTask (1) ──────── (∞) Node
                        (configures)
```

---

## State Machines

### Simulation States

Simulation states are managed by the nvair platform. The CLI does not handle state transitions.

**Platform States**:
- `STORED` - Simulation exists but is not loaded
- `LOADED` - Simulation is loaded and nodes are accessible
- (Other states: `LOAD`, `STORE`, `DESTROY`, `DUPLICATE`, `EXTEND`, `REBUILD` - transitional)

**CLI Behavior**:
- `nvair create -d <topology-dir>` - Creates simulation and automatically sets to LOADED state
- CLI only reads the state, does not modify it
- No state transition logic in CLI

---

## Validation Rules

### Configuration
- `username` must be non-empty
- `bearer_token` must be valid JWT format
- `bearer_token_expires_at` must be in future or past but recent
- `api_endpoint` must be valid HTTPS URL

### Simulation
- `name` must be 1-63 characters, lowercase, alphanumeric + hyphens
- `state` must be one of defined enum values
- `created_at` must be ≤ current time
- `expires_at` must be > `created_at`
- `node_count` must be > 0

### Node
- `name` must be 1-63 characters, valid hostname
- `management_ip` must be valid IPv4 or IPv6 address
- `state` must be one of defined enum values
- `roles` if present, must be non-empty list of valid role names

### RemoteCommand
- `command` must be non-empty string
- `timeout_seconds` must be 1-3600 (1 second to 1 hour)

---

## Cluster Configuration Format

For network cluster deployments, nvair supports a simplified YAML configuration format (`cluster.yaml`) that defines nodes and switches with flexible command execution.

### ClusterConfig

```go
type ClusterConfig struct {
    Metadata ClusterMetadata `yaml:"metadata"`
    Nodes    ClusterNodes    `yaml:"nodes"`
    Switches ClusterSwitches `yaml:"switches"`
    Settings ClusterSettings `yaml:"settings"`
}

type ClusterMetadata struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Version     string `yaml:"version"`
}

type ClusterNodes struct {
    GPU     []ClusterNode `yaml:"gpu,omitempty"`
    Storage []ClusterNode `yaml:"storage,omitempty"`
}

type ClusterNode struct {
    Name       string              `yaml:"name"`
    Interfaces []NetworkInterface  `yaml:"interfaces"`
}

type NetworkInterface struct {
    Name        string `yaml:"name"`
    IP          string `yaml:"ip"`
    VLAN        int    `yaml:"vlan"`
    Description string `yaml:"description,omitempty"`
}

type ClusterSwitches struct {
    LeafGPU     []ClusterSwitch `yaml:"leafGPU,omitempty"`
    LeafStorage []ClusterSwitch `yaml:"leafStorage,omitempty"`
    SpineGPU    []ClusterSwitch `yaml:"spineGPU,omitempty"`
}

type ClusterSwitch struct {
    Name string   `yaml:"name"`
    Exec []string `yaml:"exec"` // Commands to execute on switch
}

type ClusterSettings struct {
    MTU    int               `yaml:"mtu,omitempty"`
    Sysctl map[string]any    `yaml:"sysctl,omitempty"`
}
```

### Design Philosophy

The cluster configuration uses a **command-based approach** with the `exec` field instead of abstracting specific configuration fields (like vlans, ports, svis, bgp). This provides:

1. **Flexibility**: Users can execute any NVUE (NVIDIA User Experience) commands directly
2. **Simplicity**: No need to maintain complex abstraction layers for switch configurations
3. **Transparency**: Configuration commands are explicit and match actual switch CLI
4. **Extensibility**: Easy to add new configurations without schema changes

### Example cluster.yaml

```yaml
metadata:
  name: simple-gpu-cluster
  description: GPU cluster with spine-leaf architecture
  version: "1.0"

nodes:
  gpu:
    - name: node-gpu-1
      interfaces:
        - name: eth1
          ip: 172.17.1.11/24
          vlan: 0
          description: "GPU fabric - leaf1 group"

switches:
  leafGPU:
    - name: switch-gpu-leaf1
      exec:
        - "nv set interface swp2 bridge domain br_default"
        - "nv set bridge domain br_default vlan 10,20"
        - "nv config apply"
    - name: switch-gpu-leaf2
      exec: []

settings:
  mtu: 9000
  sysctl:
    net.ipv4.ip_forward: 1
```

---

## Serialization

All entities support serialization to:
- **Go struct**: In-memory representation with `encoding/json` tags for JSON mapping
- **JSON**: API requests/responses and configuration storage
- **YAML**: Topology files (for simulation creation) and cluster configuration
- **Table output**: CLI terminal display (using go-pretty/table)
