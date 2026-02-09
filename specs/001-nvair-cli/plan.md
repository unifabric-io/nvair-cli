# Development Plan: nvcli Login Feature

**Feature**: Login Feature (nvcli login)  
**Branch**: `001-nvair-cli`  
**Date**: February 9, 2026  
**Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/specs/001-nvair-cli/spec.md`

## Executive Summary

The nvcli Login Feature implements a single sign-on flow that:
1. Exchanges an API token for a bearer token via POST `/v1/auth/login`
2. Manages Ed25519 SSH key pair generation and storage
3. Registers the public key with the platform via POST `/v1/sshkey`
4. Persists credentials securely with strict file permissions
5. Handles token refresh and transient network failures gracefully

**Delivery Timeline**: Phase-based implementation with clear milestones  
**Performance Target**: 95% of logins complete in <5 seconds under normal conditions  
**Testing Strategy**: Unit tests, integration tests, and closed-loop CI test

---

## Phase 0: Research & Design

### Objectives
- Validate API contracts and endpoint behavior
- Design data models and configuration structure
- Identify dependencies and implementation patterns

### Deliverables

#### 1. **Research Document** → [research.md](research.md)
- API contract review (`/v1/auth/login`, `/v1/sshkey`)
- SSH key generation libraries and best practices (Ed25519)
- Go cryptography packages (`golang.org/x/crypto/ssh`)
- Configuration file storage patterns
- Token refresh lifecycle
- Error handling patterns for transient failures

#### 2. **Data Model** → [data-model.md](data-model.md)
```
Config Structure:
- Email (string)
- ApiToken (string, masked in output)
- BearerToken (string, masked in output)
- BearerTokenExpiresAt (RFC3339 timestamp)
- SshKeyPath (string, default: ~/.ssh/nvair.unifabric.io)
- LastLoginAt (RFC3339 timestamp)
```

File Permissions:
- Config file: 0600 (read/write owner only)
- SSH private key: 0600
- SSH public key: 0644

#### 3. **API Contracts** → [contracts/api.md](contracts/api.md)
Document:
- `POST /v1/auth/login` request/response schema
- `GET /v1/sshkey` request/response schema
- `POST /v1/sshkey` request/response schema
- Error codes (400, 401, 409, 5xx)
- Retry behavior and rate limits

#### 4. **Installation Commands** → [installation-commands.md](installation-commands.md)
Required tools:
- Go 1.22+
- Git
- SSH client (standard)
- (Optional) OpenSSH for key generation reference

#### 5. **Quick Start Guide** → [quickstart.md](quickstart.md)
Developer setup:
- Clone and branch setup
- Building the CLI locally
- Running first test login
- Common troubleshooting

---

## Phase 1: Implementation Planning

### Module Breakdown

#### Module 1: Configuration Management
**Files**: `pkg/config/model.go`, `pkg/config/loader.go`

**Responsibilities**:
- Define Config struct (email, tokens, expiry, ssh key path)
- Load config from `$HOME/.config/nvair.unifabric.io/config.json`
- Save config with 0600 file permissions
- Validate config structure and required fields
- Handle config not found gracefully

**Dependencies**: Go standard library (encoding/json, os, path/filepath)

**Tests**:
- Load non-existent config → error
- Load valid config → parse correctly
- Save config → verify 0600 permissions
- Round-trip: save then load → identical

---

#### Module 2: SSH Key Management
**Files**: `pkg/ssh/keygen.go`, `pkg/ssh/remote.go`

**Responsibilities**:
- Generate Ed25519 key pair if missing
- Write private key with 0600 permissions
- Write public key with 0644 permissions
- Compute public key fingerprint (SHA256)
- Load existing key pair from disk
- Format public key for API submission

**Dependencies**: 
- `golang.org/x/crypto/ssh` (key generation)
- Go standard library (crypto/sha256, encoding, os)

**Tests**:
- Generate new key pair → both files exist with correct permissions
- Load existing key pair → parse correctly
- Compute fingerprint → matches OpenSSH format
- Idempotency: generate twice → second call is no-op

---

#### Module 3: HTTP API Client
**Files**: `pkg/api/client.go`, `pkg/api/endpoints.go`

**Responsibilities**:
- HTTP client with bearer token authentication
- Automatic token injection in Authorization header
- Retry logic with exponential backoff (up to 3 retries)
- Transient failure detection (5xx, network timeout)
- Request/response logging for debugging

**Dependencies**: 
- `net/http` or third-party (Resty)
- `time` (backoff timing)

**Request Methods**:
- `AuthLogin(email, apiToken)` → BearerToken, ExpiresAt, error
- `GetSshKey(keyName)` → PublicKey, Fingerprint, error (404 = not found)
- `CreateSshKey(keyName, publicKeyPem, fingerprint)` → Created/Conflict error

**Error Handling**:
- 400/401 → permanent failure, return user error
- 5xx → transient, retry with backoff
- Network timeout → transient, retry
- Permanent failure after 3 retries → return contextual error

**Tests**:
- Successful auth → token stored with expiry
- Invalid credentials (401) → error, no retry
- Transient 503 → retry succeeds on 2nd attempt
- All 3 retries fail → final error returned
- Malformed response → error handling

---

#### Module 4: Login Command
**Files**: `pkg/commands/login.go`

**Responsibilities**:
- Parse `nvcli login -u <email> -p <api-token>` arguments
- Call AuthLogin via API client
- Trigger SSH key generation if missing
- Compute key fingerprint and check existing key
- Upload public key if not registered
- Save config with new credentials
- Display success/warning messages to user

**Workflow**:
```
1. Parse -u (email) and -p (api-token) flags
2. Validate email format (basic RFC5322 check)
3. Call api.AuthLogin(email, apiToken)
   ├─ Success → proceed to step 4
   └─ Failure → return error to user
4. Ensure SSH key pair exists (generate if missing)
5. Load public key and compute fingerprint
6. Call api.GetSshKey(keyName) → check if registered
   ├─ Found with same fingerprint → skip upload
   ├─ Not found → proceed to step 7
   └─ Error (5xx) → warn but continue
7. Call api.CreateSshKey(keyName, pubKey, fingerprint)
   ├─ 201 Created → continue
   ├─ 409 Conflict → key exists, continue
   └─ Error → warn but complete login (don't block)
8. Save config to disk with 0600 perms
9. Display "Login successful" message
```

**User Flags**:
- `-u, --user <email>` (required)
- `-p, --password <api-token>` (required)

**Output Messages**:
- Success: "✓ Login successful. Credentials saved to ~/.config/nvair.unifabric.io/config.json"
- Warning: "⚠ Login successful but SSH key upload failed. Your public key may not be registered."
- Error: Clear, actionable messages for each failure scenario

**Tests**:
- First-time login: full flow succeeds
- Subsequent login: token refresh with existing key
- Missing email flag → error
- Invalid email format → error
- API returns 401 → error, no key generation
- Key generation succeeds, upload fails → warning but login completes
- Network timeout on key check → warn and continue

---

#### Module 5: Integration & Error Handling
**Files**: `pkg/commands/root.go`, `pkg/output/errors.go`

**Responsibilities**:
- Root command registration
- Global error formatting
- Structured logging
- Context propagation

**Error Categories**:
1. **Validation Errors** (user input): "Invalid email format"
2. **Authentication Errors** (401): "Invalid credentials"
3. **Transient Errors** (5xx, timeout): "Network error, please retry" (after backoff)
4. **SSH Key Errors**: "Failed to generate SSH key"
5. **File Permission Errors**: "Cannot write to ~/.ssh"
6. **Partial Failures**: "Login successful but SSH key upload failed"

---

### Implementation Sequence

**Critical Path** (must complete in order):
1. Configuration model & file I/O (`pkg/config/`)
2. SSH key generation (`pkg/ssh/keygen.go`)
3. HTTP API client with retry logic (`pkg/api/client.go`)
4. API endpoints wrapper (`pkg/api/endpoints.go`)
5. Login command implementation (`pkg/commands/login.go`)
6. Root command registration (`pkg/commands/root.go`)

**Parallel Work** (can start after steps 1-3):
- Unit tests for each module
- Golden file tests for CLI output
- Mock API server for integration tests

---

## Phase 2: Testing Strategy

### Unit Tests
**Target**: 85%+ code coverage

- Configuration: load/save, permissions
- SSH: key generation, fingerprint, idempotency
- API: retry logic, error handling, token injection
- Login command: flag parsing, workflow, error scenarios

### Integration Tests
**Setup**: Mock HTTP server that simulates API behavior

- Scenario A (First-time): Full login with key generation
- Scenario B (Token refresh): Existing key, new token
- Transient failure recovery: 503 → success
- Permanent failure: 401 immediately
- Key conflict: 409 → treated as success

### CI/CD Test
**Closed-loop test** in GitHub Actions:
- Build CLI
- Start mock API server
- Execute full login flow
- Verify config file created with correct perms
- Verify SSH keys created
- Verify tokens in config
- Report results in PR checks

---

## Phase 3: Success Criteria Validation

### Performance
- ✓ Measure login end-to-end time
- ✓ Confirm 95% complete in <5 seconds

### Functional Requirements
- ✓ `nvcli login -u <email> -p <token>` exchanges token for bearer
- ✓ Bearer token and expiry saved with 0600 perms
- ✓ SSH key pair generated/confirmed with correct perms
- ✓ Public key fingerprint computed and uploaded
- ✓ Network retries work (3 attempts with backoff)
- ✓ Partial failures don't block login

### Code Quality
- ✓ 85%+ test coverage
- ✓ No hardcoded secrets in code
- ✓ Proper error context and user messages
- ✓ Clean architecture (cmd → internal → pkg separation)

---

## Dependency & Risk Assessment

### External Dependencies
| Dependency | Risk | Mitigation |
|-----------|------|-----------|
| `/v1/auth/login` API | Medium | Contract validation in Phase 0 research |
| `/v1/sshkey` API | Medium | Comprehensive error handling for 409 |
| Ed25519 support | Low | golang.org/x/crypto is stable |
| SSH key permissions | Low | Standard POSIX, tested on multiple platforms |
| User home dir access | Low | Standard assumption validated in Phase 0 |

### Implementation Risks
| Risk | Impact | Mitigation |
|------|--------|-----------|
| Token expiry handling | High | Clear config structure, explicit ExpiresAt field |
| SSH key collision | Medium | Check existing key before upload, handle 409 |
| File permission errors | Medium | Validate write access early, clear error messages |
| Network timeouts | Medium | Exponential backoff with max 3 retries |

---

## Acceptance Criteria Checklist

- [ ] **Spec Validation**: Spec document is approved and no [NEEDS CLARIFICATION] markers remain
- [ ] **API Contracts**: `/v1/auth/login` and `/v1/sshkey` contracts documented
- [ ] **Data Model**: Config structure defined and file format chosen (JSON)
- [ ] **Module 1**: Configuration I/O complete with tests (load/save/perms)
- [ ] **Module 2**: SSH key generation complete with tests (gen/load/fingerprint)
- [ ] **Module 3**: HTTP client with retry logic complete and tested
- [ ] **Module 4**: Login command implemented with full workflow
- [ ] **Module 5**: Error handling and root command integration complete
- [ ] **Unit Tests**: 85%+ code coverage, all modules tested
- [ ] **Integration Tests**: Mock API server, scenario A & B passing
- [ ] **CI Test**: Closed-loop test in GitHub Actions, visible in PR
- [ ] **Performance**: 95% of logins complete in <5 seconds (measured)
- [ ] **Code Review**: Code reviewed, no security concerns
- [ ] **PR Ready**: All tests passing, ready for merge

---

## Next Steps

1. **Move to Phase 0**: Execute `/speckit.plan` or review Phase 0 research
2. **Clarification**: Address any [NEEDS CLARIFICATION] markers in spec
3. **Phase 1 Implementation**: Begin with configuration module
4. **Parallel Testing**: Write tests alongside implementation
5. **Integration**: Assemble modules into login command
6. **Validation**: Run CI tests and performance benchmarks
7. **Code Review**: Prepare PR with linked spec and test results
