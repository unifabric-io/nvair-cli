# Tasks: Exec Command

**Input**: Design documents from `/specs/004-exec-command/`
**Prerequisites**: `plan.md` (required), `spec.md` (required)

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the new `GetServices` API method and extend bastion with interactive session support — these are prerequisites for the exec command.

- [X] T001 Add `GetServices(simulationID string) ([]EnableSSHResponse, error)` method to `pkg/api/client.go` that calls `GET /v1/service?simulation=<simulationID>` and returns the list of services
- [X] T002 [P] Add unit test for `GetServices` in `pkg/api/client_test.go` verifying successful deserialization and error handling
- [X] T003 Add `InteractiveSessionViaBastion(cfg BastionExecConfig) error` to `pkg/bastion/bastion.go` — connects through bastion to target, requests PTY, attaches stdin/stdout/stderr, handles SIGWINCH for terminal resize, forwards signals
- [X] T004 Add `InteractiveSessionOnBastion(cfg BastionExecConfig) error` to `pkg/bastion/bastion.go` — same as T003 but connects directly to bastion (for oob-mgmt-server target)
- [X] T005 [P] Extract shared `startInteractiveSession(client *ssh.Client) error` helper in `pkg/bastion/bastion.go` — PTY request, raw terminal mode, resize listener, stdin/stdout/stderr wiring, wait for session end

**Checkpoint**: API client can list services; bastion supports both non-interactive (existing) and interactive SSH sessions.

---

## Phase 2: User Story 1 — Execute Commands on Simulation Nodes (Priority: P1) 🎯 MVP

**Goal**: Deliver `nvair exec <node-name> -s <simulation> [-- <command>]` supporting both non-interactive and interactive modes, with proper node type credential selection and error handling.

**Independent Test**: Run `nvair exec node-gpu-1 -s simple -- hostname` and verify correct output; run `nvair exec node-gpu-1 -s simple` and verify interactive shell opens.

### Implementation

- [X] T006 [US1] Create `pkg/commands/exec/exec.go` with `Command` struct (fields: `SimulationName`, `APIEndpoint`, `Verbose`), `NewCommand()` constructor, and `Register(cmd)` method with required `-s, --simulation` flag
- [X] T007 [US1] Implement `ensureAuthenticatedClient` helper in `pkg/commands/exec/exec.go` (load config, validate token, create API client — same pattern as `get` and `create`)
- [X] T008 [US1] Implement `resolveSimulationID` helper in `pkg/commands/exec/exec.go` — fetch simulations via API, match by title, return ID or "simulation not found" error
- [X] T009 [US1] Implement `findSSHService` helper in `pkg/commands/exec/exec.go` — call `GetServices(simulationID)`, filter for `service_type == "ssh"`, return `Host` and `SrcPort` or error "SSH service not found, run nvair create first"
- [X] T010 [US1] Implement `findNodeByName` helper in `pkg/commands/exec/exec.go` — call `GetNodes(simulationID)`, match by name, parse `mgmt_ip` from metadata, return node info or error listing available node names
- [X] T011 [US1] Implement `resolveCredentials` helper in `pkg/commands/exec/exec.go` — determine user/password by node type: oob-mgmt-server → direct bastion (ubuntu, key-only), image contains "cumulus" → cumulus/Dangerous1#, otherwise → ubuntu/nvidia
- [X] T012 [US1] Implement `Execute(args []string, dashIndex int)` method in `pkg/commands/exec/exec.go` — orchestrate full flow: auth → resolve simulation → find SSH service → find node → resolve credentials → build BastionExecConfig → dispatch to non-interactive or interactive mode based on dashIndex and args
- [X] T013 [US1] Add `newExecCommand()` method to `pkg/commands/root.go` — register `exec` command with `Use: "exec <node-name>"`, pass `args`, `cmd.ArgsLenAtDash()`, and `rc.Verbose` to exec command; add to `rootCmd.AddCommand()`
- [X] T014 [P] [US1] Add unit tests in `pkg/commands/exec/exec_test.go` — test `resolveCredentials` (ubuntu node, cumulus switch, oob-mgmt-server), test missing `-s` flag error, test node-not-found error message includes available nodes, test simulation-not-found error

**Checkpoint**: User Story 1 is functional — both non-interactive and interactive exec modes work end-to-end.

---

## Phase 3: Polish & Cross-Cutting Concerns

**Purpose**: Documentation updates and e2e test coverage.

- [X] T015 [P] Update `docs/quickstart.md` — remove `(TODO)` from exec usage example, add non-interactive example (`nvair exec node-gpu-1 -s simple -- hostname`) and interactive example (`nvair exec node-gpu-1 -s simple`)
- [X] T016 Extend `TestIntegration_RealAPI_Create` in `e2e/create_test.go` — after simulation creation, for each of `node-gpu-1` through `node-gpu-4` (hardcoded):
  1. Parse the corresponding netplan YAML (`examples/simple/node-gpu-1.yaml` ~ `node-gpu-4.yaml`) to extract expected IPs for all interfaces defined in the file (eth1-eth9; eth0 is not present in netplan so naturally excluded)
  2. Run `nvair exec <node> -s <sim-name> -- ip -j addr` via the exec command
  3. Parse the JSON output from `ip -j addr`, extract actual IPs per interface name
  4. Assert: for each interface in the netplan YAML (eth1-eth9), the actual IP on the node matches the expected IP from the netplan definition
  5. This validates that netplan configurations submitted during `nvair create` were correctly applied to the simulation nodes

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately.
- **User Story 1 (Phase 2)**: Depends on Phase 1 completion (needs `GetServices` and interactive bastion support).
- **Polish (Phase 3)**: Depends on Phase 2 completion.

### Within Phase 1

- `T001` and `T002` (API) can run in parallel with `T003`-`T005` (bastion).
- `T003` and `T004` depend on `T005` (shared helper) or can be written together.

### Within Phase 2

- `T006` first (command scaffold).
- `T007`-`T011` are helpers — can be written in any order after `T006`.
- `T012` depends on all helpers (`T007`-`T011`).
- `T013` depends on `T006` (command must exist to register).
- `T014` can run in parallel with `T012`-`T013` (tests different files).

### Within Phase 3

- `T015` and `T016` can run in parallel.

---

## Parallel Opportunities

```text
Phase 1:
  [T001, T002] ──parallel── [T003, T004, T005]

Phase 2:
  T006 → [T007, T008, T009, T010, T011] → T012 → T013
                                           T014 ─parallel─

Phase 3:
  [T015, T016] ──parallel──
```

## Implementation Strategy

- **MVP**: Phase 1 + Phase 2 delivers a fully functional `nvair exec` command.
- **Total tasks**: 16
- **Phase 1 (Setup)**: 5 tasks
- **Phase 2 (US1)**: 9 tasks
- **Phase 3 (Polish)**: 2 tasks
- **Parallel opportunities**: Phase 1 API/bastion tracks; Phase 2 T014 parallel with T012-T013; Phase 3 both tasks parallel.
- **Suggested MVP scope**: Phase 1 + Phase 2 (all tasks T001-T014).
