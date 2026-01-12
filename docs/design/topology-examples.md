# Topology Examples

**Status**: Design Reference  
**Generated**: January 13, 2026

This document describes topology and configuration examples for the nvair CLI.

## Overview

nvair supports the following configuration formats:

1. **Topology Files** (`topology.json`) - NVIDIA Air JSON format exported from air.nvidia.com
2. **Node Configuration Files** (`nodename.netplan.yaml`) - Standard Netplan YAML format for node network configuration
3. **Switch Configuration Files** (`switchname.yaml`) - NVUE configuration export format for switches

## Directory Structure

```
examples/
├── README.md
└── simple/
    ├── topology.json                 # NVIDIA Air topology format
    ├── node-gpu-1.netplan.yaml       # Netplan configuration for node-gpu-1
    ├── node-gpu-2.netplan.yaml       # Netplan configuration for node-gpu-2
    ├── node-gpu-3.netplan.yaml       # Netplan configuration for node-gpu-3
    ├── node-gpu-4.netplan.yaml       # Netplan configuration for node-gpu-4
    ├── node-storage-1.netplan.yaml   # Netplan configuration for storage node
    ├── switch-gpu-leaf1.yaml         # NVUE config export for GPU leaf switch 1
    ├── switch-gpu-leaf2.yaml         # NVUE config export for GPU leaf switch 2
    ├── switch-gpu-leaf3.yaml         # NVUE config export for GPU leaf switch 3
    ├── switch-gpu-leaf4.yaml         # NVUE config export for GPU leaf switch 4
    ├── switch-gpu-spine1.yaml        # NVUE config export for GPU spine switch
    ├── switch-storage-leaf1.yaml     # NVUE config export for storage leaf switch
    └── README.md
```

## Usage

### Create Simulation with All Configurations

```bash
# Create simulation and automatically apply all node and switch configurations
nvcli create -d examples/simple/
```

This command will:
1. Read `topology.json` to create the simulation
2. Automatically detect and apply all `*.netplan.yaml` files to corresponding nodes
3. Automatically detect and apply all `*.yaml` files to corresponding switches
4. Set up the complete cluster environment in one command

## Formats

### topology.json

NVIDIA Air export format (JSON). Can be exported from air.nvidia.com or created manually.

**References**: https://air.nvidia.com/docs/ | https://air.nvidia.com/api/

### Node Configuration (nodename.netplan.yaml)

Standard Netplan YAML format for node network configuration. File naming follows the pattern `nodename.netplan.yaml`.

**Example: node-gpu-1.netplan.yaml**
```yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    eth1:
      addresses:
        - "172.17.1.11/24"
      mtu: 4200
      routes:
        - to: 0.0.0.0/0
          via: 172.17.1.1
          table: 101
      routing-policy:
        - from: 172.17.1.11
          table: 101
          priority: 31761
    eth2:
      addresses:
        - "172.17.2.11/24"
      mtu: 4200
      routes:
        - to: 0.0.0.0/0
          via: 172.17.2.1
          table: 102
      routing-policy:
        - from: 172.17.2.11
          table: 102
          priority: 31762
    # ... additional interfaces
```

**Key Features**:
- Standard Netplan YAML format
- Policy-based routing with routing tables
- MTU configuration per interface
- Multiple network interfaces per node
- Automatically applied when running `nvcli create -d <directory>/`

**Design Philosophy**:
- **Native format**: Use standard Netplan configuration
- **Direct application**: No abstraction layer between configuration and system
- **File naming convention**: `nodename.netplan.yaml` for automatic node matching
- **Full feature access**: All Netplan features available

### Switch Configuration (switchname.yaml)

NVUE configuration export format for switches. File naming follows the pattern `switchname.yaml`.

**Example: switch-gpu-leaf1.yaml**
```yaml
- header:
    model: vx
    nvue-api-version: nvue_v1
    rev-id: 1.0
    version: Cumulus Linux 5.15.0
- set:
    bridge:
      domain:
        br_default:
          untagged: 1
          vlan:
            10,20,30,40: {}
    interface:
      eth0:
        ipv4:
          dhcp-client:
            set-hostname: enabled
            state: enabled
        type: eth
        vrf: mgmt
      swp2,6:
        bridge:
          domain:
            br_default:
              access: 10
      swp3,7:
        bridge:
          domain:
            br_default:
              access: 20
      vlan10:
        ipv4:
          address:
            172.17.1.1/24: {}
        type: svi
        vlan: 10
```

**Key Features**:
- Complete NVUE configuration export format
- Exported from actual switch configurations using `nv config show -o yaml`
- Includes bridge domains, VLANs, interfaces, and routing
- Automatically imported when running `nvcli create -d <directory>/`

**Creating Switch Configuration Files**:

1. **Export from existing switches**:
   ```bash
   # SSH to a Cumulus switch and export configuration
   nv config show -o yaml > switch-name.yaml
   ```

2. **Create manually**: Follow the NVUE YAML schema format as shown in the examples

**Design Philosophy**:
- **Export-based**: Complete configuration exports from real switches
- **No abstraction**: Direct NVUE format without CLI wrapper
- **File naming convention**: `switchname.yaml` for automatic switch matching
- **Idempotent**: Same configuration can be reapplied safely

## Configuration Workflow

### 1. Prepare Topology

Create or export `topology.json` from air.nvidia.com

### 2. Configure Nodes

For each node in the topology, create a corresponding `nodename.netplan.yaml` file:

```bash
# Example for node-gpu-1
vim node-gpu-1.netplan.yaml
```

### 3. Configure Switches

For each switch, either export or create a `switchname.yaml` file:

```bash
# Option 1: Export from existing switch
ssh admin@switch-ip "nv config show -o yaml" > switch-gpu-leaf1.yaml

# Option 2: Create manually
vim switch-gpu-leaf1.yaml
```

### 4. Deploy

Deploy everything with a single command:

```bash
nvcli create -d examples/simple/
```

## Examples

See full examples at:
- [examples/simple/](../../examples/simple/) - GPU cluster with spine-leaf architecture
- [examples/README.md](../../examples/README.md) - Complete examples documentation
