# Quickstart

## Overview

The nvair CLI is a command-line tool that provides access to the [air.nvidia.com](https://air.nvidia.com/) platform for network simulation management. Key features:

* **Authentication**: Log in once with automatic credential handling
* **Simulation Management**: Create, delete, and view network simulations
* **Node Discovery**: View nodes within simulations along with their IP addresses
* **Remote Execution**: Easily execute commands on nodes via SSH
* **Port Forwarding**: Manage port forwarding rules more conveniently
* **Rich Examples**: Provides example topologies for AI data center scenarios


## Prerequisites

### Requirements

- Linux, macOS 11+, or Windows 10+
- A valid NVIDIA Air API token (for commands that access your account)

### Create NVIDIA Air API token

1. Go to `https://air.nvidia.com`
2. Click the settings icon in the upper right corner
3. Give your API token a name and set an expiration date
4. Confirm to create the token

![](./images/create-token.png)

## Core Concepts

The following core concepts are used throughout the nvair CLI documentation and design:

- **Simulation**: Simulation is a simulated environment created on the NVIDIA Air platform. It consists of multiple nodes and switches.

- **Node**: Node represents a node in the topology, such as a Linux host or a Cumulus switch.

- **Forward**: A port forwarding rule for a simulation. It maps external ports to internal ports (e.g., Kubernetes API Server and other TCP/UDP workloads).

## CLI Command Reference

``` bash
nvair <command> [options]

Commands:

  login                            Authenticate with NVIDIA Air platform
      -u, --username <string>      Username (email) for authentication
      -p, --password <string>      API token for authentication (get from https://air.nvidia.com/settings)
      
  create                           Create a simulation from topology directory
      -d, --directory <path>       Directory containing topology.json and config files
      --dry-run                    Validate configuration files without creating simulation
      
  get                              Get resources (simulations, nodes, forwards)
    simulation                     List all simulations (alias: simulations)
    node                           List nodes in a simulation (alias: nodes)
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
    forward                        List port forward for a simulation (alias: forward)
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
       
  delete                           Delete resources
    simulation                     Delete a simulation
      <name>                       Simulation name (required)
    forward                        Delete a port forwarding rule
      -p, --port <port>            Forwarding port
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
       
  exec                             Execute commands on nodes via SSH
      <node-name>                  Node name (required)
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
      --                           Separator before command
      <command>                    Command to execute (required)
       
  add                              Add resources
    forward                        Add a port forwarding rule 
      -s, --simulation <name>      Simulation name (optional, defaults to first simulation)
      -p, --port <port>            Forwarding port

Global Options:
  -v, --verbose                    Enable verbose logging for debugging
  -h, --help                       Show help for command
  --version                        Show version information
```

> The create command performs the following main steps:
> 1. Load and validate topology configuration from the specified directory
> 2. Create the simulation on NVIDIA Air platform
> 3. Set simulation state to 'load' and wait for initialization jobs
> 4. Configure SSH access through the bastion host
> 5. Reset passwords and apply configurations to switches
>    - This resets the bastion password to `dangerous` and the switch passwords to a known value. It does not affect normal use: password login is not used in practice, and the reset only skips the forced password-change prompt.
> 6. Upload and apply Netplan configurations to Linux nodes
>
> Note: NVIDIA Air automatically provides a bastion (jump) machine in your topology as an additional built-in node for secure access to other nodes.

## Usage Examples

```bash
# Authenticate
nvair login -u user@example.com -p <api-token>

# Authenticate with verbose logging (for debugging)
nvair --verbose login -u user@example.com -p <api-token>

# Validate topology config (dry-run)
nvair create -d examples/simple/ --dry-run

# Create simulation
nvair create -d examples/simple/

# List simulations
nvair get simulation

# Add a port forwarding rule to a simulation
nvair add forward -p 6443
nvair add forward -p 30032

# List port forwarding rule in a simulation
nvair get forward

# Delete a port forwarding rule by service name
nvair delete forward -p 6443

# Execute command on a node
nvair exec node-gpu-1 -s my-cluster -- hostname
```

## Verbose Mode

Enable verbose logging with the `--verbose` or `-v` global flag to get detailed information for debugging:

```bash
# Login with verbose output
nvair --verbose login -u user@example.com -p <api-token>

# Example verbose output shows:
# [DEBUG] [2026-02-10 10:23:45] Verbose mode enabled
# [DEBUG] [2026-02-10 10:23:45] Login command started with username: user@example.com
# [DEBUG] [2026-02-10 10:23:45] Flags validated successfully
# [DEBUG] [2026-02-10 10:23:45] Step 1/6: Authenticating with API endpoint: https://air.nvidia.com/api
# [DEBUG] [2026-02-10 10:23:45] doRequest: [Attempt 1/3] POST https://air.nvidia.com/api/v1/login/
# [DEBUG] [2026-02-10 10:23:45] doRequest: Request body: {"username":"user@example.com","password":"..."}
# ...
```

Verbose mode logs are printed to stderr and include timestamps. This is especially useful for:
- Debugging authentication failures
- Troubleshooting network connectivity issues
- Understanding SSH key generation process
- Inspecting API request/response details
- Analyzing retry behavior on transient failures

## Troubleshooting

### Authentication errors
- Ensure your API token is valid and has required scopes. Re-run `nvair login -u <email> -p <api-token>`.
- Use `nvair --verbose login -u <email> -p <api-token> -v` to see detailed API request/response information.

### SSH connection failures
- Verify the node's management IP is reachable from your network.
- If firewall or network blocks exist, use a reachable bastion host or check VPN settings.
- Use `nvair --verbose` to check SSH key generation and registration details.

### Command timeout or unexpected errors
- Re-run with `--verbose` to get detailed logs including:
  - API endpoint calls and response codes
  - SSH key fingerprints and registration status
  - Network retry attempts and backoff timing
  - Configuration file operations

## Next Steps

- For example topologies and usage samples, see the [examples directory](../examples/) and [examples guide](../examples/README.md).
- For developer-focused setup, including build, tests, and CI, see the [development guide](development/development.md).
- For API details and the data model, see the [API contract](design/contracts/api.md) and [data model](design/data-model.md).
