# Feature Specification: Exec Command

**Feature Branch**: `004-exec-command`  
**Created**: 2026-03-16  
**Status**: Draft  
**Input**: User description: "Implement nvair exec command for executing commands on simulation nodes via SSH, supporting both non-interactive and interactive modes"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Execute Commands on Simulation Nodes (Priority: P1)

As a user managing a simulation, I want to execute commands on any node in my simulation directly from the CLI so that I can inspect, configure, and troubleshoot nodes without manually setting up SSH connections through the bastion host.

The exec command supports two modes:

- **Non-interactive mode**: Run a single command on a node and get the output back (e.g., `nvair exec node-gpu-1 -- hostname`). The command runs, prints stdout/stderr, and exits with the remote command's exit code.
- **Interactive mode**: When no command is provided after `--`, open an interactive SSH shell session on the node (e.g., `nvair exec node-gpu-1`). The user's terminal is attached to the remote shell, supporting full TTY interaction (tab completion, signal handling, window resizing). The session ends when the user exits the shell.

The exec command resolves the target node by name within the specified simulation, locates the bastion host (oob-mgmt-server), and establishes an SSH tunnel through the bastion to reach the target node. The simulation name is required via the `-s` flag.

**Why this priority**: This is the core and only user story. Remote command execution is the fundamental value of the exec feature — without it, users must manually discover bastion credentials, SSH keys, and node IPs to reach simulation nodes.

**Independent Test**: Can be fully tested by running `nvair exec <node-name> -- <command>` against a running simulation and verifying the correct output is returned; and by running `nvair exec <node-name>` and verifying an interactive shell session opens.

**Acceptance Scenarios**:

1. **Given** a logged-in user with a running simulation, **When** the user runs `nvair exec node-gpu-1 -- hostname`, **Then** the CLI prints the hostname of node-gpu-1 and exits with exit code 0.
2. **Given** a logged-in user with a running simulation, **When** the user runs `nvair exec node-gpu-1 -s my-sim -- cat /etc/os-release`, **Then** the CLI prints the OS release info of node-gpu-1 in simulation "my-sim".
3. **Given** a logged-in user with a running simulation, **When** the user runs `nvair exec node-gpu-1`, **Then** the CLI opens an interactive SSH shell session on node-gpu-1 with full TTY support.
4. **Given** a logged-in user with a running simulation, **When** the user runs `nvair exec node-gpu-1` without `-s` flag, **Then** the CLI prints an error message indicating the simulation flag is required.
5. **Given** a logged-in user, **When** the user runs `nvair exec nonexistent-node -- hostname`, **Then** the CLI prints an error message indicating the node was not found and exits with a non-zero exit code.
6. **Given** a non-interactive exec session, **When** the remote command fails (non-zero exit code), **Then** the CLI exits with the same exit code as the remote command.
7. **Given** an interactive exec session, **When** the user resizes their terminal window, **Then** the remote TTY adjusts to match the new dimensions.
8. **Given** an interactive exec session, **When** the user presses Ctrl+C, **Then** the signal is forwarded to the remote process rather than terminating the local CLI.

---

### Edge Cases

- What happens when the user is not logged in? → CLI prints an authentication error and directs the user to run `nvair login`.
- What happens when the specified simulation does not exist? → CLI prints a "simulation not found" error with the simulation name.
- What happens when the specified node does not exist in the simulation? → CLI prints a "node not found" error with the node name and lists available nodes.
- What happens when the bastion host (oob-mgmt-server) is unreachable? → CLI prints a connection error indicating the bastion is not reachable.
- What happens when the target node is unreachable from the bastion? → CLI prints a connection error indicating the target node is not reachable.
- What happens when the SSH service is not yet created for the simulation? → CLI lists existing services, fails to find the SSH forwarding service, and prints an error directing the user to create the simulation first via `nvair create`.
- What happens when the user specifies `oob-mgmt-server` as the target node? → CLI connects directly to the bastion host without tunneling through itself.
- What happens when verbose mode is enabled? → CLI logs each step: simulation resolution, node lookup, bastion connection, SSH tunnel establishment, and command execution.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `exec` command MUST accept a node name as a positional argument to identify the target node.
- **FR-002**: The `exec` command MUST require a `-s, --simulation` flag to specify the simulation by name.
- **FR-003**: When a command is provided after `--`, the CLI MUST execute it on the target node in non-interactive mode, printing stdout and stderr to the local terminal, and exiting with the remote command's exit code.
- **FR-004**: When no command is provided after `--` (or `--` is omitted), the CLI MUST open an interactive SSH shell session on the target node with full TTY support (input echoing, signal forwarding, terminal resize propagation).
- **FR-005**: The CLI MUST resolve the target node by name within the specified simulation to obtain its management IP address.
- **FR-006**: The CLI MUST connect to the target node through the bastion host (oob-mgmt-server) using the existing SSH key infrastructure — public key authentication to the bastion. For host nodes (e.g., node-gpu-1), the CLI MUST use password authentication (ubuntu/nvidia) from the bastion to the target. For switch nodes (e.g., switch-gpu-leaf1), the CLI MUST use password authentication (cumulus/Dangerous1#) from the bastion to the target. When the target is the bastion itself (oob-mgmt-server), the CLI MUST connect directly without tunneling.
- **FR-007**: The CLI MUST look up the existing SSH service on the bastion host's outbound interface by listing services for the simulation. If no SSH service is found, the CLI MUST report an error directing the user to create the simulation first.
- **FR-008**: The CLI MUST display a clear error message when the specified node is not found in the simulation.
- **FR-009**: The CLI MUST display a clear error message when the specified simulation is not found.
- **FR-010**: The CLI MUST support verbose logging (`-v, --verbose`) to show detailed connection and execution steps.
- **FR-011**: The CLI MUST forward the remote command's exit code as its own exit code in non-interactive mode.

### Key Entities

- **Simulation**: A simulated network environment identified by name; contains multiple nodes. Resolved via the NVIDIA Air API.
- **Node**: A machine in the simulation (host or switch) identified by name; has a management IP used for SSH access.
- **Bastion Host (oob-mgmt-server)**: The jump host automatically provisioned by NVIDIA Air; serves as the SSH gateway to all other nodes in the simulation.
- **SSH Service**: A port forwarding rule on the bastion host's outbound interface that exposes SSH access from the external network.

## Assumptions

- The bastion host (oob-mgmt-server) is always present in a simulation and is the standard jump host for reaching other nodes.
- Target nodes are reachable from the bastion via their management IP on port 22.
- The existing SSH key (stored at `~/.ssh/nvair.unifabric.io`) is used for bastion authentication. The CLI already manages key generation and registration during `nvair create`.
- Target node authentication depends on node type: host nodes use ubuntu/nvidia credentials, switch nodes use cumulus/Dangerous1# credentials (consistent with the existing create workflow).
- The user's terminal supports standard TTY operations for interactive mode.
- The SSH service on the bastion already exists from a previous `nvair create`. The exec command does not create SSH services — it only looks up and uses existing ones.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can execute a command on any node in a running simulation with a single CLI invocation (e.g., `nvair exec node-gpu-1 -- hostname`).
- **SC-002**: Users can open an interactive shell on any node with a single CLI invocation (e.g., `nvair exec node-gpu-1`) and interact with it as if directly SSH'd in.
- **SC-003**: 100% of supported error scenarios (node not found, simulation not found, bastion unreachable, not logged in) produce a clear, actionable error message.
- **SC-004**: The remote command's exit code is faithfully propagated to the CLI's exit code in non-interactive mode.
