# NVIDIA Air Example Topology Guide

The `examples/` directory provides common NVIDIA Air topology simulations for quick deployment and learning.

## Available Examples

| Example Directory    | Description   | Node Configuration                | Use Case          |
| :------------------- | :------------ | :-------------------------------- | :---------------- |
| [`simple/`](simple/) | GPU Cluster   | 4 GPU, 1 Storage, 2 Leaf, 1 Spine | AI/ML Training    |

> Note: Examples are for learning and quick simulation.

## Quick Simulation Creation

Before running the following command, you must log in to your NVIDIA Air account using
`nvair login -u user@example.com -p <api-token>`. For more details, please refer to the [quickstart](../../docs/quickstart.md).

Use the following command to quickly create a simulation from an example topology:

```bash
nvair create -d examples/simple
```

> Node and switch configuration files will be automatically applied.

To use a different example, simply replace the directory path:

```bash
nvair create -d examples/<case-directory_path>
```

## Custom Topologies

For instructions on creating your own custom topology simulations, please refer to: [Custom Topology Simulation Guide](../docs/usage/custom-topology-simulation.md)
