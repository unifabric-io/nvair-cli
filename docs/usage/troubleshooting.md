# Troubleshooting

## Authentication errors
- Ensure your API token is valid and has required scopes. Re-run `nvair login -u <email> -p <api-token>`.
- Use `nvair --verbose login -u <email> -p <api-token> -v` to see detailed API request/response information.

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
