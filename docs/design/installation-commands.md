# Installation Commands Reference

**Status**: Technical Reference  
**Generated**: January 12, 2026

This document defines the installation commands embedded in the nvair CLI for various software tools.

## Overview

The CLI installs software by executing commands directly on target nodes via SSH (through bastion host). Installation logic is **embedded in the CLI code**, not fetched from an API.

## Installation Architecture

```
┌─────────┐    SSH + Private Key     ┌──────────────┐
│   CLI   │ ──────────────────────>  │   Bastion    │
└─────────┘                          │ oob-mgmt-svr │
                                     └──────┬───────┘
                                            │ SSH
                                            │
                                            v
                                     ┌──────────────┐
                                     │ Target Nodes │
                                     │ (docker,     │
                                     │  kubeadm...) │
                                     └──────────────┘
```

## Supported Tools

### 1. Docker

**Command**: `nvair install docker -s <simulation-name> [--version <version>]`

**Parameters**:
- `-s, --simulation`: Simulation name (required)
- `--version`: Docker version to install (optional, defaults to latest stable)

**Installation Steps**:
```bash
# Download and execute Docker installation script
curl -fsSL https://get.docker.com -o get-docker.sh

# Install specific version if provided
if [ -n "$VERSION" ]; then
  sudo sh get-docker.sh --version $VERSION
else
  sudo sh get-docker.sh
fi

# Add current user to docker group
sudo usermod -aG docker $USER

# Enable and start Docker service
sudo systemctl enable docker
sudo systemctl start docker

# Verify installation
docker --version
```

**Target Nodes**: All nodes in simulation (unless specified with `--nodes` flag)

**Prerequisites**: 
- Ubuntu/Debian-based OS
- Internet access on nodes
- sudo privileges

---

### 2. Kubeadm

**Command**: `nvair install kubeadm --control-plane-nodes <node1,node2> --worker-nodes <node3,node4> -s <simulation-name> [--version <version>]`

**Parameters**:
- `--control-plane-nodes`: Comma-separated list of control plane nodes
- `--worker-nodes`: Comma-separated list of worker nodes
- `-s, --simulation`: Simulation name (required)
- `--version`: Kubernetes version to install (optional, defaults to latest stable)

**Installation Steps**:
```bash
# Update package index
sudo apt-get update

# Install dependencies
sudo apt-get install -y apt-transport-https ca-certificates curl

# Add Kubernetes apt repository
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.28/deb/Release.key | \
  sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg

echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.28/deb/ /' | \
  sudo tee /etc/apt/sources.list.d/kubernetes.list

# Install kubeadm, kubelet, kubectl
sudo apt-get update

if [ -n "$VERSION" ]; then
  sudo apt-get install -y kubelet=$VERSION kubeadm=$VERSION kubectl=$VERSION
else
  sudo apt-get install -y kubelet kubeadm kubectl
fi

# Hold packages at current version
sudo apt-mark hold kubelet kubeadm kubectl

# Verify installation
kubeadm version
```

**Target Nodes**: 
- Control plane nodes (specified with `--control-plane-nodes`)
- Worker nodes (specified with `--worker-nodes`)

**Prerequisites**:
- Docker already installed
- At least 2 CPUs on control plane nodes
- Swap disabled (`sudo swapoff -a`)

---

## Implementation Guidelines

### CLI Installation Flow

```go
func InstallTool(tool string, nodes []Node, bastionHost BastionHost) error {
    for _, node := range nodes {
        // 1. Connect to bastion
        bastionClient := connectToBastion(bastionHost)
        
        // 2. SSH from bastion to target node
        nodeSession := bastionClient.NewSession(node.ManagementIP)
        
        // 3. Execute installation commands
        commands := getInstallCommands(tool)
        for _, cmd := range commands {
            output, err := nodeSession.Run(cmd)
            if err != nil {
                logError(node, cmd, err)
                continue
            }
            logSuccess(node, cmd, output)
        }
        
        // 4. Verify installation
        if !verifyInstallation(nodeSession, tool) {
            return fmt.Errorf("installation verification failed for %s", tool)
        }
    }
    return nil
}
```

### Command Embedding

```go
var installCommands = map[string][]string{
    "docker": {
        "curl -fsSL https://get.docker.com -o get-docker.sh",
        "sudo sh get-docker.sh",
        "sudo usermod -aG docker $USER",
        "sudo systemctl enable docker",
        "sudo systemctl start docker",
    },
    "kubeadm": {
        "sudo apt-get update",
        "sudo apt-get install -y apt-transport-https ca-certificates curl",
        // ... more commands
    },
    // ... other tools
}
```

### Error Handling

- **Network Errors**: Retry up to 3 times with exponential backoff
- **Command Failures**: Log error, continue with remaining nodes
- **Verification Failures**: Mark installation as failed, report to user
- **Partial Success**: Report which nodes succeeded and which failed

### Progress Reporting

```
Installing docker on 3 nodes...
[1/3] node1 (192.168.1.10): Installing... ✓ (45s)
[2/3] node2 (192.168.1.11): Installing... ✓ (43s)
[3/3] node3 (192.168.1.12): Installing... ✗ Failed: connection timeout

Summary:
  ✓ 2 succeeded
  ✗ 1 failed

Failed nodes:
  - node3: connection timeout
```

## Version Management

Installation commands use specific versions where appropriate:

- **Kubernetes**: v1.28 (stable)
- **Helm**: Latest stable (v3.x)
- **Docker**: Latest stable from get.docker.com
- **Helm Charts**: Latest compatible versions

**Note**: Versions should be configurable via CLI flags or environment variables for flexibility.

## Testing

### Unit Tests

Mock SSH session to test command execution:
```go
func TestDockerInstallation(t *testing.T) {
    mockSession := newMockSSHSession()
    mockSession.ExpectCommand("curl -fsSL https://get.docker.com -o get-docker.sh")
    mockSession.ExpectCommand("sudo sh get-docker.sh")
    
    err := installDocker(mockSession)
    assert.NoError(t, err)
    assert.True(t, mockSession.AllCommandsExecuted())
}
```

### Integration Tests

Test against real simulation nodes (in test environment):
- Verify tool is actually installed
- Verify tool is functional (e.g., `docker ps` works)
- Verify cleanup (can uninstall)

---

**References**:
- Docker installation: https://get.docker.com
- Kubernetes documentation: https://kubernetes.io/docs/setup/

