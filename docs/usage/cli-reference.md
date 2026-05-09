# CLI Command Reference

```bash
nvair <command> [options]

Available Commands:

  add              Add resources
  completion       Generate the autocompletion script for the specified shell
  cp               Copy files between local machine and simulation nodes
  create           Create a simulation from topology
  delete           Delete resources
  exec             Execute commands on simulation nodes via SSH
  get              Get simulations, nodes, and forwards
  help             Help about any command
  login            Authenticate with NVIDIA Air
  logout           Logout from NVIDIA Air
  print-ssh-command Print SSH command for bastion host
  status           Show current login and connectivity status

Subcommands:

  add forward
      <forward-name>               Forward service name (required)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      --target-node <node>         Target node name (required)
      --target-port <port>         Target port on target node (required)

  create
      -d, --directory <path>       Directory containing topology.json and config files
      --dry-run                    Validate configuration files without creating simulation
      --delete-if-exists           Delete an existing simulation with the same name before creating

  cp
      <src> <dest>                 Copy between local and remote paths
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      --timeout <duration>         Copy timeout (default 2m)

  delete simulation
      <name>                       Simulation name (required)

  delete forward
      <forward-name>               Forward service name (required)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)

  exec
      <node-name>                  Node name (required)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      -i, --stdin                  Keep stdin open (must be used with -t for interactive shell)
      -t, --tty                    Allocate TTY (must be used with -i for interactive shell)
      --                           Separator before command
      <command>                    Command to execute (required)

  get simulation
      -o, --output <json|yaml>     Structured output format (optional)

  get nodes
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      -o, --output <json|yaml>     Structured output format (optional)

  get forward
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      -o, --output <json|yaml>     Structured output format (optional)

  print-ssh-command
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)

Global Options:
  -v, --verbose                    Enable verbose logging for debugging
  -h, --help                       Show help for command
  --version                        Show version information
```

> `-s, --simulation` can be omitted only when exactly one simulation exists in your account.  
> If there are multiple simulations, you must specify `-s <name>`.

> The create command performs the following main steps:
> 1. Load and validate topology configuration from the specified directory
> 2. Create the simulation on NVIDIA Air platform
> 3. Wait for the imported simulation to become `INACTIVE`
> 4. Start the simulation and wait for it to become `ACTIVE`
> 5. Configure SSH access through the bastion host
> 6. Reset passwords and apply configurations to switches
>    - This resets the bastion password to `dangerous` and the switch passwords to a known value. It does not affect normal use: password login is not used in practice, and the reset only skips the forced password-change prompt.
> 7. Upload and apply Netplan configurations to Linux nodes
>
> Note: NVIDIA Air automatically provides a bastion (jump) machine in your topology as an additional built-in node for secure access to other nodes.
