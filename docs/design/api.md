# API Contracts

**Status**: Current  
**Base URL**: `https://api.dsx-air.nvidia.com/api`  
**Authentication**: `Authorization: Bearer <apiToken>`

This document only records the dsx-air API endpoints currently used by the CLI.
Old v1/v2 contracts and no-longer-used endpoints have been removed.

---

## Authentication Model

The CLI uses the user-provided API token directly as a bearer token.
There is no separate login-exchange API in the current implementation.

Request header:

```http
Authorization: Bearer <apiToken>
Content-Type: application/json
```

---

## SSH Key Management

Used by `nvair login`.

### GET `/v3/users/ssh-keys/?limit=`

List the current user's SSH public keys.

CLI use:
- Find an existing key by `name`
- Compare fingerprints before upload

Relevant response fields:

```json
{
  "count": 1,
  "results": [
    {
      "id": "9f40cda4-9562-4937-955c-86e65e917a61",
      "created": "2026-05-09T08:00:00Z",
      "name": "nvair.unifabric.io-cli",
      "fingerprint": "qe2hUthJPcQ2UWhGCi5Sl5NBYX3F2SZbwY5PhKO1Jfc="
    }
  ]
}
```

### POST `/v3/users/ssh-keys/`

Upload a new SSH public key.

Request:

```json
{
  "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...",
  "name": "nvair.unifabric.io-cli"
}
```

### DELETE `/v3/users/ssh-keys/{id}/`

Delete an existing SSH key by ID.

---

## Simulations

Used by `nvair create`, `nvair get simulations`, `nvair delete simulation`, `nvair status`, and simulation resolution logic shared by other commands.

### GET `/v3/simulations`

List simulations visible to the current user.

Relevant response fields:

```json
{
  "count": 1,
  "results": [
    {
      "id": "87eece83-83f0-4ccb-92ab-258fcbab4d3b",
      "name": "demo",
      "state": "ACTIVE",
      "created": "2026-05-09T09:20:02.477033Z"
    }
  ]
}
```

### GET `/v3/simulations/{simulation-id}/`

Fetch a single simulation.

CLI use:
- Poll imported simulations until `INACTIVE`
- Poll started simulations until `ACTIVE`

Relevant response fields:

```json
{
  "id": "a616954f-03fa-4140-bbed-564b30980150",
  "name": "simple",
  "state": "REQUESTING",
  "created": "2026-05-09T09:20:02.477033Z"
}
```

### POST `/v3/simulations/import/`

Create a simulation by uploading the topology payload.

Request body:
- The CLI sends the parsed `topology.json` document directly.

Relevant response fields:

```json
{
  "id": "ff5124ff-96d3-489e-8185-fd3075ca377e",
  "title": "simple"
}
```

### PATCH `/v3/simulations/{simulation-id}/start/`

Start a simulation after it reaches `INACTIVE`.

Request:

```json
{}
```

Relevant response fields:

```json
{
  "id": "a616954f-03fa-4140-bbed-564b30980150",
  "name": "simple",
  "state": "REQUESTING"
}
```

### DELETE `/v3/simulations/{simulation-id}/`

Delete a simulation by ID.

CLI note:
- `nvair delete simulation <name>` first resolves the simulation name with `GET /v3/simulations`, then deletes by ID.

---

## Nodes

Used by `nvair create`, `nvair get nodes`, `nvair get simulations`, `nvair exec`, `nvair cp`, and `nvair add forward`.

### GET `/v3/simulations/nodes/?simulation={simulation-id}&ordering=image&limit={max}`

List nodes for one simulation.

Current CLI query parameters:
- `simulation`: required
- `ordering=image`: preferred server-side grouping by image
- `limit=<max int>`: fetch all rows in one request

Relevant response fields:

```json
{
  "count": 2,
  "results": [
    {
      "id": "node-1",
      "name": "node-gpu-1",
      "state": "ACTIVE",
      "simulation": "sim-1",
      "image": "img-ubuntu",
      "management_ip": "192.168.200.6",
      "metadata": null
    },
    {
      "id": "node-2",
      "name": "switch-gpu-leaf1",
      "state": "ACTIVE",
      "simulation": "sim-1",
      "image": "img-cumulus",
      "management_ip": "192.168.200.111",
      "metadata": {
        "legacy": "value"
      }
    }
  ]
}
```

CLI notes:
- New API uses top-level `image`
- New API uses top-level `management_ip`
- CLI still falls back to legacy `os` and `metadata.mgmt_ip` when needed

### GET `/v3/simulations/nodes/?ordering=image&limit={max}`

List nodes across all simulations.

Used by:
- `nvair get simulations`

---

## Images

Used to resolve node image IDs into human-readable names.

### GET `/v3/images?limit={max}`

Relevant response fields:

```json
{
  "count": 2,
  "results": [
    {
      "id": "img-cumulus",
      "name": "cumulus-vx-5.15.0"
    },
    {
      "id": "img-ubuntu",
      "name": "generic/ubuntu2404"
    }
  ]
}
```

Used by:
- `nvair create`
- `nvair get simulations`
- `nvair get nodes`
- `nvair exec`
- `nvair cp`

---

## Node Interfaces

Used by `nvair create` and `nvair add forward` to find the outbound interface on `oob-mgmt-server`.

### GET `/v3/simulations/nodes/interfaces?simulation={simulation-id}&node={node-id}`

Relevant response fields:

```json
{
  "results": [
    {
      "id": "abd681e9-3e2d-40a9-83b8-97ab33fe0ed6",
      "name": "eth0",
      "interface_type": "MGMT",
      "mac_address": "52:54:00:12:34:56",
      "link_up": true,
      "internal_ipv4": "192.168.200.1",
      "simulation": "sim-1",
      "node": "node-1",
      "outbound": true
    }
  ]
}
```

---

## Interface Services / Forwards

Used by `nvair create`, `nvair add forward`, `nvair get forward`, `nvair delete forward`, and `nvair print-ssh-command`.

### GET `/v3/simulations/nodes/interfaces/services/?simulation={simulation-id}&limit=25`

List services for a simulation.

Relevant response fields:

```json
{
  "count": 2,
  "results": [
    {
      "id": "svc-1",
      "name": "bastion-ssh",
      "interface": "if-out",
      "service_type": "SSH",
      "node_port": 22,
      "worker_port": 16821,
      "worker_fqdn": "worker01.air.nvidia.com",
      "host": "worker01.air.nvidia.com",
      "link": "ssh://ubuntu@worker01.air.nvidia.com:16821"
    }
  ]
}
```

CLI notes:
- The API may return either paginated `{count, results}` or a plain array
- The CLI normalizes `node_port -> dest_port`, `worker_port -> src_port`, and `worker_fqdn -> host`

### POST `/v3/simulations/nodes/interfaces/services/`

Create a new service on a node interface.

Request:

```json
{
  "name": "bastion-ssh",
  "interface": "abd681e9-3e2d-40a9-83b8-97ab33fe0ed6",
  "node_port": 22,
  "service_type": "SSH"
}
```

CLI notes:
- `service_type` is normalized by the CLI to `SSH`, `HTTP`, `HTTPS`, or `OTHER`
- `nvair create` uses this endpoint to create the bastion SSH service
- `nvair add forward` uses the same endpoint for named forwards

### DELETE `/v3/simulations/nodes/interfaces/services/{service-id}/`

Delete a service by ID.

CLI note:
- `nvair delete forward <name>` first resolves the target service via `GET .../services/?simulation=...`, then deletes by ID

---

## Scope

This document is intentionally limited to endpoints that are part of the current CLI flow.
Historical contracts and unused client-side experiments are not recorded here.
