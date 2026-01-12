# Quickstart Guide for nvair CLI Development

## Project Overview

The nvair CLI is a command-line tool that provides access to the [air.nvidia.com](https://air.nvidia.com/) platform for network simulation management. Key features:

- **Authentication**: Login once, automatic credential handling
- **Simulation Management**: Create and list network simulations
- **Node Discovery**: View nodes within simulations with IP addresses
- **Remote Execution**: Execute arbitrary commands on nodes via SSH
- **Software Installation**: Batch install Docker, Kubernetes, and supporting tools

## Prerequisites

### System Requirements

For end users:
- Linux, macOS 11+, or Windows 10+
- Single static binary distribution (no runtime required)
- A valid NVIDIA Air API token (for commands that access your account)

### Create NVIDIA Air API token

1. Go to `https://air.nvidia.com`
2. Click the settings icon in the upper right corner
3. Give your API token a name and set an expiration date
4. Confirm to create the token

![](./images/create-token.png)

## Core Concepts

The following core concepts are used throughout the nvair CLI documentation and design:

- **Simulation**: A Simulation is a network environment created on the NVIDIA Air platform. It contains a set of virtual Nodes and the network topology; it does not include application-level services.

- **Node**: A Node is an individual virtual device within a Simulation (for example, a GPU server, storage node, or switch). It represents infrastructure with attributes like name and management IP; it does not represent the applications running on that device.

- **Service**: A Service represents a network endpoint exposed within a Simulation (for example via port forwarding or Kubernetes NodePort). It denotes an externally reachable service or rule, not the underlying Node.

Short: Simulation = a network environment (many Nodes); Node = a single virtual device; Service = an externally exposed network service.

## CLI Command Reference

```text
nvcli <command> [options]

Commands:

  login                           Authenticate with NVIDIA Air platform
      -u, --username <string>     Username (email) for authentication
      -p, --password <string>     API token for authentication (get from https://air.nvidia.com/settings)
      
  create                          Create a simulation from topology directory
      -d, --directory <path>      Directory containing topology.json and config files
      --dry-run                   Validate configuration files without creating simulation
      
  get                              Get resources (simulations, nodes, services)
    simulation                     List all simulations (alias: simulations)
    node                           List nodes in a simulation (alias: nodes)
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
    service                        List services for a simulation (alias: services)
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
       
  delete                           Delete resources
    simulation                     Delete a simulation
      <name>                       Simulation name (required)
    service                        Delete a service forwarding rule
      <name>                       Service Name (required)
       
  exec                             Execute commands on nodes via SSH
      <node-name>                  Node name (required)
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
      --                           Separator before command
      <command>                    Command to execute (required)
       
  install                          Install software on simulation nodes
    docker                         Install Docker on all nodes
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
      --version <string>           Docker version to install (optional)
    kubeadm                        Install Kubernetes with kubeadm
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
      --version <string>           Kubernetes version to install (optional)
      --control-plane-node <nodes> Control plane nodes (comma-separated)
      --worker-node <nodes>        Worker nodes (comma-separated)
      
  forward                          Service forwarding management
    sync                           Sync Kubernetes NodePort services to Air
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
      --control-plane <node>       Control plane node name (required)

Global Options:
  -v, --verbose                    Enable verbose output (API requests, SSH details)
  -h, --help                       Show help for command
  --version                        Show version information
```

## Usage Examples

```bash
# Authenticate
nvcli login -u user@example.com -p <api-token>

# Validate topology config (dry-run)
nvcli create -d examples/simple/ --dry-run

# Create simulation
nvcli create -d examples/simple/

# List simulations
nvcli get simulation

# Execute command on a node
nvcli exec node-gpu-1 -s my-cluster -- hostname

# Install Docker on all nodes
nvcli install docker -s my-cluster --version 24.0.7

# Sync Kubernetes services
nvcli forward sync -s my-cluster --control-plane node1
```

## Troubleshooting

### Authentication errors
- Ensure your API token is valid and has required scopes. Re-run `nvcli login -u <email> -p <api-token>`.

### SSH connection failures
- Verify the node's management IP is reachable from your network.
- If firewall or network blocks exist, use a reachable bastion host or check VPN settings.

### Command timeout or unexpected errors
- Re-run with `-v` to get verbose logs and check the API / SSH details.

## Getting Help

- For developer-focused setup (build, tests, CI): `docs/development.md`
- For API contract and data model: `docs/design/contracts/api.md` and `docs/design/data-model.md`
- For examples and topologies: `examples/` and `docs/design/topology-examples.md`
