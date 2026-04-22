# Phase 0: Research & Clarification

**Status**: Complete  
**Generated**: January 9, 2026

This document resolves all unknowns and clarifications needed before Phase 1 design begins.

## Research Tasks

### 1. API Endpoint & Authentication Specification

**Question**: What is the exact API endpoint URL and authentication mechanism for nvair.unifabric.io?

**Decision**: 
- **Primary Endpoint**: `https://api.dsx-air.nvidia.com/api`
- **API Versions**: Uses `/api/v3` endpoints for current CLI operations
- **Swagger Documentation**: https://api.dsx-air.nvidia.com/api/
- **Authentication Method**: API key sent directly in `Authorization: Bearer <apiToken>`
- **Token Exchange**: None; the CLI does not exchange API keys for separate bearer tokens
- **Token Storage**: Username, API key, and API endpoint in plaintext JSON config with `0600` file permissions
- **Token Expiry**: No local bearer-token expiry is tracked

**Rationale**: The platform API key is sufficient for authenticated CLI operations, so removing the derived bearer token avoids redundant credential storage and refresh logic.

**Alternatives Considered**: 
- SSH key-based API auth: Would add complexity; API key auth is simpler for platform calls
- Bearer-token exchange and refresh: Adds local state without providing value when the API key can be used directly

---

### 2. User SSH Key Generation and Upload

**Question**: How does the CLI manage user SSH keys for authentication to the platform?

**Decision**:
- **Key Generation**: On `nvair login`, CLI generates Ed25519 SSH key pair if not exists
- **Key Location**: 
  - Private key: `$HOME/.ssh/nvair.unifabric.io`
  - Public key: `$HOME/.ssh/nvair.unifabric.io.pub`
- **Key Permissions**: Private key with 0600, public key with 0644
- **Idempotency**: If key pair already exists, CLI uses existing keys (does not regenerate)
- **Upload to Platform**: 
  1. After authentication, CLI queries API to check if public key exists in user's account
  2. If not found, CLI uploads public key to user's nvidia account
  3. This enables SSH access to bastion hosts using this key pair

**Rationale**: Ed25519 provides strong security with smaller key size. Automatic generation and upload simplifies user experience—no manual SSH key setup required.

**Alternatives Considered**:
- Manual key setup: Would require additional user steps
- RSA keys: Ed25519 is more modern and efficient
- Always regenerate: Would invalidate existing uploaded keys

---

### 3. Simulation Topology File Format

**Question**: What format are simulation topology files (accepted by `nvair create -d`)?

**Decision**:
- **Primary Format**: YAML (.yaml, .yml)
- **Alternative Format**: JSON (.json)
- **Schema**: Follows network topology schema with nodes, links, and configuration
- **Validation**: CLI validates file exists and is valid YAML/JSON before upload

**Rationale**: YAML is human-readable and industry-standard for infrastructure definitions (Kubernetes, Terraform). JSON provides programmatic flexibility.

**Alternatives Considered**:
- Proprietary format: Would require custom parsers; YAML/JSON are standard
- Only JSON: YAML is more user-friendly for manual editing

---

### 4. SSH Key Management for Node Access

**Question**: How are SSH keys provisioned for accessing simulation nodes? How does SSH access work through a bastion host?

**Decision**:
- **Architecture**: SSH connections go through a bastion/jump host (oob-mgmt-server)
- **Bastion Password Reset**: Before first use, CLI must reset the bastion host password using SSH public key auth
- **Key Storage**: SSH private keys are provided by the nvair platform API
- **CLI Behavior**: 
  1. On first access, CLI retrieves SSH private key from platform API
  2. CLI resets bastion host password using `passwd` command via PTY session
  3. Subsequent connections use bastion as jump host with the new password
- **Key Types**: Support Ed25519 and RSA keys
- **Permissions**: Keys stored in `$HOME/.ssh/nvair/` with 0600 permissions

**Bastion Password Reset Flow**:
```
1. SSH to bastion with private key (no password)
2. Request PTY with proper terminal modes (ECHO=0)
3. Execute `passwd` command
4. Handle interactive prompts:
   - "Current password:" → send old password
   - "New password:" → send new password
   - "Retype new password:" → send new password again
5. Store new password for subsequent jump host connections
```

**Rationale**: Bastion host pattern provides security and centralized access control. Password reset ensures user-specific credentials.

**Alternatives Considered**:
- Direct node access: Would bypass platform security controls
- Manual password reset: Would require additional user steps
- SSH key forwarding only: Platform requires password-based bastion auth

---

### 4. Installation Commands Behavior

**Question**: What do the `nvair install *` commands actually do (Docker, Helm, etc.)? How do they connect to nodes?

**Decision**:
- **Connection Method**: All installations go through bastion host (SSH jump host pattern)
- **Installation Method**: CLI connects to each node via SSH (through bastion) and runs installation commands directly
- **Installation Scripts**: CLI embeds installation commands for each tool (not fetched from API)
- **Execution Flow**:
  1. Ensure bastion password is reset (see Q4)
  2. Connect to bastion host with user's SSH key
  3. From bastion, SSH to target nodes
  4. Execute installation commands directly (e.g., `curl -fsSL https://get.docker.com | sh`)
- **Progress Reporting**: CLI shows per-node status as operations complete
- **Error Handling**: If one node fails, CLI logs error and continues with remaining nodes; reports summary at end
- **Supported Tools**: Docker, kubeadm, Helm CLI, kube-prometheus-stack, Spiderpool, Unifabric

**Rationale**: Bastion host pattern matches platform security model. Direct SSH execution provides simplicity and reliability without API dependencies.

**Alternatives Considered**:
- Fetch scripts from API: Not available; CLI must embed installation logic
- Direct node access: Would bypass platform security controls
- Ansible/Terraform integration: Would add complexity; simple SSH commands sufficient

---

### 5. Configuration Directory Permissions

**Question**: What are the exact file permissions for the configuration directory and config file?

**Decision**:
- **Directory**: `$HOME/.config/nvair.unifabric.io/` with permissions 0700 (user rwx only)
- **Config File**: `config.json` with permissions 0600 (user rw only)
- **Auto-creation**: CLI creates directory and sets permissions on first login
- **Permission Enforcement**: On every config read, CLI verifies permissions are secure; warns user if insecure

**Rationale**: Protects credentials from unauthorized access by other users on shared systems.

**Alternatives Considered**:
- Standard permissions (0755): Would expose credentials to other users
- No permissions checking: Would be insecure; enforcement is critical

---

### 6. Error Handling & Network Resilience

**Question**: How should CLI handle network errors, timeouts, and transient failures?

**Decision**:
- **Timeout**: All API calls have 30-second timeout
- **Retry Logic**: Transient errors (5xx, connection timeouts) retry up to 3 times with exponential backoff (1s, 2s, 4s)
- **Permanent Errors**: 4xx errors fail immediately with user-friendly message
- **Network Loss During SSH**: If SSH connection drops mid-command, CLI shows error and exits (no auto-reconnect)
- **Verbose Mode**: `-v/--verbose` flag shows HTTP request/response for debugging

**Rationale**: Balances resilience with user experience. Exponential backoff prevents overwhelming platform. Verbose mode aids troubleshooting.

**Alternatives Considered**:
- No retries: Would fail on transient issues
- Infinite retries: Could hang indefinitely
- Auto-reconnect SSH: Could mask issues or create unintended duplicate commands

---

### 7. Output Table Formatting

**Question**: What are the exact table column widths and formatting rules?

**Decision**:
- **Column Alignment**: 
  - Text (Name, State, MgmtIP): left-aligned
  - ID (UUIDs): left-aligned
  - Timestamps: left-aligned
  - Numeric (if any): right-aligned
- **Column Spacing**: Minimum 2 spaces between columns
- **Long Values**: Truncate to terminal width with "..." if needed; use `-w/--wide` flag to disable truncation
- **Headers**: Bold and uppercase in supported terminals; plain uppercase in non-TTY output
- **Empty Results**: Display "No results found" message instead of empty table

**Rationale**: Balances readability with terminal constraints. Consistency across commands improves UX.

**Alternatives Considered**:
- Fixed column widths: Would waste space or truncate unnecessarily
- JSON output only: Would break human readability requirement

---

### 8. Command Exit Codes

**Question**: What exit codes should the CLI use for different scenarios?

**Decision**:
- **0**: Successful execution
- **1**: General error (invalid arguments, API errors, SSH errors)
- **2**: Configuration missing or invalid (not logged in)
- **3**: Network error (can't reach API)
- **4**: SSH access error (can't connect to node)
- **5**: Installation failure on one or more nodes

**Rationale**: Enables scripts to handle different failure modes appropriately.

**Alternatives Considered**:
- Single exit code: Would make scripting difficult
- Too many codes: Would be hard to document

---

### 9. Platform Dependencies

**Question**: What should be the minimum requirements and platform support?

**Decision**:
- **Go Version**: 1.22+ (current LTS)
- **Operating Systems**: Linux, macOS 11+, Windows 10+
- **System Requirements**: 
  - OpenSSH client (optional; CLI uses x/crypto/ssh)
  - curl or wget (optional for script bootstrap)
- **Package Distribution**: GitHub Releases + Homebrew/Scoop; single static binary

**Rationale**: Go provides single static binary, cross-platform support, and fast startup time, ideal for CLI tools.

**Alternatives Considered**:
- Python: Requires runtime and dependency management; complex distribution
- Rust: Steeper learning curve; SSH ecosystem more fragmented

---

### 11. Service Port Forwarding Sync

**Question**: How does the CLI sync Kubernetes NodePort services to Air service forwarding rules?

**Decision**:
- **Command**: `nvair forward sync -s <simulation-name> --control-plane <node-name>`
- **All Namespaces**: Syncs all NodePort services across all namespaces (no namespace filter)
- **Service Naming**: Air services created with `k8s-` prefix (e.g., `k8s-default-my-app-30080`)
- **Nginx Proxy**: CLI first ensures nginx is installed on bastion host (oob-mgmt-server)
- **K8s Discovery**: CLI SSHs to control plane node and executes `kubectl get svc --all-namespaces` to query all NodePort services
- **Nginx Configuration**: CLI generates nginx stream blocks to proxy traffic from bastion to K8s NodePort services
- **Air Service Creation**: CLI creates Air service forwarding rules pointing to bastion's nginx ports
- **Sync Strategy**: 
  1. Connect to bastion via SSH
  2. Check/install nginx on bastion
  3. SSH from bastion to control plane node
  4. Execute `kubectl get svc --all-namespaces -o json` remotely
  5. Parse JSON output to extract all NodePort services
  6. Generate nginx stream config for each NodePort
  7. Write config to `/etc/nginx/streams.d/k8s-nodeports.conf`
  8. Reload nginx
  9. Create Air service for each NodePort with `k8s-` prefix (pointing to bastion)
- **Traffic Flow**: External → Air Service (k8s-*) → Bastion (nginx) → K8s NodePort → Pod
- **No Kubeconfig Needed**: Uses remote kubectl execution instead of local kubeconfig

**Rationale**: Using bastion as nginx proxy provides centralized traffic routing. `k8s-` prefix prevents naming conflicts with other Air services. Syncing all namespaces ensures complete cluster visibility. Remote kubectl execution simplifies authentication (no kubeconfig needed). Nginx handles both TCP and UDP protocols.

**Alternatives Considered**:
- Direct Air service to K8s nodes: Would bypass bastion security model
- Local kubeconfig: Requires kubeconfig distribution; remote kubectl is simpler
- Manual nginx configuration: Too error-prone; auto-generation ensures consistency

---

### 12. Configuration Refresh & Token Expiry

**Question**: How does CLI handle expired tokens and configuration refresh?

**Decision**:
- **Authentication Check**: Commands verify that local configuration contains an API key before making authenticated requests
- **Token Exchange**: None; authenticated requests use the stored API key directly

**Rationale**: The API key is the durable credential. Removing bearer-token refresh avoids unnecessary background auth calls and reduces redundant secret storage.

**Alternatives Considered**:
- Manual re-login on expiry: Would be disruptive to user workflow
- Never expire: Would be insecure
- Inactivity-based logout: Not applicable for CLI tools (commands are short-lived)

---

## Summary of Decisions

All research questions have been resolved with clear, implementable decisions. No NEEDS CLARIFICATION items remain. The technical approach is standard and well-established in CLI tool development.

### Key Technical Stack Confirmed

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| CLI Framework | Cobra | Mature CLI framework with clear command/subcommand structure |
| HTTP Client | Resty (or net/http) | Convenient retry, timeout and middleware support |
| SSH Client | golang.org/x/crypto/ssh | Pure Go, no external dependencies |
| Table Format | go-pretty/table | Excellent terminal table rendering |
| Config Storage | JSON | Simple, readable, standard format |
| Testing | `go test` | Native Go testing, easy CI integration |

All decisions enable P1 user stories to be independently implemented and tested without major rework.
