# Tasks: Get Simulations And Nodes

**Input**: Design documents from `/specs/003-get-simulations-nodes/`
**Prerequisites**: `plan.md` (required), `spec.md` (required)

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the new `get` command entrypoint and shared command wiring.

- [X] T001 Create `get` command scaffolding and root handler in `pkg/commands/get/command.go`
- [X] T002 [P] Register `get` command in root command tree in `pkg/commands/root.go`
- [X] T003 [P] Add validation error helpers/messages for get command input failures in `pkg/output/errors.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build shared logic required by all `get` resource handlers.

**CRITICAL**: No user story implementation starts before this phase is complete.

- [X] T004 Implement shared config/auth bootstrap for `get` command execution in `pkg/commands/get/command.go`
- [X] T005 Implement output format parsing (`default|json|yaml`) and unsupported-format validation in `pkg/commands/get/command.go`
- [X] T006 Implement shared `results`-only serializers for JSON and YAML in `pkg/commands/get/command.go`
- [X] T007 [P] Align API client list fields used by get command rendering in `pkg/api/client.go`

**Checkpoint**: Foundation ready - user stories can now be implemented.

---

## Phase 3: User Story 1 - List Simulations With Structured Output (Priority: P1) 🎯 MVP

**Goal**: Deliver `nvair get simulations|simulation` with default output and structured `-o json|-o yaml` (`results`-only).

**Independent Test**: Run `nvair get simulations -o json` and `nvair get simulation -o yaml`; verify valid structured output and empty-list behavior when no simulations exist.

- [X] T008 [US1] Implement simulations fetch flow using `Client.GetSimulations()` in `pkg/commands/get/command.go`
- [X] T009 [US1] Implement simulations structured output (`-o json`, `-o yaml`) emitting only `results` payload in `pkg/commands/get/command.go`
- [X] T010 [US1] Implement simulations default output path preserving existing default output behavior in `pkg/commands/get/command.go`
- [X] T011 [US1] Register `simulations` and `simulation` resource commands under `get` in `pkg/commands/get/command.go`

**Checkpoint**: User Story 1 is functional and independently testable.

---

## Phase 4: User Story 2 - List Nodes With Structured Output (Priority: P2)

**Goal**: Deliver `nvair get nodes|node` with mandatory `--simulation <name>`, structured output, and deterministic not-found behavior.

**Independent Test**: Run `nvair get nodes --simulation <name> -o json` and `nvair get node --simulation <name> -o yaml`; verify success. Run with missing/unknown simulation and verify clear errors with non-zero exit.

- [X] T012 [US2] Register `nodes` and `node` resource commands with required `--simulation` flag in `pkg/commands/get/command.go`
- [X] T013 [US2] Implement simulation name-to-ID resolution (exact title match) via `Client.GetSimulations()` in `pkg/commands/get/command.go`
- [X] T014 [US2] Implement nodes fetch flow using resolved simulation ID via `Client.GetNodes(simulationID)` in `pkg/commands/get/command.go`
- [X] T015 [US2] Implement missing `--simulation` validation and `simulation not found` error behavior with non-zero propagation in `pkg/commands/get/command.go`
- [X] T016 [US2] Implement nodes structured output (`-o json`, `-o yaml`) emitting only `results` payload in `pkg/commands/get/command.go`

**Checkpoint**: User Story 2 is functional and independently testable.

---

## Phase 5: User Story 3 - Use Singular And Plural Aliases Interchangeably (Priority: P3)

**Goal**: Ensure singular/plural command aliases are behaviorally equivalent for simulations and nodes.

**Independent Test**: Execute singular/plural pairs with identical flags and compare outputs and exit behavior for equivalence.

- [X] T017 [US3] Route `simulation` and `simulations` aliases to the same simulations execution path in `pkg/commands/get/command.go`
- [X] T018 [US3] Route `node` and `nodes` aliases to the same nodes execution path in `pkg/commands/get/command.go`
- [X] T019 [US3] Align usage/help examples for singular and plural resource aliases in `pkg/commands/get/command.go`

**Checkpoint**: User Story 3 is functional and independently testable.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Finalize docs and end-to-end validation across stories.

- [X] T020 [P] Document new `get` command usage (`simulations|simulation`, `nodes|node`, `-o`, `--simulation`) in `README.md`
- [X] T021 [P] Update CLI usage guidance with success/failure examples in `docs/quickstart.md`
- [X] T022 Validate feature scenarios and finalize runnable examples in `specs/003-get-simulations-nodes/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Setup (Phase 1): No dependencies.
- Foundational (Phase 2): Depends on Setup completion; blocks all user stories.
- User Stories (Phases 3-5): Depend on Foundational completion.
- Polish (Phase 6): Depends on completion of desired user stories.

### User Story Dependencies

- **US1 (P1)**: Starts after Foundational phase; no dependency on other user stories.
- **US2 (P2)**: Starts after Foundational phase; depends on shared output and validation foundations only.
- **US3 (P3)**: Starts after US1 and US2 handlers exist to verify alias equivalence paths.

### Within Each User Story

- Command registration before command execution wiring.
- Data fetch and validation before output rendering.
- Error behavior alignment before documentation updates.

---

## Parallel Opportunities

- **Setup**: `T002` and `T003` can run in parallel after `T001`.
- **Foundational**: `T007` can run in parallel with `T004-T006`.
- **Polish**: `T020` and `T021` can run in parallel before `T022`.

### Parallel Example: User Story 1

```bash
# US1 has primarily same-file implementation tasks in pkg/commands/get/command.go,
# so execute sequentially to avoid merge conflicts:
T008 -> T009 -> T010 -> T011
```

### Parallel Example: User Story 2

```bash
# US2 also centers on shared command handler logic in one file,
# so execute sequentially for correctness:
T012 -> T013 -> T014 -> T015 -> T016
```

### Parallel Example: User Story 3

```bash
# Alias routing and help alignment are closely coupled in one file:
T017 -> T018 -> T019
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup).
2. Complete Phase 2 (Foundational).
3. Complete Phase 3 (US1).
4. Validate US1 independently with `-o json` and `-o yaml`.

### Incremental Delivery

1. Deliver US1 (simulations retrieval and structured output).
2. Deliver US2 (nodes retrieval with required `--simulation`).
3. Deliver US3 (alias equivalence hardening).
4. Complete Polish phase for docs and final quickstart validation.

### Team Strategy

1. One developer implements command logic in `pkg/commands/get/command.go`.
2. One developer handles root/error/documentation updates (`pkg/commands/root.go`, `pkg/output/errors.go`, `README.md`, `docs/quickstart.md`).
3. Integrate and run final scenario validation in feature quickstart.

---

## Notes

- `[P]` tasks are safe parallel work items across different files.
- `[US1]`, `[US2]`, `[US3]` labels map directly to prioritized stories in `spec.md`.
- Structured output must remain `results`-only for both JSON and YAML.
- Nodes commands must enforce `--simulation <name>` and return non-zero on not-found.
