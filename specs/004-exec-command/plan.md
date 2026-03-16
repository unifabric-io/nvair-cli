# Implementation Plan: Exec Command

**Branch**: `004-exec-command` | **Date**: 2026-03-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-exec-command/spec.md`

## Summary

Add `nvair exec` command to execute commands on simulation nodes via SSH through the bastion host:
- Non-interactive mode: `nvair exec <node> -s <sim> -- <command>` — runs command, prints output, exits with remote exit code
- Interactive mode: `nvair exec <node> -s <sim>` — opens interactive SSH shell with full TTY support

The command reuses the existing bastion/SSH infrastructure from `create`. It resolves the simulation and node via the API, looks up the existing SSH service, and connects through the bastion (oob-mgmt-server) to the target node. Node type (host vs switch) determines credentials. The `-s` flag is required.

New API method needed: `GetServices(simulationID)` to look up existing SSH service — uses `GET /v1/service?simulation=<id>`. New bastion functions needed for interactive SSH sessions with PTY support (terminal resize, signal forwarding).

## Technical Context

**Language/Version**: Go 1.25.6  
**Primary Dependencies**: `github.com/spf13/cobra`, `golang.org/x/crypto/ssh`, existing `pkg/api`, `pkg/bastion`, `pkg/ssh`  
**Storage**: N/A (read-only — config and SSH keys already on disk from `login`/`create`)  
**Testing**: `make test-unit`, `make test-e2e`  
**Target Platform**: Cross-platform CLI (Linux/macOS/Windows)  
**Project Type**: Single Go CLI project  
**Constraints**: `-s` flag is required; SSH service must already exist (created by `nvair create`); must distinguish host nodes (ubuntu/nvidia) from switch nodes (cumulus/Dangerous1#) and bastion (direct connect)  
**Scale/Scope**: One new command (`exec`) + new API method (`GetServices`) + bastion interactive session support + tests

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] Command/flag behavior is defined using existing Cobra command patterns.
- [x] Validation and runtime errors are defined with non-zero exit behavior.
- [x] Structured output impact: N/A — exec outputs raw command stdout/stderr, not structured resources.
- [x] Backward compatibility: new command only, no changes to existing commands.
- [x] Tests required: unit tests for node resolution, credential selection, flag validation; e2e tests for exec flow.

Post-Phase 1 Re-check:
- Design stays within existing CLI architecture. No constitution violations.
- Result: PASS.

## Project Structure

### Documentation (this feature)

```text
specs/004-exec-command/
├── plan.md              # This file
├── tasks.md             # Phase 2 output (/speckit.tasks)
└── checklists/
    └── requirements.md
```

Post-implementation update: `docs/quickstart.md`

### Source Code (repository root)

```text
pkg/
├── commands/
│   ├── root.go              # Add newExecCommand()
│   └── exec/
│       ├── exec.go          # Command struct, Register(), Execute()
│       └── exec_test.go     # Unit tests
├── api/
│   ├── client.go            # Add GetServices() method
│   └── client_test.go       # Add GetServices test
└── bastion/
    ├── bastion.go           # Add interactive session functions
    └── bastion_test.go      # Add interactive session tests
```

## Implementation Modules

### Module 1: API Client — GetServices

Add `GetServices(simulationID string) ([]EnableSSHResponse, error)` to `pkg/api/client.go`.

- Calls `GET /v1/service?simulation=<simulationID>`
- Returns list of services; caller filters for `service_type == "ssh"` to find the bastion SSH service
- Returns the `Host` and `SrcPort` needed to connect to the bastion externally

### Module 2: Bastion — Interactive Session Support

Add to `pkg/bastion/bastion.go`:

- `InteractiveSessionViaBastion(cfg BastionExecConfig) error` — opens an interactive SSH shell on a target node through the bastion. Requests PTY, attaches stdin/stdout/stderr, handles `SIGWINCH` for terminal resize, forwards signals (Ctrl+C → remote).
- `InteractiveSessionOnBastion(cfg BastionExecConfig) error` — same but directly on bastion (for `oob-mgmt-server` target).
- Helper: `startInteractiveSession(client *ssh.Client) error` — shared PTY + raw terminal + resize logic.

### Module 3: Exec Command

New `pkg/commands/exec/` package following existing command pattern:

**Command struct fields**: `SimulationName`, `APIEndpoint`, `Verbose`

**Register()**: flags `-s, --simulation` (required), positional arg `<node-name>`, `ArgsLenAtDash` for `--` separator

**Execute(args []string) flow**:
1. Load config, create API client (`ensureAuthenticatedClient`)
2. Resolve simulation by name → simulation ID (`resolveSimulationID`)
3. Get services for simulation → find SSH service (`service_type == "ssh"`) → extract `Host`, `SrcPort` for bastion address. If not found → error: "SSH service not found, run nvair create first"
4. Get nodes for simulation → find target node by name → extract `mgmt_ip` from metadata. If not found → error listing available nodes
5. Determine credentials by node type:
   - Node name == `oob-mgmt-server`: direct bastion connect (public key auth, user `ubuntu`)
   - Node image contains `cumulus`: `cumulus` / `Dangerous1#`
   - Otherwise (generic/ubuntu): `ubuntu` / `nvidia`
6. Load SSH key path (`ssh.DefaultKeyPath()`)
7. If command args present (after `--`): non-interactive → `bastion.ExecCommandViaBastion()` or `ExecCommandOnBastion()` → print stdout/stderr → exit with remote exit code
8. If no command args: interactive → `bastion.InteractiveSessionViaBastion()` or `InteractiveSessionOnBastion()`

### Module 4: Root Command Registration

Add `rc.newExecCommand()` in `pkg/commands/root.go`:
- Use: `exec <node-name>`
- Pass `args` and `cmd.ArgsLenAtDash()` to exec command for `--` separation
- Pass `rc.Verbose` flag

### Module 5: Update docs/quickstart.md

After implementation, update `docs/quickstart.md`:
- Remove `(TODO)` from exec usage examples
- Add exec usage examples for both non-interactive and interactive modes

### Module 6: E2E Test — Extend TestIntegration_RealAPI_Create

Extend `e2e/create_test.go` `TestIntegration_RealAPI_Create` to validate exec after simulation creation:
- For each generic (ubuntu) node, run `nvair exec <node> -s <sim> -- ip -j addr` and parse the output
- Extract IP addresses from `ip -j addr` output and compare against the expected IPs defined in the local topology netplan YAML files
- This validates the full create → exec pipeline end-to-end: simulation was provisioned correctly, exec connects through the bastion, and netplan configs were applied with correct IPs

## Complexity Tracking

No constitution violations. No new data models or API contracts documents needed — the feature reuses existing patterns and the `GET /v1/service` endpoint is already documented in `docs/design/api.md`.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
