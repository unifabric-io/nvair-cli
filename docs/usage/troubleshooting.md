# Troubleshooting

## Authentication errors
- Ensure your API token is valid and has required scopes. Re-run `nvair login -u <email> -p <api-token>`.
- Use `nvair --verbose login -u <email> -p <api-token> -v` to see detailed API request/response information.

## `nvair status` shows `User          : Not logged in`
- Re-run `nvair login -u <email> -p <api-token>` to create a fresh local session.
- If you recently changed or revoked your API token, log in again so the CLI can store a usable bearer token.
- If the local config was partially written or is missing required fields, `nvair status` will intentionally treat that state as logged out.

## `nvair status` shows `Access        : No`
- Re-run `nvair --verbose status` to distinguish a remote connectivity failure from a valid local session.
- Confirm the API endpoint is reachable from your network and that any VPN or proxy requirements are satisfied.
- If your account access changed, re-run `nvair login` to refresh credentials and validate authorization again.

## SSH connection failures
- Verify the node's management IP is reachable from your network.
- If firewall or network blocks exist, use a reachable bastion host or check VPN settings.
- Use `nvair --verbose` to check SSH key generation and registration details.

## Command timeout or unexpected errors
- Re-run with `--verbose` to get detailed logs including:
  - API endpoint calls and response codes
  - SSH key fingerprints and registration status
  - Network retry attempts and backoff timing
  - Configuration file operations
