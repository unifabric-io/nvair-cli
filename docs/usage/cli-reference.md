# CLI Command Reference

```bash
nvair <command> [options]

Commands:

  login                            Authenticate with NVIDIA Air platform
      -u, --username <string>      Username (email) for authentication
      -p, --password <string>      API token for authentication (get from https://air.nvidia.com/settings)
      
  create                           Create a simulation from topology directory
      -d, -
      -directory <path>       Directory containing topology.json and config files
      --dry-run                    Validate configuration files without creating simulation
      
  get                              Get resources (simulations, nodes, forwards)
    simulation                     List all simulations (alias: simulations)
      -o, --output <json|yaml>     Structured output format (optional)
    node                           List nodes in a simulation (alias: nodes)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      -o, --output <json|yaml>     Structured output format (optional)
    forward                        List port forwards in a simulation (alias: forwards)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      -o, --output <json|yaml>     Structured output format (optional)

  print-ssh-command                Print bastion SSH command for a simulation
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
       
  delete                           Delete resources
    simulation                     Delete a simulation
      <name>                       Simulation name (required)
    forward                        Delete a port forwarding rule
      <forward-name>               Forward service name (required)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
       
  exec                             Execute commands on nodes via SSH
      <node-name>                  Node name (required)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
      -i, --stdin                  Keep stdin open (must be used with -t for interactive shell)
      -t, --tty                    Allocate TTY (must be used with -i for interactive shell)
      --                           Separator before command
      <command>                    Command to execute (required)
       
  add                              Add resources
    forward                        Add a port forwarding rule
      <forward-name>               Forward service name (required)
      -s, --simulation <name>      Simulation name (optional when only one simulation exists)
          --target-node <node>     Target node name (required)
          --target-port <port>     Target port on target node (required)

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
> 3. Set simulation state to 'load' and wait for initialization jobs
> 4. Configure SSH access through the bastion host
> 5. Reset passwords and apply configurations to switches
>    - This resets the bastion password to `dangerous` and the switch passwords to a known value. It does not affect normal use: password login is not used in practice, and the reset only skips the forced password-change prompt.
> 6. Upload and apply Netplan configurations to Linux nodes
>
> Note: NVIDIA Air automatically provides a bastion (jump) machine in your topology as an additional built-in node for secure access to other nodes.
