# Implementation Plan: Create and Delete Simulation Commands

**Feature**: Create and Delete Simulation Commands  
**Branch**: `002-create-delete`  
**Date**: February 25, 2026  
**Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/specs/002-create-delete/spec.md`

## Executive Summary

The Create and Delete feature adds simulation lifecycle management to nvair:

**Create Command** (`nvair create -d <directory>$ `):
- Loads and validates `topology.json` and configuration files from directory
- Validates topology structure before API submission
- Supports `--dry-run` for validation-only mode
- Submits topology to NVIDIA Air API and returns simulation ID
- Displays clear success/error messages with verbose logging support

**Delete Command** (`nvair delete simulation|service <name>`):
- Deletes simulations or services with user confirmation
- Validates authentication before deletion
- Handles API errors gracefully
- Provides clear feedback on success/failure

**Delivery Timeline**: Phase 0 (research) → Phase 1 (design + implementation contracts) → Phase 2 (implementation)  
**Performance Target**: Create simulation completes in <10 seconds; delete in <5 seconds  
**Testing Strategy**: Unit tests for validation logic, integration tests for API interactions, end-to-end tests with mocked API

---

## Technical Context

**Language/Version**: Go 1.22+  
**Primary Dependencies**: go-yaml/yaml (topology parsing), resty or net/http (already used in API client)  
**Storage**: Topology files in user-provided directory (read-only), simulation metadata from API responses  
**Testing**: `go test` with mocked API responses, golden files for topology validation examples  
**Target Platform**: Linux, macOS, Windows (CLI)  
**Project Type**: Single CLI application (extend existing nvair)  
**Performance Goals**: 
- Create simulation: <10 seconds (including API roundtrip)
- Dry-run validation: <5 seconds
- Delete operation: <5 seconds
**Constraints**: 
- Requires network access to NVIDIA Air API
- Requires user authentication (bearer token must be valid)
- Topology validation rules delegated to API
**Scale/Scope**: Single CLI tool with 2-3 new commands and ~1000 lines of code

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

✅ **All Gates Passed**:
- Clear, well-defined user workflows (create from directory, delete with confirmation)
- Straightforward error handling (network errors, validation errors, auth failures)
- Standard patterns: File I/O + YAML parsing + HTTP client + confirmation prompts
- No architectural complexity (adapts existing pattern from login command)
- Feature scope is well-bounded (2 commands, clear responsibilities)
- No violations of project constitution principles

## Project Structure

### Documentation (this feature)

```text
specs/002-create-delete/
├── plan.md              # This file (implementation plan)
├── spec.md              # Feature specification (user stories & acceptance criteria)
├── research.md          # Phase 0: Research findings and decisions
├── data-model.md        # Phase 1: Entity definitions and validation rules
├── contracts/           # Phase 1: API contract definitions
│   └── api.md           # Create/delete API endpoints
├── quickstart.md        # Phase 1: Developer quick start guide
└── checklists/
    └── requirements.md  # Quality checklist
```

### Source Code (repository root)

```text
pkg/
├── commands/
│   ├── create.go        # Create simulation command
│   ├── delete.go        # Delete command (subcommands: simulation, service)
│   └── root.go          # Extended with "create" and "delete" cases
├── api/
│   └── client.go        # Extended with create/delete endpoints
├── topology/            # NEW - Topology validation
│   ├── loader.go        # Load and parse topology files
│   ├── validator.go     # Validate topology structure
│   └── types.go         # Topology data structures
└── output/
    └── errors.go        # Extended error formatting for new errors

examples/
└── simple/              # Existing topology for testing
    └── topology.json

tests/
├── unit/
│   ├── topology_test.go
│   ├── create_test.go
│   └── delete_test.go
├── integration/
│   ├── create_integration_test.go
│   └── delete_integration_test.go
└── fixtures/
    ├── valid_topology.json
    ├── invalid_topology.json
    └── api_responses.json
```

---

## Phase 0: Research & Design

### Objectives
- Validate topology file format and structure
- Design validation rules and error messages
- Understand NVIDIA Air API contracts for create/delete
- Plan confirmation mechanism for destructive operations
- Design data models for topology and responses

### Deliverables

#### 1. **Research Document** → [research.md](research.md)
Topics to investigate:
- Topology.json schema and required fields
- NVIDIA Air API contracts:
  - `POST /v1/simulations` (create simulation)
  - `DELETE /v1/simulations/{id}` (delete simulation)
  - `DELETE /v1/services/{id}` (delete service)
- Existing topology examples in `examples/simple/`
- YAML/JSON parsing libraries available in Go
- File validation patterns and best practices
- User confirmation mechanisms in CLI (stdin handling)
- Error handling for partial failures

#### 2. **Data Model** → [data-model.md](data-model.md)
Define structures:
```
Topology (loaded from topology.json):
- Version (string)
- Name (string)
- Nodes ([]Node)
- Links ([]Link)
- Services ([]Service)

CreateRequest:
- TopologyPath (string, from -d flag)
- DryRun (bool)

CreateResponse:
- SimulationId (string)
- SimulationName (string)
- Status (string)

DeleteRequest:
- ResourceType (enum: simulation, service)
- ResourceName (string)

DeleteResponse:
- Success (bool)
- Message (string)

ValidationError:
- Field (string)
- Message (string)
```

#### 3. **API Contracts** → [contracts/api.md](contracts/api.md)
Document:
- `POST /v1/simulations` request/response
- `POST /v1/simulations/validate` (dry-run endpoint, if exists)
- `DELETE /v1/simulations/{id}` request/response
- `DELETE /v1/services/{id}` request/response
- Error responses (400, 401, 404, 5xx)
- Rate limiting and timeout expectations

#### 4. **Quick Start Guide** → [quickstart.md](quickstart.md)
Include:
- Creating a simulation from examples/simple/
- Dry-run validation workflow
- Deleting simulations
- Troubleshooting common errors
- Verbose output examples

---

## Phase 1: Implementation Planning

### Module Breakdown

#### Module 1: Topology Loading & Validation
**Files**: `pkg/topology/loader.go`, `pkg/topology/validator.go`, `pkg/topology/types.go`

**Responsibilities**:
- Load YAML/JSON topology files from directory
- Parse topology structure into Go types
- Validate required fields and structure
- Provide detailed validation errors
- Handle missing or malformed files

**Data Structures**:
```go
type Topology struct {
    Version  string
    Name     string
    Description string
    Nodes    []Node
    Links    []Link
    Services []Service
}

type ValidationError struct {
    Field   string
    Message string
    Path    string // file path if applicable
}
```

**Validation Rules**:
- topology.json must exist in directory
- Name field is required and non-empty
- Nodes array is required and non-empty
- Each node must have a name
- Links must reference valid nodes

**Dependencies**: 
- `gopkg.in/yaml.v3` (YAML parsing)
- Go standard library (os, path/filepath, encoding/json)

**Tests**:
- Load valid topology → parsed correctly
- Missing topology.json → specific error
- Invalid YAML → parsing error with location
- Validation: missing name → error with field path
- Validation: empty nodes → error with count
- Multiple validation errors → all returned

---

#### Module 2: Create Command
**Files**: `pkg/commands/create.go`

**Responsibilities**:
- Parse `-d <directory>` and `--dry-run` flags
- Call topology loader to validate files
- Call API to create simulation (unless dry-run)
- Format and display response
- Handle all error scenarios

**Workflow**:
```
1. Parse -d (directory) and --dry-run flags
2. Validate directory exists
3. Load topology from directory
   ├─ Success → proceed to step 4
   └─ Validation error → display all errors and exit
4. If --dry-run:
   ├─ Display validation summary and exit
   └─ If not dry-run: proceed to step 5
5. Check authentication (bearer token)
6. Call api.CreateSimulation(topology)
   ├─ Success → display simulation ID and success message
   ├─ 400 Bad Request → display API validation errors
   ├─ 401 Unauthorized → display auth error
   └─ 5xx → display network error
```

**User Flags**:
- `-d, --directory <path>` (required)
- `--dry-run` (optional, validation only)

**Output Messages**:
- Success: "✓ Simulation created successfully. ID: <id>, Name: <name>"
- Dry-run success: "✓ Topology validation passed. Ready to create."
- Validation errors: List each error with field and message
- Network error: "✗ Failed to create simulation: <error message>"
- Auth error: "✗ Not authenticated. Please run 'nvair login' first."

**Tests**:
- Valid topology, dry-run → validation passes, no API call
- Valid topology, create → simulation created, ID returned
- Invalid topology, dry-run → validation errors displayed
- Invalid topology, create → validation errors displayed before API call
- Directory not found → error message
- API returns 400 → error details displayed
- API returns 401 → auth error displayed
- Verbose mode → detailed logs of validation and API calls

---

#### Module 3: Delete Command
**Files**: `pkg/commands/delete.go`

**Responsibilities**:
- Parse `delete simulation|service <name>` subcommands
- Request user confirmation
- Call appropriate API delete endpoint
- Display success/error messages

**Subcommands**:
1. `nvair delete simulation <name>` → DELETE /v1/simulations/{id}
2. `nvair delete service <name>` → DELETE /v1/services/{id}

**Workflow**:
```
1. Parse resource type and resource name
2. Validate inputs (name not empty)
3. Request confirmation: "Delete <type> '<name>'? (yes/no)"
4. If user confirms:
   ├─ Call api.DeleteResource(type, name)
   ├─ Success → display "Deleted" message
   ├─ 404 Not Found → display "Not found" error
   └─ Other errors → display error details
5. If user cancels:
   └─ Display "Operation cancelled"
```

**User Confirmation**:
- Prompt: "Delete <resource-type> '<name>'? (yes/no): "
- Accept "yes", "y", "Yes", "YES"
- Cancel on "no", "n", "No", "NO" or EOF
- Invalid input: re-prompt

**Output Messages**:
- Confirmation prompt (with resource details)
- Success: "✓ <resource-type> '<name>' deleted successfully."
- Not found: "✗ <resource-type> '<name>' not found."
- Network error: "✗ Failed to delete: <error message>"
- Cancelled: "Operation cancelled."
- Auth error: "✗ Not authenticated. Please run 'nvair login' first."

**Tests**:
- Delete simulation with confirmation "yes" → deleted
- Delete simulation with confirmation "no" → cancelled
- Delete service not found → error displayed
- Delete without confirmation → prompt shown
- API returns 5xx → error displayed
- Verbose mode → detailed logs

---

#### Module 4: API Client Extensions
**Files**: `pkg/api/client.go` (extended)

**New Methods**:
```go
// CreateSimulation(topology *Topology) → SimulationResponse, error
// DeleteSimulation(name string) → error
// DeleteService(name string) → error
```

**Responsibilities**:
- HTTP POST to create simulation
- HTTP DELETE to remove simulations/services
- Handle API error responses
- Log requests/responses in verbose mode
- Support retry logic for transient failures (reuse existing)

**Error Handling**:
- 400 → validation error, don't retry
- 401 → auth error, don't retry
- 404 → not found error, don't retry
- 5xx → transient, retry with backoff
- Network timeout → transient, retry

**Tests**:
- Successful creation → response parsed correctly
- Successful deletion → status 204/200 handled
- API returns 400 → validation error returned
- API returns 5xx → retry succeeds on 3rd attempt
- Network timeout → retry logic works

---

#### Module 5: Integration & Error Handling
**Files**: `pkg/commands/root.go` (extended), `pkg/output/errors.go` (extended)

**Responsibilities**:
- Register "create" and "delete" commands in root router
- Format topology validation errors
- Format API errors with user-friendly messages
- Support verbose logging across all new code

**Error Formatting**:
```
Topology Validation Error:
  ✗ Topology validation failed:
    - name: name field is required
    - nodes: must have at least 1 node

API Error:
  ✗ Failed to create simulation:
    Server responded with status 400: Invalid topology structure

File Not Found Error:
  ✗ Directory not found: /path/to/topology
```

**Verbose Logging**:
- Topology file paths being checked
- Validation steps and results
- API endpoint URLs and timeouts
- Request body (with sensitive data masked)
- Response codes and body (sample)

---

### Implementation Sequence

**Critical Path** (must complete in order):
1. Topology data types and loader (`pkg/topology/types.go`, `loader.go`)
2. Topology validator (`pkg/topology/validator.go`)
3. API client extensions (`pkg/api/client.go` - create/delete methods)
4. Create command (`pkg/commands/create.go`)
5. Delete command (`pkg/commands/delete.go`)
6. Root command integration (`pkg/commands/root.go`)
7. Error formatting updates (`pkg/output/errors.go`)

**Parallelizable**:
- Test fixtures and mock API responses (can start immediately)
- Documentation updates to quickstart.md
- Example topologies

---

## Phase 2: Testing & Delivery

### Testing Strategy

**Unit Tests**:
- Topology parsing: valid/invalid YAML, missing fields, malformed JSON
- Validation: all validation rules, multiple errors, edge cases
- Confirmation parsing: "yes", "no", invalid input, case variations
- Error formatting: field names, messages, multi-error display

**Integration Tests**:
- Create with mocked API: successful response handling
- Create dry-run: no API call made
- Delete with mocked API: confirmation flow, success/error responses
- Authentication validation: error when bearer token missing/expired

**End-to-End Tests**:
- Full create workflow with test topology
- Full delete workflow with confirmation
- Verbose output verification

**Mocking Strategy**:
- Mock net/http.RoundTripper for API responses
- Golden files for typical API responses
- Example topologies in tests/fixtures/

### Coverage Goals
- Minimum 80% code coverage for new modules
- All error paths tested
- All user workflows tested (happy path + error cases)

### Delivery Checklist
- [ ] All tests passing
- [ ] Verbose logs working end-to-end
- [ ] Documentation updated (quickstart.md, contracts/api.md)
- [ ] Code review ready
- [ ] Example workflows in docs
- [ ] Commit with feature branch and tags
