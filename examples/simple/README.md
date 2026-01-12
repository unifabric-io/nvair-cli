# GPU Cluster Topology

## Overview

AI/GPU cluster topology exported from NVIDIA Air, designed for AI/ML training workloads with GPU nodes connected via spine-leaf fabric.

## Architecture

![GPU Cluster Topology](./topology.png)

## Files

- **topology.json**: NVIDIA Air simulation topology (defines VMs and network)
- **node-gpu-1.netplan.yaml** to **node-gpu-4.netplan.yaml**: Netplan configuration for GPU nodes
- **node-storage-1.netplan.yaml**: Netplan configuration for storage node
- **switch-gpu-leaf1.yaml** to **switch-gpu-leaf4.yaml**: GPU leaf switch NVUE configuration exports
- **switch-gpu-spine1.yaml**: GPU spine switch NVUE configuration export
- **switch-storage-leaf1.yaml**: Storage leaf switch NVUE configuration export
- **topology.png**: Network diagram
- **README.md**: This documentation

## Specifications

### GPU Compute Nodes (4 nodes)
- **Names**: node-gpu-1, node-gpu-2, node-gpu-3, node-gpu-4
- **OS**: Ubuntu 24.04
- **NICs**: 9 interfaces per node (eth1-eth9)
  - eth1-eth8: Connected to GPU leaf switches (dual-homed across 2 leaf groups)
  - eth9: Connected to storage network

### GPU Network Fabric

**Spine Layer**:
- 1x switch-gpu-spine1 (Cumulus VX 5.15.0)

**Leaf Layer** (4 leaf switches):
- switch-gpu-leaf1, switch-gpu-leaf2, switch-gpu-leaf3, switch-gpu-leaf4
- OS: Cumulus VX 5.15.0
- Each leaf forms a "Leaf Group" with redundant connections to GPU nodes

**Connection Pattern**:
- Each GPU node connects to 2 leaf groups (8 NICs for GPU fabric)
- Each leaf group connects to 2 GPU nodes
- All leaf switches connect to spine switch (full mesh)

### Storage Network

**Storage Leaf Switch**:
- switch-storage-leaf1 (Cumulus VX 5.15.0)

**Storage Node**:
- node-storage-1 (Ubuntu 24.04)
- 2x storage network connections

### Network Topology Details

| GPU Node | Leaf Group 1 (leaf1) | Leaf Group 2 (leaf2) | Leaf Group 3 (leaf3) | Leaf Group 4 (leaf4) | Storage |
|----------|---------------------|---------------------|---------------------|---------------------|---------|
| node-gpu-1 | eth1-eth4 (4x) | eth5-eth8 (4x) | - | - | eth9 |
| node-gpu-2 | eth1-eth4 (4x) | eth5-eth8 (4x) | - | - | eth9 |
| node-gpu-3 | - | - | eth1-eth4 (4x) | eth5-eth8 (4x) | eth9 |
| node-gpu-4 | - | - | eth1-eth4 (4x) | eth5-eth8 (4x) | eth9 |

**Redundancy**: Each GPU node has 8 high-bandwidth connections for GPU-to-GPU communication across 2 leaf groups.

## Usage

### Create Simulation

```bash
nvcli create -d examples/simple/
```

This will:
1. Read `topology.json` from the directory
2. Submit to NVIDIA Air API
3. Create simulation with GPU cluster topology
4. Automatically detect and apply `*.netplan.yaml` files to corresponding nodes
5. Automatically detect and apply switch `*.yaml` configuration files
6. Set state to `load`
7. Return simulation ID and details

The command automatically applies all node and switch configurations in one step, eliminating the need for separate configuration commands.

### Access GPU Nodes

```bash
# Get simulation details
nvcli get simulation

# Get nodes (shows all GPU nodes, switches, storage)
nvcli get node -s <simulation-name>

# SSH to GPU node
nvcli exec node-gpu-1 -s <simulation-name> -- hostname

# Check GPU node connectivity (netplan configuration applied)
nvcli exec node-gpu-1 -s <simulation-name> -- ip addr show
```

### Configure GPU Fabric

```bash
# Check leaf switch configuration (automatically applied from YAML files)
nvcli exec switch-gpu-leaf1 -s <simulation-name> -- net show interface

# Check storage network
nvcli exec switch-storage-leaf1 -s <simulation-name> -- net show interface
```

## Use Cases

- **AI/ML Training**: Distributed GPU training with high-bandwidth interconnect
- **GPU Networking**: Test RDMA, RoCE, InfiniBand over Ethernet
- **Storage Performance**: Evaluate storage network performance for AI workloads
- **Failover Testing**: Test redundancy with dual-homed GPU nodes
- **Network Optimization**: Tune spine-leaf fabric for GPU communication patterns

## Topology File

The `topology.json` file uses the official NVIDIA Air format. This file:

- **Exported**: From existing GPU cluster simulation on air.nvidia.com
- **Modified**: Can edit node resources, add/remove nodes
- **Reused**: As a template for custom GPU cluster designs

## Advanced Configuration

### Kubernetes Deployment

Deploy Kubernetes across GPU nodes:

```bash
# Install kubeadm on all nodes
nvcli install kubeadm \
  --version 1.28.0 \
  --control-plane-nodes node-gpu-1 \
  --worker-nodes node-gpu-2,node-gpu-3,node-gpu-4 \
  -s <simulation-name>
```

## Cleanup

```bash
# Delete simulation (includes all GPU nodes, switches, storage)
nvcli delete simulation <simulation-name>
```

## Notes

- This topology is designed for AI/ML workloads with GPU-to-GPU communication
- Each GPU node has 8x connections to the GPU fabric for maximum bandwidth
- Storage network is separate to avoid contention
- The spine-leaf design provides non-blocking bandwidth
- Format: NVIDIA Air topology.json (exported from air.nvidia.com)
- Node configurations are in standard Netplan format (`*.netplan.yaml`)
- Switch configurations are automatically applied from `*.yaml` files
- All configurations are applied in one command: `nvcli create -d examples/simple/`

## Configuration Files

### Node Configuration (Netplan)

Each node has a corresponding `nodename.netplan.yaml` file containing standard Netplan configuration:

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
    # ... additional interfaces
```

**Key Features**:
- Standard Netplan YAML format
- Policy-based routing with routing tables
- MTU configuration (4200 bytes for GPU fabric)
- Multiple network interfaces per node
- Automatically applied when running `nvcli create -d <directory>/`

### Switch Configuration

Each switch has a corresponding `switchname.yaml` file containing complete NVUE configuration export:

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
- Can be generated by exporting from existing switches or created manually

### IP Addressing

**GPU Nodes**: 
- Network ranges: `172.17.1.0/24` - `172.17.9.0/24`
- Each node has 9 interfaces (eth1-eth9)
- Node 1: `.11` suffix, Node 2: `.12`, etc.

**Storage Nodes**:
- Network ranges: `172.17.10.0/24` - `172.17.11.0/24`

### Design Philosophy

The new configuration design:

- **Direct files**: Use native configuration formats (Netplan for nodes, NVUE exports for switches)
- **File naming convention**: `nodename.netplan.yaml` and `switchname.yaml`
- **Automatic application**: Configurations applied automatically during `nvcli create`
- **No abstraction**: Direct access to full features of Netplan and NVUE
- **Export-based**: Switch configurations are complete NVUE exports (from `nv config show -o yaml`)
- **Simple workflow**: One command creates and configures everything

### Creating Switch Configuration Files

Switch configuration files can be created in two ways:

1. **Export from existing switches**:
   ```bash
   # SSH to a Cumulus switch and export configuration
   nv config show -o yaml > switch-name.yaml
   ```

2. **Create manually**: Follow the NVUE YAML schema format as shown in the examples
