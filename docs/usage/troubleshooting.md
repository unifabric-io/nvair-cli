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

## Switch unreachable / `context deadline exceeded` during `nvair create`
- Symptom: `switch <name> unreachable: context deadline exceeded` while resetting switch passwords.
- Cause: a Cumulus switch can miss its first DHCP round on the management network (the OOB switch is not forwarding yet) and then waits minutes before its DHCP client retries, so its management IP does not answer even though it has booted.
- `nvair create` reaches the switch over its IPv6 link-local address (derived from the management MAC) through the bastion, resets the password, and restarts DHCP so the management IP comes up immediately. It falls back to pinging the management IP for up to 20 minutes when link-local is unavailable.
- To investigate manually, SSH to the bastion and run `ping <switch-mgmt-ip>` and `ip neigh`. An `INCOMPLETE` neighbor entry only means the bastion got no answer to neighbor discovery. It can mean the switch has not obtained its lease yet, but also that the switch is down or there is an L2 connectivity problem, so check the switch state too.
- Re-run with `--verbose` to see the last probe error and which path (link-local or management IP) was used.

## Command timeout or unexpected errors
- Re-run with `--verbose` to get detailed logs including:
  - API endpoint calls and response codes
  - SSH key fingerprints and registration status
  - Network retry attempts and backoff timing
  - Configuration file operations
