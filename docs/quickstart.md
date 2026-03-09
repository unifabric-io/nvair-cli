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
      -o, --output <json|yaml>     Structured output format (optional)
    node                           List nodes in a simulation (alias: nodes)
      -s, --simulation <name>      Simulation name (required)
      -o, --output <json|yaml>     Structured output format (optional)
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

```log
# login
$ nvair login -u user@example.com -p <api-token>
✓ Login successful. Credentials saved to /home/node/.config/nvair.unifabric.io/config.json

# Validate topology config (dry-run)
$ nvair create -d examples/simple/ --dry-run
✓ Topology loaded and validated (dry-run)

# Create simulation
$ nvair create -d examples/simple/
✓ Topology loaded and validated
✓ Simulation created successfully. ID: cbfa2bbf-7714-478c-90a8-e27de9ce5a99, Name: simple
✓ Simulation state set to 'load', result: success, jobs: [5207a0da-7487-4b46-9304-61e520d7590a f19e7fa0-f7c5-4c63-b8fe-f6e5df717fdd 5a6a52c7-b266-460a-b594-571d69fe2e40 d0c93a94-6a75-475d-8f6e-588e81fcfd6a b8dc5f99-4d7b-40b9-904a-2b497fcb88b2]
Waiting for 5 jobs to complete ...
✓ All jobs completed successfully.
✓ Found outbound interface on oob-mgmt-server. Interface ID: 93164154-74b1-461a-a095-5f9f0684d3cc
✓ SSH service created successfully. Host: worker02.air.nvidia.com, Port: 11084
Waiting for SSH access to become ready...
Resetting switches passwords via bastion...
ping -c1 -W6 192.168.200.113 ...
ping -c1 -W6 192.168.200.114 ...
ping -c1 -W6 192.168.200.112 ...
ping -c1 -W6 192.168.200.111 ...
ping -c1 -W6 192.168.200.131 ...
ping -c1 -W6 192.168.200.121 ...
Switch switch-gpu-leaf3 reachable, updating password...
Switch switch-gpu-leaf1 reachable, updating password...
Switch switch-gpu-leaf4 reachable, updating password...
Switch switch-storage-leaf1 reachable, updating password...
Switch switch-gpu-spine1 reachable, updating password...
Switch switch-gpu-leaf2 reachable, updating password...
✓ Switch switch-gpu-leaf3 password updated.
✓ Switch switch-gpu-leaf1 password updated.
✓ Switch switch-gpu-leaf4 password updated.
✓ Switch switch-storage-leaf1 password updated.
✓ Switch switch-gpu-leaf2 password updated.
✓ Switch switch-gpu-spine1 password updated.
✓ Switch password reset completed.
Copying switch configs via bastion
Copying config for switch switch-gpu-leaf2 (192.168.200.112)...
Copying config for switch switch-gpu-spine1 (192.168.200.121)...
Copying config for switch switch-storage-leaf1 (192.168.200.131)...
Copying config for switch switch-gpu-leaf1 (192.168.200.111)...
Copying config for switch switch-gpu-leaf3 (192.168.200.113)...
Copying config for switch switch-gpu-leaf4 (192.168.200.114)...
✓ Config copied to switch switch-gpu-leaf4.
✓ Config copied to switch switch-storage-leaf1.
✓ Config copied to switch switch-gpu-spine1.
✓ Config copied to switch switch-gpu-leaf2.
✓ Config copied to switch switch-gpu-leaf1.
✓ Config copied to switch switch-gpu-leaf3.
✓ Switch configs copied.
Applying switch configs on switches...
✓ Config applied on switch switch-gpu-leaf3.
✓ Config applied on switch switch-gpu-leaf4.
✓ Config applied on switch switch-storage-leaf1.
✓ Config applied on switch switch-gpu-leaf1.
✓ Config applied on switch switch-gpu-spine1.
✓ Config applied on switch switch-gpu-leaf2.
✓ Switch configs applied.
Uploading netplan configs on nodes via bastion...
Copying netplan for node node-gpu-2 (192.168.200.7)...
Copying netplan for node node-gpu-1 (192.168.200.6)...
Copying netplan for node node-gpu-3 (192.168.200.8)...
Copying netplan for node node-storage-1 (192.168.200.10)...
Copying netplan for node node-gpu-4 (192.168.200.9)...
✓ Netplan uploaded and applied on node node-gpu-4.
✓ Netplan uploaded and applied on node node-storage-1.
✓ Netplan uploaded and applied on node node-gpu-1.
✓ Netplan uploaded and applied on node node-gpu-2.
✓ Netplan uploaded and applied on node node-gpu-3.
✓ Netplan configs uploaded and applied on nodes.
✓ Create simulation successfully.

# List simulations
$ nvair get simulation
NAME    STATUS  ID                                    SWITCH  HOST
simple  LOADED  d679effb-0d0a-406b-8ede-a746dc7053ec  6       5

$ nvair get simulations -o json
[
  {
    "id": "d679effb-0d0a-406b-8ede-a746dc7053ec",
    "title": "simple",
    "state": "LOADED",
    "count": {
      "switch": 6,
      "host": 5
    }
  }
]

$ nvair get simulations -o yaml
- id: d679effb-0d0a-406b-8ede-a746dc7053ec
  title: simple
  state: LOADED
  count:
    switch: 6
    host: 5

# List nodes in a simulation
$ nvair get nodes --simulation simple
NAME                  STATUS   MGMT_IP          IMAGE
oob-mgmt-switch       RUNNING  192.168.200.2    oob-mgmt-switch
switch-gpu-spine1     RUNNING  192.168.200.121  cumulus-vx-5.15.0
switch-storage-leaf1  RUNNING  192.168.200.131  cumulus-vx-5.15.0
switch-gpu-leaf4      RUNNING  192.168.200.114  cumulus-vx-5.15.0
switch-gpu-leaf1      RUNNING  192.168.200.111  cumulus-vx-5.15.0
switch-gpu-leaf2      RUNNING  192.168.200.112  cumulus-vx-5.15.0
switch-gpu-leaf3      RUNNING  192.168.200.113  cumulus-vx-5.15.0
node-gpu-3            RUNNING  192.168.200.8    generic/ubuntu2404
node-gpu-2            RUNNING  192.168.200.7    generic/ubuntu2404
node-gpu-1            RUNNING  192.168.200.6    generic/ubuntu2404
node-storage-1        RUNNING  192.168.200.10   generic/ubuntu2404
node-gpu-4            RUNNING  192.168.200.9    generic/ubuntu2404
oob-mgmt-server       RUNNING  192.168.200.1    oob-mgmt-server

# Add a port forwarding rule to a simulation (TODO)
nvair add forward -p 6443
nvair add forward -p 30032

# List port forwarding rule in a simulation  (TODO)
nvair get forward

# Delete a port forwarding rule by service name (TODO)
nvair delete forward -p 6443

# Execute command on a node
nvair exec node-gpu-1 -s my-cluster -- hostname (TODO)
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
