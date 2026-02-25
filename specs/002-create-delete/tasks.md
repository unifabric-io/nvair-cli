# Tasks: Create and Delete Simulation Commands

**Input**: Design documents from `/specs/002-create-delete/`  
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md)  
**Branch**: `002-create-delete`

## Format Guide

- **[ID]**: Task identifier (T001, T002, ...)
- **[P]**: Parallelizable (can run independently)
- **[Story]**: User story tag (US1, US2, US3, US4)
- **Description**: Clear action with file paths

## Phase 1: Setup (Project Structure)

**Purpose**: Initialize project structure and dependencies

- [X] T001 Create topology package directory structure at `pkg/topology/`
- [X] T002 Create create and delete command file stub at `pkg/commands/create.go` and `pkg/commands/delete.go`
- [X] T003 Create test fixtures directory at `tests/fixtures/` with sample topologies
- [X] T004 [P] Update `go.mod` to add required dependencies (gopkg.in/yaml.v3 if needed)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure required before any user story implementation

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T005 Define topology data types in `pkg/topology/types.go`
  - Topology, Node, Link, Service, ValidationError structs
- [X] T006 Implement topology loader in `pkg/topology/loader.go`
  - LoadTopologyFromDirectory(), load topology.json files
  - Basic error handling for missing files
- [X] T007 Implement topology validator in `pkg/topology/validator.go`
  - ValidateTopology() function
  - Check required fields (name, nodes)
  - Return collection of validation errors
- [X] T008 [P] Extend API client with create/delete methods in `pkg/api/client.go`
  - CreateSimulation() method signature
  - DeleteSimulation() and DeleteService() method signatures
- [X] T009 Add topology validation error formatting in `pkg/output/errors.go`
  - FormatValidationErrors() function
  - Structured error display with field names

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Create Simulation from Topology (Priority: P1) 🎯 MVP

**Goal**: Enable users to deploy network topology by running `nvcli create -d <directory>` and receive simulation ID

**Independent Test**: Valid topology directory → simulation created on platform with confirmation message

### Tests for User Story 1 (Optional but recommended)

- [X] T010 [P] [US1] Unit test: topology loader handles valid topology in `tests/unit/topology_loader_test.go`
- [X] T011 [P] [US1] Unit test: all validation rules in `tests/unit/topology_validator_test.go`
- [X] T012 [P] [US1] Unit test: create command flag parsing in `tests/unit/create_command_test.go`
- [X] T013 [US1] Integration test: end-to-end create workflow with mocked API in `tests/integration/create_test.go`

### Implementation for User Story 1

- [X] T014 [P] [US1] Implement CreateCommand struct and Register() in `pkg/commands/create.go`
- [X] T015 [P] [US1] Parse `-d` directory flag in create command
- [X] T016 [P] [US1] Implement dry-run flag handling (`--dry-run`) in create command
- [X] T017 [US1] Implement directory validation in create command (check directory exists)
- [X] T018 [US1] Integrate topology loader in create command workflow
- [X] T019 [US1] Integrate topology validator in create command (display validation errors on failure)
- [X] T020 [US1] Implement authentication check before API call in create command
- [X] T021 [US1] Implement CreateSimulation() API call in `pkg/api/client.go`
  - HTTP POST to `/v1/simulations` with topology payload
  - Error handling and retry logic
- [X] T022 [US1] Implement success message formatting in create command
  - Display "✓ Simulation created successfully. ID: <id>, Name: <name>"
- [X] T023 [US1] Add verbose logging for create command
  - Log topology loading steps, validation, API calls
- [X] T024 [US1] Register "create" subcommand in `pkg/commands/root.go`

**Checkpoint**: User Story 1 complete - users can create simulations and receive confirmation

---

## Phase 4: User Story 2 - Validate Topology Before Creation (Priority: P1)

**Goal**: Enable users to validate topology configuration without creating simulation using `nvcli create -d <directory> --dry-run`

**Independent Test**: Invalid topology with --dry-run → all validation errors displayed, no API call made

### Tests for User Story 2

- [X] T025 [P] [US2] Unit test: validation error message formatting in `tests/unit/validation_test.go`
- [X] T026 [P] [US2] Unit test: dry-run prevents API calls in `tests/unit/create_dryrun_test.go`
- [X] T027 [US2] Integration test: dry-run with invalid topology in `tests/integration/dryrun_test.go`

### Implementation for User Story 2

- [X] T028 [US2] Implement dry-run logic in create command (skip API when flag set)
- [X] T029 [US2] Implement validation summary display for dry-run
  - Show "✓ Topology validation passed. Ready to create." on success
  - Show all validation errors with field names on failure
- [X] T030 [US2] Add timing info to validation (log duration in verbose mode)
- [X] T031 [US2] Add verbose logging for validation steps
  - Log each file being checked, validation rules applied

**Checkpoint**: User Story 2 complete - users can validate topologies before creation

---

## Phase 5: User Story 3 - Delete Simulation (Priority: P2)

**Goal**: Enable users to delete simulations with confirmation using `nvcli delete simulation <name>`

**Independent Test**: Valid simulation name with confirmed "yes" → simulation deleted, success message displayed

### Tests for User Story 3

- [X] T032 [P] [US3] Unit test: confirmation prompt parsing in `tests/unit/delete_confirm_test.go`
- [X] T033 [P] [US3] Unit test: delete command flag parsing in `tests/unit/delete_command_test.go`
- [X] T034 [US3] Integration test: delete simulation with mocked API in `tests/integration/delete_simulation_test.go`
- [X] T035 [US3] Integration test: delete with confirmation cancellation in `tests/integration/delete_cancel_test.go`

### Implementation for User Story 3

- [X] T036 [P] [US3] Implement DeleteCommand struct and Register() in `pkg/commands/delete.go`
- [X] T037 [P] [US3] Parse subcommand "simulation" in delete command
- [X] T038 [P] [US3] Parse resource name argument in delete command
- [X] T039 [US3] Implement user confirmation prompt for simulation deletion
  - Prompt format: "Delete simulation '<name>'? (yes/no): "
  - Accept variations: yes, y, Yes, YES (same for no)
- [X] T040 [US3] Implement authentication check before API call in delete command
- [X] T041 [US3] Implement DeleteSimulation() API call in `pkg/api/client.go`
  - HTTP DELETE to `/v1/simulations/{id}` (or by name per API contract)
  - Handle 404 Not Found error
  - Error handling and retry logic
- [X] T042 [US3] Implement success message for simulation deletion
  - Display "✓ Simulation '<name>' deleted successfully."
- [X] T043 [US3] Implement cancellation message
  - Display "Operation cancelled." when user declines
- [X] T044 [US3] Add verbose logging for delete simulation operation
- [X] T045 [US3] Register "delete" subcommand in `pkg/commands/root.go` (if not done in Phase 4)

**Checkpoint**: User Story 3 complete - users can delete simulations with confirmation

---

## Phase 6: User Story 4 - Delete Service Forwarding Rule (Priority: P3)

**Goal**: Enable users to delete service forwarding rules with confirmation using `nvcli delete service <name>`

**Independent Test**: Valid service name with confirmed "yes" → service deleted, success message displayed

### Tests for User Story 4

- [ ] T046 [P] [US4] Unit test: delete service command parsing in `tests/unit/delete_service_test.go`
- [ ] T047 [US4] Integration test: delete service with mocked API in `tests/integration/delete_service_test.go`

### Implementation for User Story 4

- [X] T048 [P] [US4] Parse "service" subcommand in delete command
- [X] T049 [P] [US4] Parse service name argument in delete command
- [X] T050 [US4] Implement user confirmation prompt for service deletion
  - Prompt format: "Delete service '<name>'? (yes/no): "
- [X] T051 [US4] Implement DeleteService() API call in `pkg/api/client.go`
  - HTTP DELETE to `/v1/services/{id}` (or by name per API contract)
  - Handle 404 Not Found error
  - Error handling and retry logic
- [X] T052 [US4] Implement success/cancellation messages for service deletion
- [X] T053 [US4] Add verbose logging for delete service operation
- [X] T054 [US4] Test both subcommands through root command router

**Checkpoint**: User Story 4 complete - users can delete services and simulations

---

## Phase 7: Polish & Integration

**Purpose**: Integration testing, documentation, and release readiness

### Integration & Testing

- [ ] T055 [P] Full end-to-end test: create simulation workflow in `tests/e2e/create_workflow_test.go`
- [ ] T056 [P] Full end-to-end test: delete workflow with confirmation in `tests/e2e/delete_workflow_test.go`
- [ ] T057 [P] Error scenario tests: network timeout, 401 unauthorized, 404 not found
- [ ] T058 [P] Verbose logging tests: verify logs appear for each operation

### Documentation

- [ ] T059 [P] Update `docs/quickstart.md` with create simulation examples
- [ ] T060 [P] Update `docs/quickstart.md` with delete simulation examples
- [ ] T061 [P] Add troubleshooting section for common errors in documentation
- [ ] T062 [P] Create `specs/002-create-delete/contracts/api.md` with endpoint specifications

### Code Quality & Release

- [ ] T063 Code review: ensure all error paths are tested
- [ ] T064 Code review: verify verbose logging works end-to-end
- [ ] T065 Code review: check consistent error message formatting
- [ ] T066 Run full test suite and verify ≥80% code coverage
- [ ] T067 Build binary and verify `nvcli create` and `nvcli delete` commands available
- [ ] T068 Final commit and feature branch ready for merge

---

## Task Dependencies

```
Phase 1: T001-T004 (Independent setup tasks)
    ↓
Phase 2: T005-T009 (Blocking prerequisites for all stories)
    ↓
Phase 3: T010-T024 (User Story 1 - Create)
Phase 4: T025-T031 (User Story 2 - Dry-run) [Can start after T005-T009]
Phase 5: T032-T045 (User Story 3 - Delete Simulation) [Can start after T005-T009]
Phase 6: T046-T054 (User Story 4 - Delete Service) [Can start after T005-T009]
    ↓
Phase 7: T055-T068 (Integration & Release)
```

## Parallelization Strategy

**After Phase 2 is complete**, User Stories 1-4 can be developed in parallel:
- **Developer A**: US1 (Create command) - T010-T024
- **Developer B**: US2 (Dry-run validation) - T025-T031  
- **Developer C**: US3 (Delete Simulation) & US4 (Delete Service) - T032-T054

All work on different files, with shared dependencies already resolved in Phase 2.

## MVP Scope

**Minimum Viable Product** (Recommended for first release):
- Phase 1: Setup ✅
- Phase 2: Foundational ✅
- Phase 3: User Story 1 (Create Simulation) ✅
- Phase 4: User Story 2 (Dry-run validation) ✅

**Optional for Phase 1 Release**: US3, US4 (delete operations) can follow in Phase 2 release

## Progress Tracking

Track completion using git commits:
- After Phase 2: `git commit -am "feat: Add topology loading and parsing foundation"`
- After Phase 3: `git commit -am "feat: Implement nvcli create command (US1)"`
- After Phase 4: `git commit -am "feat: Add dry-run validation (US2)"`
- After Phase 5: `git commit -am "feat: Implement nvcli delete simulation (US3)"`
- After Phase 6: `git commit -am "feat: Implement nvcli delete service (US4)"`
- After Phase 7: `git commit -am "refactor: Polish, docs, and integration"`
