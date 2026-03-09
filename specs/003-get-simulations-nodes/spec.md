# Feature Specification: Get Simulations And Nodes

**Feature Branch**: `003-get-simulations-nodes`  
**Created**: 2026-03-09  
**Status**: Draft  
**Input**: User description: "Add new feature `nvair get simulations/simulation` and `nvair get nodes/node`, support `-o yaml` and `-o json`."

## Clarifications

### Session 2026-03-09

- Q: How should `nvair get nodes` resolve simulation scope? → A: `nvair get nodes` MUST require `--simulation <name>`.
- Q: What should structured output include? → A: Output MUST return only the `results` list.
- Q: What should happen when `--simulation <name>` does not match any simulation? → A: Return clear `simulation not found` error and exit non-zero.

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.
  
  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - List Simulations With Structured Output (Priority: P1)

As an operator, I can run `nvair get simulations` (or `nvair get simulation`) and request JSON or YAML output so I can inspect and automate against simulation data.

**Why this priority**: Simulation discovery is a primary read operation and is often the first step in operational workflows and automation scripts.

**Independent Test**: Can be fully tested by running the simulations command with `-o json` and `-o yaml` and verifying valid, complete structured output is returned.

**Acceptance Scenarios**:

1. **Given** an authenticated user with one or more simulations, **When** they run `nvair get simulations -o json`, **Then** the command returns valid JSON representing the simulation list.
2. **Given** an authenticated user with one or more simulations, **When** they run `nvair get simulation -o yaml`, **Then** the command returns valid YAML representing the same simulation list data.
3. **Given** an authenticated user with no simulations, **When** they run either simulations command variant with structured output, **Then** the command succeeds and returns an empty collection in the selected format.

---

### User Story 2 - List Nodes With Structured Output (Priority: P2)

As an operator, I can run `nvair get nodes` (or `nvair get node`) and request JSON or YAML output so I can inspect node state and integrate with downstream tools.

**Why this priority**: Node visibility is a core operational need, but depends on simulation context already established by User Story 1.

**Independent Test**: Can be fully tested by running nodes command variants with both structured output options and validating the response format and content.

**Acceptance Scenarios**:

1. **Given** an authenticated user with available node data for simulation `lab-a`, **When** they run `nvair get nodes --simulation lab-a -o json`, **Then** the command returns valid JSON for that simulation's node list.
2. **Given** an authenticated user with available node data for simulation `lab-a`, **When** they run `nvair get node --simulation lab-a -o yaml`, **Then** the command returns valid YAML for that simulation's node list.
3. **Given** an authenticated user and no simulation matching `--simulation does-not-exist`, **When** they run `nvair get nodes --simulation does-not-exist`, **Then** the command returns a clear `simulation not found` error and exits with a non-zero status.

---

### User Story 3 - Use Singular And Plural Aliases Interchangeably (Priority: P3)

As an operator, I can use either singular or plural resource names for simulations and nodes so command usage is flexible and intuitive.

**Why this priority**: Alias support improves usability and reduces command entry friction, but does not block baseline data access.

**Independent Test**: Can be independently tested by comparing output and exit status for each singular/plural pair under the same inputs and format flag.

**Acceptance Scenarios**:

1. **Given** the same user context and arguments, **When** the user runs `nvair get simulations` and `nvair get simulation`, **Then** both commands produce equivalent results and exit behavior.
2. **Given** the same user context and arguments, **When** the user runs `nvair get nodes` and `nvair get node`, **Then** both commands produce equivalent results and exit behavior.

---

### Edge Cases

- User provides unsupported output format (for example, `-o xml`).
- User requests structured output when no resources are available.
- User runs nodes command without `--simulation <name>`.
- User provides `--simulation <name>` that does not exist.
- API returns partial fields or null values for optional resource attributes.
- Singular and plural aliases are invoked with identical flags but previously diverged behavior.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support both `nvair get simulations` and `nvair get simulation` command forms for retrieving simulations.
- **FR-002**: System MUST support both `nvair get nodes` and `nvair get node` command forms for retrieving nodes, and both forms MUST require `--simulation <name>`.
- **FR-003**: System MUST accept `-o json` for simulations and nodes command forms and return syntactically valid JSON.
- **FR-004**: System MUST accept `-o yaml` for simulations and nodes command forms and return syntactically valid YAML.
- **FR-005**: System MUST return equivalent data content for singular and plural aliases of the same resource when called with identical inputs.
- **FR-006**: System MUST preserve current default output behavior when `-o` is not provided.
- **FR-007**: System MUST return a clear user-facing validation error when an unsupported output format is requested.
- **FR-008**: System MUST return a clear user-facing validation error when `--simulation <name>` is not provided for nodes commands.
- **FR-009**: System MUST maintain existing authentication and authorization behavior for all new command aliases and output options.
- **FR-010**: For `-o json` and `-o yaml`, the command output MUST contain only the resource `results` collection and MUST NOT include pagination metadata fields such as `count`, `next`, or `previous`.
- **FR-011**: If `--simulation <name>` does not match an existing simulation, system MUST return a clear `simulation not found` error and MUST exit with a non-zero status code.

### Key Entities *(include if feature involves data)*

- **Simulation Summary**: A simulation record returned by the get command, including identifier, display name, and lifecycle state.
- **Node Summary**: A node record returned by the get command, including identifier, node name, operational state, and associated simulation reference.
- **Output Format Option**: User-selected representation (`json` or `yaml`) that controls how command results are rendered.

### Assumptions

- Singular and plural forms are command aliases and do not represent different scopes or filtering behavior.
- Existing command defaults and authentication flows remain unchanged unless explicitly requested.
- Nodes retrieval is explicitly scoped by `--simulation <name>`.
- Structured output for `-o json` and `-o yaml` includes only the resource list payload (`results`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of acceptance scenarios for `simulations/simulation` and `nodes/node` pass for both `-o json` and `-o yaml` outputs.
- **SC-002**: In validation testing, at least 95% of users can retrieve and parse command output in their selected structured format on the first attempt.
- **SC-003**: Invalid output format requests return clear validation feedback in 100% of tested cases without ambiguous or silent failures.
- **SC-004**: Existing workflows that omit `-o` continue to produce expected default output in 100% of regression test cases.
