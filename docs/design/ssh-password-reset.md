# SSH Password Reset Implementation Reference

**Status**: Technical Reference  
**Generated**: January 12, 2026

This document provides implementation guidance for the bastion host password reset functionality.

## Overview

Before accessing simulation nodes via the bastion host, the CLI must reset the **bastion host (oob-mgmt-server) password only**. This ensures secure access to the bastion. The bastion then uses SSH keys to access target nodes behind it - those nodes do not require password reset.

This is done using SSH with public key authentication to establish a PTY session and execute the `passwd` command interactively on the bastion.

## Architecture

```
┌─────────┐     SSH + Private Key        ┌──────────────┐
│   CLI   │ ───────────────────────────> │   Bastion    │
└─────────┘   (reset bastion password)   │ oob-mgmt-svr │
                                         └──────────────┘
```

**Scope**: This document only covers bastion host password reset. Access to target nodes behind the bastion is handled separately using SSH key forwarding.

## Implementation Approach

### Step 1: Establish SSH Connection

```go
// Load private key from platform API or local cache
signer, err := ssh.ParsePrivateKey(privateKeyBytes)

// Configure SSH client
clientCfg := &ssh.ClientConfig{
    User: "ubuntu",  // or platform-specified username
    Auth: []ssh.AuthMethod{
        ssh.PublicKeys(signer),
    },
    HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    Timeout:         30 * time.Second,
}

// Connect to bastion
client, err := ssh.Dial("tcp", "bastion-host:22", clientCfg)
```

### Step 2: Request PTY Session

A pseudo-terminal (PTY) is required for the `passwd` command to work interactively.

```go
session, err := client.NewSession()

modes := ssh.TerminalModes{
    ssh.ECHO:          0,     // Disable echo (passwords won't show)
    ssh.TTY_OP_ISPEED: 14400,
    ssh.TTY_OP_OSPEED: 14400,
}

err = session.RequestPty("xterm", 80, 40, modes)
```

### Step 3: Handle Interactive Prompts

The `passwd` command will prompt for:
1. Current password
2. New password
3. Retype new password

**Key Challenge**: These prompts do **not** end with newlines, so standard line-based reading won't work.

```go
// Read output byte-by-byte and detect special prompts
specialPrompts := []string{
    "Current password:",
    "New password:",
    "Retype new password:",
}

// When prompt detected, send appropriate response
if strings.HasSuffix(output, "Current password:") {
    stdin.Write([]byte(oldPassword + "\n"))
} else if strings.HasSuffix(output, "New password:") {
    stdin.Write([]byte(newPassword + "\n"))
} else if strings.HasSuffix(output, "Retype new password:") {
    stdin.Write([]byte(newPassword + "\n"))
    // Password change complete
}
```

### Step 4: Execute Shell and passwd Command

```go
err = session.Shell()

// Send passwd command
stdin.Write([]byte("passwd\n"))

// Handle prompts (see Step 3)

// Wait for session to complete
session.Wait()
```

## State Machine

```
┌─────────┐
│  init   │
└────┬────┘
     │ detect "Changing password for" or "Current password:"
     v
┌─────────┐
│   old   │ ──> send old password
└────┬────┘
     │ detect "New password:"
     v
┌─────────┐
│   new   │ ──> send new password
└────┬────┘
     │ detect "Retype new password:"
     v
┌─────────┐
│  done   │ ──> send new password again
└─────────┘
```

## Complete Reference Implementation

See the provided Go code in the user request for a complete, tested implementation.

### Key Components

1. **ChangePasswordConfig** - Configuration struct with connection details and passwords
2. **loadPrivateKeySigner** - Parse SSH private key
3. **requestPTY** - Request pseudo-terminal with proper modes
4. **preparePipes** - Set up stdin/stdout/stderr pipes
5. **readPipe** - Byte-by-byte reader that detects prompts without newlines
6. **handlePasswordChange** - State machine for password change flow
7. **writeLine** - Helper to send password + newline

## Integration with CLI

### Workflow

1. User runs `nvair create -d <topology-dir>` or first command requiring bastion access
2. CLI checks if bastion password has been reset for this simulation (check local cache/config)
3. If not reset:
   - Fetch SSH private key and default bastion password from API
   - Execute password reset flow **on bastion host only**
   - Store new bastion password in config (`$HOME/.config/nvair.unifabric.io/bastion-passwords.json`)
4. Use new password for bastion SSH connections
5. **Target nodes behind bastion**: Accessed via SSH keys (no password reset needed)

### Storage Format

```json
{
  "simulations": {
    "sim-abc-123": {
      "bastionHost": "worker01.air.nvidia.com:16821",
      "username": "ubuntu",
      "password": "user-set-password",
      "lastReset": "2026-01-12T06:00:00Z"
    }
  }
}
```

**Security**: File must have 0600 permissions (user read-write only).

### Important Notes

1. **Only bastion password is reset**: Target nodes (gpu-node, storage-node, etc.) are accessed via SSH keys through the bastion
2. **Per-simulation password**: Each simulation has its own bastion host with unique password
3. **Password reuse**: Once reset, the password is stored and reused for that simulation
4. **No password for target nodes**: CLI uses SSH key forwarding to access nodes behind the bastion

## Error Handling

### Common Errors

1. **Timeout waiting for prompt**
   - Cause: Network issue or unexpected output from passwd
   - Solution: Increase timeout, log all output for debugging

2. **Password complexity requirements not met**
   - Cause: Platform enforces password policy (length, complexity)
   - Solution: Validate password before attempting reset, show requirements to user

3. **SSH connection failure**
   - Cause: Private key invalid or bastion host unreachable
   - Solution: Verify key format, check network connectivity

4. **PTY request denied**
   - Cause: SSH server doesn't support PTY
   - Solution: This should not happen with standard OpenSSH; log for investigation

## Testing

### Unit Tests

Mock SSH session and test state machine:
- Verify correct password sent for each prompt
- Test timeout handling
- Test unexpected output handling

### Integration Tests

Requires actual SSH server:
- Test against real bastion host (in test environment)
- Verify password actually changes
- Test subsequent SSH login with new password

## Security Considerations

1. **Private Key Storage**: Store in `$HOME/.ssh/nvair/` with 0600 permissions
2. **Password Storage**: Store in config with 0600 permissions (plaintext locally)
3. **Network Security**: All SSH connections use encrypted transport
4. **Password Complexity**: Encourage strong passwords (12+ chars, mixed case, numbers)
5. **Key Rotation**: Platform may rotate SSH keys periodically

## Future Enhancements

1. **Password Strength Validation**: Check password meets platform requirements before attempting reset
2. **Password Manager Integration**: Support fetching passwords from system keychain
3. **Multi-Factor Authentication**: Support if platform adds MFA requirements
4. **Session Recording**: Optional logging of SSH sessions for audit trails

---

**References**:
- golang.org/x/crypto/ssh documentation
- OpenSSH PTY behavior
- Platform API documentation for SSH key endpoints
