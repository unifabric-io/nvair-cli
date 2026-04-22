# SSH Key Generation and Upload

**Status**: Technical Reference  
**Generated**: January 12, 2026

This document describes the SSH key generation and upload process that occurs during `nvair login`.

## Overview

When a user logs in for the first time, the CLI automatically generates an SSH key pair and uploads the public key to the user's nvidia account. This enables SSH access to bastion hosts and simulation nodes.

## Workflow

```
nvair login -u user@example.com -p api-token
    │
    ├─> 1. Check if ~/.ssh/nvair.unifabric.io exists
    │       ├─> NO: Generate Ed25519 key pair
    │       └─> YES: Use existing key pair
    │
    ├─> 2. Use provided API token directly for authenticated requests
    │
    ├─> 3. GET /v3/users/ssh-keys/
    │       └─> Check if key with name "nvair.unifabric.io-cli" and matching fingerprint exists
    │
    └─> 4. If public key not found:
            └─> POST /v3/users/ssh-keys/
                └─> Upload public key with name "nvair.unifabric.io-cli"
```

## Implementation Details

### Step 1: Key Pair Generation

**Location**:
- Private key: `$HOME/.ssh/nvair.unifabric.io`
- Public key: `$HOME/.ssh/nvair.unifabric.io.pub`

**Key Type**: Ed25519 (modern, secure, compact)

**Generation (Go)**:
```go
import (
    "crypto/ed25519"
    "crypto/rand"
    "encoding/pem"
    "golang.org/x/crypto/ssh"
    "os"
)

func generateSSHKeyPair(privateKeyPath, publicKeyPath string) error {
    // Generate Ed25519 key pair
    publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        return fmt.Errorf("generate key: %w", err)
    }

    // Encode private key to PEM format (OpenSSH format)
    privateKeyPEM, err := ssh.MarshalPrivateKey(privateKey, "")
    if err != nil {
        return fmt.Errorf("marshal private key: %w", err)
    }

    // Write private key with 0600 permissions
    err = os.WriteFile(privateKeyPath, pem.EncodeToMemory(privateKeyPEM), 0600)
    if err != nil {
        return fmt.Errorf("write private key: %w", err)
    }

    // Generate SSH public key
    sshPublicKey, err := ssh.NewPublicKey(publicKey)
    if err != nil {
        return fmt.Errorf("create ssh public key: %w", err)
    }

    // Format: "ssh-ed25519 AAAAC3... comment"
    pubKeyBytes := ssh.MarshalAuthorizedKey(sshPublicKey)

    // Write public key with 0644 permissions
    err = os.WriteFile(publicKeyPath, pubKeyBytes, 0644)
    if err != nil {
        return fmt.Errorf("write public key: %w", err)
    }

    return nil
}
```

**Permissions**:
- Private key: `0600` (user read-write only)
- Public key: `0644` (user read-write, others read)

### Step 2: Check if Key Exists

```go
func sshKeyPairExists() bool {
    privateKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", "air.nvidia.com")
    publicKeyPath := privateKeyPath + ".pub"
    
    _, errPriv := os.Stat(privateKeyPath)
    _, errPub := os.Stat(publicKeyPath)
    
    return errPriv == nil && errPub == nil
}
```

**Important**: If either key is missing, regenerate both. Never regenerate if both exist.

### Step 3: Calculate Fingerprint

```go
func calculateFingerprint(publicKeyPath string) (string, error) {
    pubKeyBytes, err := os.ReadFile(publicKeyPath)
    if err != nil {
        return "", err
    }

    // Parse SSH public key
    pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubKeyBytes)
    if err != nil {
        return "", err
    }

    // Get SHA256 fingerprint
    fingerprint := ssh.FingerprintSHA256(pubKey)
    return fingerprint, nil
}
```

Output format: `SHA256:abc123def456...`

### Step 4: Check if Key Exists in Account

```go
func publicKeyExistsInAccount(client *http.Client, apiToken, fingerprint, apiEndpoint string) (bool, error) {
    req, _ := http.NewRequest("GET", apiEndpoint+"/v3/users/ssh-keys/", nil)
    req.Header.Set("Authorization", "Bearer "+apiToken)

    resp, err := client.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()

    var result []struct {
        ID          string `json:"id"`
        Name        string `json:"name"`
        Fingerprint string `json:"fingerprint"`
    }

    json.NewDecoder(resp.Body).Decode(&result)

    // Check if key with name "nvair.unifabric.io-cli" and matching fingerprint exists
    for _, key := range result {
        if key.Name == "nvair.unifabric.io-cli" && key.Fingerprint == fingerprint {
            return true, nil
        }
    }

    return false, nil
}
```

### Step 5: Upload Public Key

```go
func uploadPublicKey(client *http.Client, apiToken, publicKeyPath, apiEndpoint string) error {
    pubKeyBytes, err := os.ReadFile(publicKeyPath)
    if err != nil {
        return err
    }

    payload := map[string]string{
        "public_key": string(pubKeyBytes),
        "name":       "nvair.unifabric.io-cli",
    }

    payloadBytes, _ := json.Marshal(payload)

    req, _ := http.NewRequest("POST", 
        apiEndpoint+"/v3/users/ssh-keys/", 
        bytes.NewBuffer(payloadBytes))
    req.Header.Set("Authorization", "Bearer "+apiToken)
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode == 409 {
        // Key already exists (race condition)
        return nil
    }

    if resp.StatusCode != 201 {
        return fmt.Errorf("upload failed: %d", resp.StatusCode)
    }

    return nil
}
```

## Complete Login Flow

```go
func Login(username, apiToken string) error {
    privateKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", "air.nvidia.com")
    publicKeyPath := privateKeyPath + ".pub"
    apiEndpoint := "https://air.nvidia.com/api"

    // 1. Generate SSH key pair if not exists
    if !sshKeyPairExists() {
        fmt.Println("Generating SSH key pair...")
        if err := generateSSHKeyPair(privateKeyPath, publicKeyPath); err != nil {
            return fmt.Errorf("generate SSH keys: %w", err)
        }
        fmt.Printf("✓ SSH key pair created at %s\n", privateKeyPath)
    } else {
        fmt.Println("✓ Using existing SSH key pair")
    }

    // 2. Use the provided API token directly for authenticated requests.

    // 3. Check if public key exists in account
    fingerprint, err := calculateFingerprint(publicKeyPath)
    if err != nil {
        return fmt.Errorf("calculate fingerprint: %w", err)
    }

    exists, err := publicKeyExistsInAccount(httpClient, apiToken, fingerprint, apiEndpoint)
    if err != nil {
        return fmt.Errorf("check public key: %w", err)
    }

    // 4. Upload public key if not found
    if !exists {
        fmt.Println("Uploading public key to nvidia account...")
        if err := uploadPublicKey(httpClient, apiToken, publicKeyPath, apiEndpoint); err != nil {
            return fmt.Errorf("upload public key: %w", err)
        }
        fmt.Printf("✓ Public key uploaded (fingerprint: %s)\n", fingerprint)
    } else {
        fmt.Println("✓ Public key already exists in account")
    }

    // 5. Save configuration
    config := Configuration{
        Username:    username,
        APIToken:    apiToken,
        APIEndpoint: apiEndpoint,
    }

    if err := saveConfig(config); err != nil {
        return fmt.Errorf("save config: %w", err)
    }

    fmt.Println("✓ Login successful!")
    return nil
}
```

## Error Handling

### Key Generation Failures

**Insufficient Permissions**:
```
Error: failed to write private key: permission denied
Solution: Ensure ~/.ssh directory exists and is writable
```

**Disk Full**:
```
Error: failed to write public key: no space left on device
Solution: Free up disk space
```

### Upload Failures

**409 Conflict**:
- Key already exists (possibly race condition or manual upload)
- Treat as success, continue login

**400 Bad Request**:
- Invalid key format (should not happen with generated keys)
- Log error and abort login

**Network Errors**:
- Retry up to 3 times with exponential backoff
- If still failing, warn user but continue (they can upload manually later)

## User Experience

### First Login

```
$ nvair login -u user@example.com -p nvair_abc123
Generating SSH key pair...
✓ SSH key pair created at /home/user/.ssh/nvair.unifabric.io
Authenticating...
✓ Authentication successful
Uploading public key to nvidia account...
✓ Public key uploaded (fingerprint: SHA256:abc123def456...)
✓ Configuration saved
✓ Login successful!
```

### Subsequent Login (key pair exists)

```
$ nvair login -u user@example.com -p nvair_abc123
✓ Using existing SSH key pair
Authenticating...
✓ Authentication successful
✓ Public key already exists in account
✓ Configuration saved
✓ Login successful!
```

## Security Considerations

1. **Private Key Protection**: Never upload or transmit private key
2. **Permissions**: Enforce strict permissions (0600 for private, 0644 for public)
3. **Key Rotation**: Users can regenerate keys by deleting existing pair before login
4. **Fingerprint Validation**: Always verify fingerprint matches before using key
5. **Secure Storage**: Keys stored in standard SSH directory (`~/.ssh/`)

## Testing

### Unit Tests

```go
func TestSSHKeyGeneration(t *testing.T) {
    tmpDir := t.TempDir()
    privPath := filepath.Join(tmpDir, "test_key")
    pubPath := privPath + ".pub"

    err := generateSSHKeyPair(privPath, pubPath)
    assert.NoError(t, err)

    // Verify files exist
    assert.FileExists(t, privPath)
    assert.FileExists(t, pubPath)

    // Verify permissions
    privInfo, _ := os.Stat(privPath)
    assert.Equal(t, 0600, privInfo.Mode().Perm())

    pubInfo, _ := os.Stat(pubPath)
    assert.Equal(t, 0644, pubInfo.Mode().Perm())

    // Verify key format
    pubKeyBytes, _ := os.ReadFile(pubPath)
    assert.Contains(t, string(pubKeyBytes), "ssh-ed25519")
}
```

### Integration Tests

Mock API server to test:
- Public key upload
- Duplicate key detection (409 response)
- Invalid key format (400 response)

---

**References**:
- Ed25519 key format: RFC 8032
- OpenSSH key format: PROTOCOL.key
- golang.org/x/crypto/ssh package documentation
