# Tasks: Status Command

**Input**: Design documents from `/specs/005-status-command/`  
**Prerequisites**: `spec.md` (required)  
**Scope Note**: Generated from `spec.md` only for this simplified feature; no additional design artifacts are required.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the new CLI command entrypoint and package scaffold.

- [X] T001 Create `status` command scaffold with `Command`, `NewCommand()`, and `Register(cmd)` in `pkg/commands/status/status.go`
- [X] T002 Register `status` as a root subcommand with `Use`, `Short`, and `RunE` wiring in `pkg/commands/root.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the shared session and connectivity logic required before the user story is complete.

**⚠️ CRITICAL**: No user story work should begin until this phase is complete.

- [X] T003 Implement local session loading and usable-session detection in `pkg/commands/status/status.go`
- [X] T004 Implement expired-token refresh handling and unrecoverable-session fallback in `pkg/commands/status/status.go`
- [X] T005 Implement an authenticated nvair connectivity probe for the current user in `pkg/commands/status/status.go`

**Checkpoint**: The command can determine whether a usable local session exists and whether nvair connectivity can be checked safely.

---

## Phase 3: User Story 1 - Check Login And Connectivity Status (Priority: P1) 🎯 MVP

**Goal**: Deliver `nvair status` so operators can see the current logged-in user and whether that user can connect to nvair, or clearly see that no user is logged in.

**Independent Test**: Run `nvair status` with no config, with a valid session that can reach nvair, and with a valid session that cannot reach nvair; verify all three outputs are distinct and match the specification.

- [X] T006 [US1] Implement the no-session and unusable-session result branch in `pkg/commands/status/status.go`
- [X] T007 [US1] Implement the logged-in and can-connect result branch in `pkg/commands/status/status.go`
- [X] T008 [US1] Implement the logged-in and cannot-connect result branch in `pkg/commands/status/status.go`
- [X] T009 [US1] Assemble the final `Execute()` flow to load session state, refresh tokens when possible, probe nvair, and select the correct result branch in `pkg/commands/status/status.go`
- [X] T010 [US1] Finalize human-readable output wording and verbose logging for all three states in `pkg/commands/status/status.go`

**Checkpoint**: User Story 1 is fully functional and independently testable.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Improve discoverability and operator guidance for the new command.

- [X] T011 [P] Document `nvair status` usage and example outputs in `docs/usage/cli-reference.md`
- [X] T012 [P] Add troubleshooting guidance for `not logged in` and `cannot connect` results in `docs/usage/troubleshooting.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Setup (Phase 1): No dependencies.
- Foundational (Phase 2): Depends on Setup completion; blocks user story work.
- User Story 1 (Phase 3): Depends on Foundational completion.
- Polish (Phase 4): Depends on User Story 1 completion.

### User Story Dependencies

- **US1 (P1)**: Starts after the Foundational phase and has no dependency on any other user story.

### Within User Story 1

- Implement the three result branches before assembling the final execution flow.
- Assemble the final flow before polishing message wording and verbose logging.

---

## Parallel Opportunities

- `T011` and `T012` can run in parallel after User Story 1 is complete because they update different documentation files.
- Core implementation tasks in `pkg/commands/status/status.go` should stay sequential to avoid same-file conflicts.

### Parallel Example: User Story 1

```bash
# User Story 1 is centered on one implementation file, so keep it sequential:
T006 -> T007 -> T008 -> T009 -> T010
```

### Parallel Example: Polish

```bash
T011
T012
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup).
2. Complete Phase 2 (Foundational).
3. Complete Phase 3 (User Story 1).
4. Validate the three required status outcomes before moving to documentation.

### Incremental Delivery

1. Deliver the command scaffold and root wiring.
2. Deliver session classification and connectivity probing.
3. Deliver the three user-visible status branches.
4. Finish docs and troubleshooting guidance.

### Team Strategy

1. One developer implements Phases 1-3 in `pkg/commands/status/status.go` and `pkg/commands/root.go`.
2. One developer updates `docs/usage/cli-reference.md` and `docs/usage/troubleshooting.md` after the command behavior is finalized.

---

## Notes

- Generated from `spec.md` only because this feature intentionally skipped `plan.md`, `research.md`, `contracts/`, and other design artifacts.
- No explicit test tasks were added because the feature specification and user request did not require TDD or dedicated test-task generation.
- All tasks follow the required checklist format: checkbox, task ID, optional `[P]`, optional `[US1]`, and exact file path.
