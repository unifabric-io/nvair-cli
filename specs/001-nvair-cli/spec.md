# nvcli Login Feature

## Overview

Provide a single sign-on flow for the nvcli command-line tool that allows a user to authenticate with their email and API token, obtain a bearer token, and automatically manage local SSH public key generation and upload. This feature is a P1 core capability for the CLI.

## User Scenarios

### Scenario A — First-time login
- User runs `nvcli login -u user@example.com -p <api-token>`.
- The CLI generates an Ed25519 key pair if none exists, exchanges the API token for a bearer token, checks whether the public key is registered in the user's account, uploads the public key if needed, and stores credentials securely in local configuration.

### Scenario B — Subsequent login / token refresh
- The user already has a local key pair and the API token stored. When the bearer token is expired, the CLI automatically refreshes it using the stored API token without requiring interactive re-login.

## Functional Requirements

### 1. Login exchange
- Running `nvcli login -u <email> -p <api-token>` performs a POST to `/v1/auth/login` and either returns a bearer token (HTTP 200) or an appropriate error (e.g., 400/401).
- On success, the CLI saves `bearerToken` and `bearerTokenExpiresAt` in a local config file with file permissions set to 0600.

### 2. SSH key management
- If `$HOME/.ssh/nvair.unifabric.io` does not exist, the CLI generates an Ed25519 key pair and writes the private key with mode 0600 and the public key with mode 0644.
- The CLI computes the public key fingerprint and calls GET `/v1/sshkey` to check for an existing key with the same name and fingerprint.
- If not found, the CLI uploads the public key via POST `/v1/sshkey` and handles `201 Created` and `409 Conflict` responses correctly.

### 3. Errors and fallbacks
- Transient network errors and 5xx responses are retried up to 3 times with exponential backoff; user-facing error messages are returned on permanent failure.
- If public key upload fails but authentication succeeded, the CLI should warn the user but complete the login flow (not block on upload failure).


## Success Criteria

- 95% of first-time logins complete within 5 seconds end-to-end under normal network conditions.
- After login, a user can run `nvcli get simulation` and receive either a list of simulations (when available) or a clear permission/quota error.
- The login flow creates or confirms a local key pair and ensures the private key file permissions are 0600.
- CI must include a closed-loop test: a full login flow (with mock or test API endpoints) is executed automatically, verifying all requirements and error handling, and the test result is visible in the PR or main branch status.

## Assumptions

- Platform endpoints `/v1/auth/login` and `/v1/sshkey` behave according to the documented contracts.
- Users have write permissions to their `$HOME/.ssh` and `$HOME/.config` directories.

