# Implementation Plan: Get Simulations And Nodes

**Branch**: `003-get-simulations-nodes` | **Date**: 2026-03-09 | **Spec**: `/specs/003-get-simulations-nodes/spec.md`
**Input**: Feature specification from `/specs/003-get-simulations-nodes/spec.md`

## Summary

Add read-only `get` resource commands for simulations and nodes with singular/plural aliases:
- `nvair get simulations` and `nvair get simulation`
- `nvair get nodes` and `nvair get node`

Design constraints from clarified spec:
- Nodes queries MUST require `--simulation <name>`
- Structured output MUST support `-o json` and `-o yaml`
- Structured output MUST include only the `results` collection
- Missing simulation name match MUST return `simulation not found` with non-zero exit

Implementation will follow existing command architecture (Cobra command wrappers + `pkg/api/client.go` calls + centralized error formatting) and add focused unit tests around validation, alias parity, output format, and error behavior.

## Technical Context

**Language/Version**: Go 1.25.6  
**Primary Dependencies**: `github.com/spf13/cobra`, `encoding/json`, `gopkg.in/yaml.v3`, existing `pkg/api` client  
**Storage**: N/A (read-only API retrieval + in-memory transformation)  
**Testing**: `make test-unit`, `make test-e2e` for e2e test.
**Target Platform**: Cross-platform CLI (Linux/macOS/Windows)  
**Project Type**: Single Go CLI project  
**Performance Goals**: No specific performance requirements.
**Constraints**: Must preserve existing default output when `-o` absent; must enforce `--simulation <name>` for node listing; must keep auth/token-refresh behavior unchanged  
**Scale/Scope**: One new command group (`get`) + aliases + output rendering + targeted tests; no backend/API schema changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Pre-Phase 0 Gate Review:
- `.specify/memory/constitution.md` currently contains placeholder tokens and no enforceable principles.
- Result: PASS (no active constitutional constraints to violate).
- Risk note: governance requirements are undefined until constitution is ratified.

Post-Phase 1 Re-check:
- Design artifacts remain within existing CLI architecture and do not introduce conflicting governance assumptions.
- Result: PASS.

## Project Structure

### Documentation (this feature)

```text
specs/003-get-simulations-nodes/
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
└── nvair/
    └── main.go

pkg/
├── commands/
│   ├── root.go
│   └── get/
│       ├── command.go
│       └── command_test.go
├── api/
│   ├── client.go
│   └── client_test.go
└── output/
    └── errors.go
```

**Structure Decision**: Keep a single-project CLI structure and add a dedicated `pkg/commands/get/` module consistent with existing `create`, `delete`, `login`, and `logout` command organization.

## Complexity Tracking

No constitution violations identified that require justification.
