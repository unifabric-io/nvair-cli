<!--
Sync Impact Report
- Version change: template -> 1.0.0
- Modified principles:
	- PRINCIPLE_1_NAME -> I. Cobra-First CLI Contracts
	- PRINCIPLE_2_NAME -> II. Clear Errors and Exit Codes
	- PRINCIPLE_3_NAME -> III. Structured and Stable Output
	- PRINCIPLE_4_NAME -> IV. Behavior Changes Require Tests
	- PRINCIPLE_5_NAME -> V. Backward Compatibility by Default
- Added sections:
	- Additional Constraints
	- Development Workflow & Quality Gates
- Removed sections:
	- None
- Templates requiring updates:
	- ✅ updated: .specify/templates/plan-template.md
	- ✅ updated: .specify/templates/tasks-template.md
	- ✅ reviewed, no changes required: .specify/templates/spec-template.md
	- ✅ reviewed, no changes required: README.md
	- ✅ reviewed, no changes required: docs/quickstart.md
	- ✅ not applicable (directory missing): .specify/templates/commands/*.md
- Follow-up TODOs:
	- None
-->

# nvair-cli Constitution

## Core Principles

### I. Cobra-First CLI Contracts
All user-facing behavior MUST be exposed through the existing Cobra command tree and
documented command/flag patterns. New commands and flags MUST have clear help text and
predictable defaults. Rationale: users rely on stable CLI ergonomics for automation.

### II. Clear Errors and Exit Codes
Validation and runtime failures MUST return actionable error messages and a non-zero exit
code. Success paths MUST return zero. Errors MUST go to stderr; machine-readable output
MUST stay on stdout. Rationale: scripting and CI depend on reliable process semantics.

### III. Structured and Stable Output
Commands that return resource data SHOULD support structured output (`json` and `yaml`)
when applicable. Human-readable default output MUST remain concise and stable unless a
feature spec explicitly approves a change. Rationale: the CLI serves both humans and tools.

### IV. Behavior Changes Require Tests
Any code change that alters observable behavior MUST include or update tests covering the
new behavior and relevant failure paths. Unit tests are required for logic changes; e2e or
integration tests SHOULD be added when command flows or API contracts are affected.
Rationale: test coverage protects release confidence and prevents regressions.

### V. Backward Compatibility by Default
Existing default behavior, command names, and flag semantics MUST remain backward
compatible unless an approved feature spec declares and justifies a breaking change.
Breaking changes MUST include migration notes. Rationale: users depend on CLI stability.

## Additional Constraints

- Language and runtime: Go CLI using Cobra in `cmd/` and `pkg/commands/`.
- Error handling: no silent failures; returned errors must preserve context.
- Output formats: `json`/`yaml` support applies where commands emit structured resources.
- Logging: verbose/debug details belong in logs/stderr, not mixed into structured stdout.

## Development Workflow & Quality Gates

1. Define behavior in the feature spec, including expected output and failure modes.
2. Implement with minimal scope inside existing command architecture.
3. Add/update tests for changed behavior before merge.
4. Verify command semantics manually for touched commands:
	 - success returns exit code 0
	 - validation/runtime failures return non-zero
	 - structured output remains valid for `json`/`yaml` paths where applicable
5. Update docs when command UX, flags, output, or compatibility expectations change.

## Governance

This constitution is authoritative for planning, specs, and tasks in this repository.

- Amendment process: submit changes in a PR with rationale, impacted principles, and
	required template/doc updates.
- Versioning policy: use semantic versioning for this constitution.
	- MAJOR: remove or redefine principles/governance in a backward-incompatible way.
	- MINOR: add a principle/section or materially expand policy requirements.
	- PATCH: clarifications, wording improvements, and non-semantic edits.
- Compliance review: each implementation plan and PR review MUST include a constitution
	check against these principles and document any justified exceptions.

**Version**: 1.0.0 | **Ratified**: 2026-03-09 | **Last Amended**: 2026-03-09
