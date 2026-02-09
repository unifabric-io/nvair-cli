# Implementation Plan: nvair CLI Tool

**Branch**: `001-nvair-cli` | **Date**: January 9, 2026 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-nvair-cli/spec.md`

## Summary

The nvair CLI tool provides command-line interface access to the air.nvidia.com platform for managing network simulations. Users authenticate once via login, storing credentials locally, then execute subsequent commands (create simulations, query nodes, run remote commands, install software) without re-authentication. The implementation handles configuration persistence, API token exchange, SSH remote execution, and table-formatted output for all queries.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.22+  
**Primary Dependencies**: Cobra (CLI framework), Resty (HTTP client) or net/http, golang.org/x/crypto/ssh (SSH), go-pretty/table (table formatting)  
**Storage**: Local JSON configuration file (`$HOME/.config/nvair.unifabric.io/config.json`)  
**Testing**: `go test` with table-driven tests; httptest for HTTP; golden files for CLI output  
**Target Platform**: Linux, macOS, Windows (cross-platform CLI)  
**Project Type**: Single CLI application (Go module)  
**Performance Goals**: Login/command execution <5 seconds under normal network conditions  
**Constraints**: Requires network access to air.nvidia.com API, requires SSH client capability  
**Scale/Scope**: Single-purpose CLI tool, ~15-20 commands, managed simulations up to 100 nodes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

✅ **All Gates Passed** - This is a straightforward CLI tool with:
- Well-defined single responsibility (simulate control interface)
- Clear, non-ambiguous requirements from specification
- No architectural complexity or novel patterns required
- Standard patterns: CLI framework + HTTP client + SSH client + local file storage
- No violations of project constitution principles identified

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── spec.md              # Feature specification (user stories & acceptance criteria)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── api.md           # API contracts
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)

# Additional technical references (implementation-specific):
# - ssh-key-setup.md
# - ssh-password-reset.md
# - installation-commands.md
# - forward-sync.md
# - topology-examples.md
```

### Source Code (repository root)

```text
nvair-cli/
├── cmd/nvair/                 # main package (CLI entry)
│   └── main.go
├── pkg/
│   ├── config/
│   │   ├── loader.go          # Load/save configuration JSON with perms
│   │   └── model.go           # Config structs
│   ├── api/
│   │   ├── client.go          # HTTP client with auth/token handling
│   │   └── endpoints.go       # API wrappers (v2)
│   ├── commands/
│   │   ├── root.go            # Cobra root cmd
│   │   ├── login.go           # nvair login
│   │   ├── simulation.go      # nvair create / get simulation
│   │   ├── node.go            # nvair get node
│   │   ├── exec.go            # nvair exec
│   │   └── install.go         # nvair install *
│   ├── ssh/
│   │   └── remote.go          # SSH client wrapper (x/crypto/ssh)
│   ├── output/
│   │   ├── table.go           # go-pretty/table formatting
│   │   └── errors.go          # error rendering
│   └── utils/
│       ├── validators.go
│       └── constants.go
├── pkg/version/
│   └── version.go             # version info for --version
├── test/                      # e2e/integration helpers (optional)
│   └── e2e_test.go
├── examples/                 # Topology definition examples
│   ├── README.md             # Examples documentation
│   └── simple/               # Simple 2-node topology
│       ├── topology.json     # NVIDIA Air JSON format
│       └── README.md
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

**Structure Decision**: Monolithic Go CLI (Cobra) with domain-separated commands (login/simulation/node/exec/install), clear internal module separation: config, API client, SSH, output and validation. Testing uses co-located `_test.go` unit tests, with integration/E2E tests and fixtures in `test/` directory as needed.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
