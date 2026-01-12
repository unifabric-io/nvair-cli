# Examples

This directory contains sample configurations for NVIDIA Air simulations and cluster deployments.

## Quick Start

```bash
# Create a simulation from topology file and apply node configurations
nvcli create -d examples/simple/
```

## Available Examples

| Example | Description | Nodes | Use Case |
|---------|-------------|-------|----------|
| [simple/](simple/) | GPU Cluster Topology | 4 GPU + 1 Storage | AI/ML Training Cluster |

## Configuration Formats

### 1. Topology Files (topology.json)

NVIDIA Air simulation topology format for creating virtual network environments:

- **Format**: JSON (NVIDIA Air native format)
- **Purpose**: Define virtual machines, switches, and network connections
- **Usage**: `nvcli create -d <directory>/`
- **Source**: Exported from air.nvidia.com or created manually

### 2. Node Configuration Files (*.netplan.yaml)

Netplan YAML format for node network configuration:

- **Format**: YAML (Netplan standard format)
- **Purpose**: Define network interfaces, IP addresses, routes, and policies for each node
- **Naming**: `nodename.netplan.yaml` (e.g., `node-gpu-1.netplan.yaml`)
- **Usage**: Automatically applied when running `nvcli create -d <directory>/`
- **Design**: Direct netplan files that get applied to corresponding nodes

### 3. Switch Configuration Files (switchname.yaml)

NVUE configuration export format for switches:

- **Format**: YAML (NVUE configuration export)
- **Purpose**: Complete switch configuration including interfaces, VLANs, routing, and bridge domains
- **Naming**: `switchname.yaml` (e.g., `switch-gpu-leaf1.yaml`)
- **Source**: Exported from switches using `nv config show -o yaml` or created manually
- **Usage**: Automatically imported when running `nvcli create -d <directory>/`
- **Design**: Complete configuration exports that get applied to corresponding switches

## Directory Structure

Each example directory contains:

- `topology.json` - NVIDIA Air topology (VM and network definition)
- `nodename.netplan.yaml` - Node network configuration files (one per node)
- `switchname.yaml` - Switch NVUE configuration exports (one per switch)
- `README.md` - Example-specific documentation

The `topology.json` file uses the standard NVIDIA Air simulation format that can be:
1. Exported from existing simulations on air.nvidia.com
2. Created manually following the Air API schema
3. Modified from examples provided here

Node configuration files follow the pattern `nodename.netplan.yaml` and contain standard Netplan YAML configuration.

Switch configuration files follow the pattern `switchname.yaml` and contain complete NVUE configuration exports (from `nv config show -o yaml`).

When running `nvcli create -d <directory>/`, these files are automatically applied to their corresponding nodes and switches.

## Creating Simulations

```bash
# From a local topology file (automatically applies node and switch configurations)
nvcli create -d examples/simple/

# From a custom topology directory
nvcli create -d /path/to/my-topology/
```

The `create` command will:
1. Read `topology.json` to create the simulation
2. Automatically detect and apply `*.netplan.yaml` files to corresponding nodes
3. Automatically detect and apply switch `*.yaml` configuration files
4. Set up the complete cluster environment in one command

## Exporting Topologies from NVIDIA Air

You can export existing simulations from air.nvidia.com:

1. Navigate to your simulation on air.nvidia.com
2. Click "Export Topology" or use the API
3. Save as `topology.json`
4. Use with `nvair create -d <directory>/`

## Creating Custom Topologies

1. Copy an example as a template:
   ```bash
   cp -r examples/simple/ my-topology/
   ```

2. Edit `topology.json` with your node and link definitions

3. Create or modify node netplan files:
   ```bash
   # Edit node-gpu-1.netplan.yaml, node-gpu-2.netplan.yaml, etc.
   ```

4. Create or modify switch configuration files:
   ```bash
   # Option 1: Export from existing switch
   ssh admin@switch-ip "nv config show -o yaml" > switch-gpu-leaf1.yaml
   
   # Option 2: Edit manually following NVUE YAML format
   vim switch-gpu-leaf1.yaml
   ```

5. Create simulation with all configurations applied:
   ```bash
   nvcli create -d my-topology/
   ```

## Tips

- Use descriptive node names (e.g., `gpu-node1`, `storage-node1`)
- Name netplan files to match node names: `nodename.netplan.yaml`
- Name switch config files to match switch names: `switchname.yaml`
- Tag nodes with roles for easier management
- The `topology.json` format is the official NVIDIA Air format
- All configurations are applied automatically with `nvcli create -d <directory>/`

## Getting Help

```bash
nvair create --help
```

## Documentation

For detailed information:
- [topology-examples.md](../specs/001-nvair-cli/topology-examples.md) - Complete guide
- [NVIDIA Air Docs](https://air.nvidia.com/docs/) - Official platform documentation
- [Air API](https://air.nvidia.com/api/) - API reference