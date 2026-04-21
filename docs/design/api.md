# API Contracts

**Status**: Complete  
**Generated**: January 9, 2026  
**Base URL**: `https://air.nvidia.com/api`  
**Authentication**: Bearer Token (OAuth2)  
**Swagger Documentation**: https://air.nvidia.com/api/

All requests must include: `Authorization: Bearer <token>`

---

## 1. Authentication Endpoint

### POST /v1/auth/login

Exchange username and API token for bearer token.

Request:
```json
{
    "username": "example@example.com",
    "password": "YzVhYWQ0M2UtNjY0ZC00NzgwLWI4YTktNDI5MmZlZGU5MTRh" # api token
}
```

Response (200 OK):
```json
{
    "result": "OK",
    "message": "Successfully logged in.",
    "token": "base64-1.base64-2.base64-3"
}
```

After being base64 decoding, the result is as follows:
```json
{
  "account": "b0fb214a-8b3d-44d9-0000-50a743b37945",
  "realm": "api",
  "exp": 1766735420,
  "admin": false,
  "staff": false,
  "jti": "cf9ba7d382274a3492fc585ed8070000",
  "token_type": "access"
}
```

HTTP Status Codes:
- `200 OK`: Successful authentication
- `400 Bad Request`: Missing required fields
- `401 Unauthorized`: Invalid credentials
- `429 Too Many Requests`: Rate limited (retry after X seconds)

---

## 2. SSH Public Key Management

### GET /v1/sshkey

List all SSH public keys associated with the user's account.

Response (200 OK):
```json
[
    {
        "id": "9f40cda4-9562-4937-955c-86e65e917a61",
        "url": "https://air.nvidia.com/api/v1/sshkey/9f40cda4-9562-4937-955c-86e65e917a61/",
        "account": "https://air.nvidia.com/api/v1/account/b0fb214a-8b3d-44d9-9ce3-50a743b37965/",
        "name": "nvair.unifabric.io-cli",
        "fingerprint": "qe2hUthJPcQ2UWhGCi5Sl5NBYX3F2SZbwY5PhKO1Jfc="
    }
]
```

**Notes**:
- Response is an array of SSH key objects
- `fingerprint` is base64-encoded
- `name` is used to identify the key (e.g., "nvair.unifabric.io-cli")

### POST /v1/sshkey

Upload a new SSH public key to the user's account.

Request:
```json
{
  "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host",
  "name": "nvair.unifabric.io-cli"
}
```

Response (201 Created):
```json
{
    "id": "9f40cda4-9562-4937-955c-86e65e917a41",
    "url": "https://air.nvidia.com/api/v1/sshkey/9f40cda4-9562-4937-955c-86e65e917a41/",
    "account": "https://air.nvidia.com/api/v1/account/b0fb214a-8b3d-44d9-9ce3-50a743b37945/",
    "name": "nvair.unifabric.io-cli",
    "fingerprint": "qe2hUthJPcQ2UWhGCi5Sl5NBYX3F2SZbwY5PhKO1Jfc="
}
```

HTTP Status Codes:
- `201 Created`: Key uploaded successfully
- `400 Bad Request`: Invalid key format or missing required fields
- `409 Conflict`: Key with same name and fingerprint already exists

**Notes**:
- Use `name` field to identify the key source (recommended: "nvair.unifabric.io-cli")
- CLI should check for existing keys with matching `name` and `fingerprint` before uploading

---

## 3. Simulation Endpoints

### POST /v2/simulations

Create a new simulation from topology file.

Request:
```json
{}
```

Response (201 Created):
```json
{
  "id": "63c77456-c2b5-46ed-b610-f6f2bc77665b",
  "title": "demo"
}
```

HTTP Status Codes:
- `201 Created`: Simulation created successfully
- `400 Bad Request`: Invalid topology or parameters
- `403 Forbidden`: User quota exceeded
- `422 Unprocessable Entity`: Topology validation failed


### GET /v2/simulations

List simulations.

Response (200 Created):

```json
{
    "count": 1,
    "next": null,
    "previous": null,
    "results": [
        {
          "cloned": false,
          "created": "2026-01-06T08:06:46.262185Z",
          "documentation": null,
          "expires": true,
          "expires_at": "2026-01-20T08:06:46.261668Z",
          "id": "87eece83-83f0-4ccb-92ab-258fcbab4d3b",
          "metadata": null,
          "modified": "2026-01-09T14:24:30.720428Z",
          "node_count": 11,
          "netq_auto_enabled": false,
          "netq_username": "example+87eece83@example.com",
          "netq_password": "Xl&4pfmD",
          "oob_auto_enabled": true,
          "organization": null,
          "organization_name": null,
          "owner": "example@example.com",
          "sleep": true,
          "sleep_at": "2026-01-09T14:23:18.313695Z",
          "state": "STORED",
          "title": "demo",
          "write_ok": true
        }
    ]
}
```


### POST /api/v2/simulations/{simulation-id}/load

Wake up a simulation

Response (200 No Response)


### DELETE /api/v2/simulations/{simulation-id}

Delete an existing simulation

Response (204 No Response)


### GET /api/v2/simulations/nodes?simulation={simulation-id}&ordering=os

List simulation nodes

```json
{
  "count": 123,
  "next": "http://api.example.org/accounts/?offset=400&limit=100",
  "previous": "http://api.example.org/accounts/?offset=200&limit=100",
  "results": [
    {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "created": "2026-01-12T03:46:19.897Z",
      "modified": "2026-01-12T03:46:19.897Z",
      "simulation": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "worker": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "console_port": 2147483647,
      "serial_port": 2147483647,
      "state": "RUNNING",
      "console_username": "string",
      "console_password": "string",
      "name": "string",
      "os": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "memory": 0,
      "storage": 0,
      "cpu": 0,
      "metadata": "string",
      "version": 0,
      "features": "string",
      "pos_x": 0,
      "pos_y": 0,
      "system": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "boot_group": 0,
      "console_url": "string"
    }
  ]
}
```

### /v2/simulations/nodes/interfaces?simulation={simulation-id}&node={node-id}

List node interfaces.

```json
{
  "count": 123,
  "next": "http://api.example.org/accounts/?offset=400&limit=100",
  "previous": "http://api.example.org/accounts/?offset=200&limit=100",
  "results": [
    {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "link_up": true,
      "internal_ipv4": "198.51.100.42",
      "full_ipv6": "2001:0db8:5b96:0000:0000:426f:8e17:642a",
      "prefix_ipv6": "2001:0db8:5b96:0000:0000:426f:8e17:642a",
      "port_number": 2147483647,
      "node": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "simulation": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "breakout": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "name": "string",
      "interface_type": "DATA_PLANE_INTF",
      "mac_address": "string",
      "preserve_mac": true,
      "outbound": true,
      "link": "3fa85f64-5717-4562-b3fc-2c963f66afa6"
    }
  ]
}
```

### POST /v1/service

Enable ssh forward

Request:
```json
{
    "name": "bastion-ssh",
    "simulation": "87eece83-83f0-4ccb-92ab-258fcbab4d3b",
    "interface": "ac6975d0-b988-40ec-90e2-b59f0d006064",
    "dest_port": 22,
    "service_type": "ssh"
}
```

Response (201 Created):
```json
{
    "url": "https://air.nvidia.com/api/v1/service/aeeca4f4-c22c-4bf3-84df-ff80ccf4f9ee/",
    "id": "aeeca4f4-c22c-4bf3-84df-ff80ccf4f9ee",
    "name": "bastion-ssh",
    "simulation": "https://air.nvidia.com/api/v1/simulation/87eece83-83f0-4ccb-92ab-258fcbab4d3b/",
    "interface": "https://air.nvidia.com/api/v1/simulation-interface/ac6975d0-b988-40ec-90e2-b59f0d006064/",
    "dest_port": 22,
    "src_port": 16821,
    "link": "ssh://ubuntu@worker01.air.nvidia.com:16821",
    "service_type": "ssh",
    "node_name": "oob-mgmt-server",
    "interface_name": "eth0",
    "host": "worker01.air.nvidia.com",
    "os_default_username": "ubuntu"
}
```

### GET /v1/service

List all services for a simulation.

Query Parameters:
- `simulation`: Simulation ID (required)

Response (200 OK):
```json
[
  {
    "id": "aeeca4f4-c22c-4bf3-84df-ff80ccf4f9ee",
    "name": "bastion-ssh",
    "simulation": "87eece83-83f0-4ccb-92ab-258fcbab4d3b",
    "interface": "ac6975d0-b988-40ec-90e2-b59f0d006064",
    "dest_port": 22,
    "src_port": 16821,
    "link": "ssh://ubuntu@worker01.air.nvidia.com:16821",
    "service_type": "ssh",
    "node_name": "oob-mgmt-server"
  },
  {
    "id": "bbf3c5e5-d33d-5ecf-a3bc-539g1fef92ff",
    "name": "k8s-default-my-web-app-30080",
    "simulation": "87eece83-83f0-4ccb-92ab-258fcbab4d3b",
    "interface": "bd7086e1-c099-51fd-b1cd-64a18f4d93fe",
    "dest_port": 30080,
    "src_port": 17922,
    "link": "http://worker01.air.nvidia.com:17922",
    "service_type": "tcp",
    "node_name": "bastion-host"
  }
]
```

**Note**: Services synced from Kubernetes have `k8s-` prefix (e.g., `k8s-default-my-web-app-30080`).

### DELETE /v1/service/{service-id}

Delete a service forwarding rule.

Response (204 No Content)

---

## 9. Get SSH Private Key

### GET /v2/simulations/{simulation-id}/ssh-key

Retrieve SSH private key for accessing simulation nodes.

Response (200 OK):
```json
{
  "private_key": "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA...\n-----END OPENSSH PRIVATE KEY-----\n",
  "key_type": "ed25519",
  "default_password": "initial-password-123"
}
```

**Notes**:
- Private key is used to access the bastion host (oob-mgmt-server)
- `default_password` is the initial bastion password that should be changed on first use
- Keys are simulation-specific
