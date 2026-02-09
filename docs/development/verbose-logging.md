# Verbose Logging Guide

## Overview

The `--verbose` (or `-v`) global flag enables detailed logging for all nvair CLI commands. This is useful for debugging authentication issues, troubleshooting network problems, and understanding the internal operations of the CLI.

## Usage

Place the `--verbose` flag before the subcommand:

```bash
nvcli --verbose <command> [options]
nvcli -v <command> [options]
```

## Examples

### Login with verbose output

```bash
nvcli --verbose login -u user@example.com -p <api-token>
```

### Logout with verbose output

```bash
nvcli --verbose logout
```

## Log Format

Verbose logs are printed to stderr with the following format:

```
[DEBUG] [YYYY-MM-DD HH:MM:SS] <message>
```

Example:
```
[DEBUG] [2026-02-10 10:23:45] Login command started with username: user@example.com
[DEBUG] [2026-02-10 10:23:45] AuthLogin: Starting authentication for user: user@example.com
[DEBUG] [2026-02-10 10:23:45] doRequest: [Attempt 1/3] POST https://air.nvidia.com/api/v1/login/
```

## What Gets Logged

### Configuration Management
- Config file paths
- Loading/saving operations
- Directory creation
- File permission operations

### SSH Key Management
- SSH key path determination
- Key pair generation progress
- File write operations with permission details
- Key loading and fingerprint computation

### API Client
- Request method, URL, and attempt number
- Request body (with truncated secrets)
- Response status codes and timing
- Retry attempts with backoff timing
- Bearer token injection (with truncated token for security)

### Commands
- Command initialization and flag validation
- Step-by-step progress through multi-step operations
- User input and decision points
- Success/failure messages with details

## Debugging Common Issues

### Authentication Failures

Run with verbose logging to see:
- Exact API endpoint being called
- Request and response bodies
- HTTP status codes
- Network error details

```bash
nvcli --verbose login -u user@example.com -p <api-token>
```

### SSH Key Issues

Verbose output shows:
- SSH key path
- Whether keys already exist or need generation
- Key file permissions (0600 for private, 0644 for public)
- SSH key fingerprint (SHA256 base64-encoded)
- Key registration status

### Network/Retry Issues

Verbose output shows:
- Each retry attempt with timing
- HTTP status codes that trigger retries (5xx errors)
- Backoff wait times
- Final result after all retries

## Security Considerations

Verbose logs may contain:
- Usernames and email addresses
- Truncated API tokens (displayed as `my-token-...`)
- Truncated bearer tokens (displayed as `my-bearer-...`)
- SSH key fingerprints

**Note**: Full tokens are never logged. For security, avoid sharing verbose logs that contain sensitive information.

## Implementation Details

Verbose logging is controlled by the `pkg/logging` package:

- `SetVerbose(io.Writer)` - Enable verbose logging to a writer
- `Verbose(format, args...)` - Log a verbose message
- `Info(format, args...)` - Log an info message (always shown)
- `Warn(format, args...)` - Log a warning message (always shown)
- `Error(format, args...)` - Log an error message (always shown)

The `--verbose` flag is parsed at the root command level and passed to subcommands via the command struct.
