# Feature Specification: Create and Delete Simulation Commands

**Feature Branch**: `002-create-delete`  
**Created**: February 25, 2026  
**Status**: Draft  
**Input**: User description: "Implement nvcli create and nvcli delete commands"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create Simulation from Topology (Priority: P1)

A developer has a network topology directory with `topology.json` and configuration files. They want to deploy this topology as a simulation on the NVIDIA Air platform using the `nvcli create` command.

**Why this priority**: Creation of simulations is the primary workflow after authentication. Without this, users cannot provision network environments to work with.

**Independent Test**: Can be fully tested by providing a valid topology directory, running the create command, and verifying the API call results and output formatting.

**Acceptance Scenarios**:

1. **Given** a valid topology directory exists with `topology.json`, **When** user runs `nvcli create -d examples/simple/`, **Then** system creates simulation on NVIDIA Air and displays success message with simulation ID
2. **Given** user is authenticated, **When** user runs `nvcli create -d examples/simple/ --dry-run`, **Then** system validates configuration without creating simulation and displays validation results
3. **Given** topology directory is invalid, **When** user runs `nvcli create -d /invalid/path/`, **Then** system displays clear error message about missing or invalid files

---

### User Story 2 - Validate Topology Before Creation (Priority: P1)

A developer wants to validate their topology configuration without creating a simulation, to catch errors early in their workflow.

**Why this priority**: Pre-validation (dry-run) prevents failed deployments and provides immediate feedback on configuration issues, reducing iteration time.

**Independent Test**: Can be fully tested by running the command with `--dry-run` flag against sample topologies and verifying validation logic without API calls.

**Acceptance Scenarios**:

1. **Given** a valid topology directory exists, **When** user runs `nvcli create -d examples/simple/ --dry-run`, **Then** system validates all files and displays success without making API calls
2. **Given** an invalid topology (missing required fields), **When** user runs `nvcli create -d invalid-topology/ --dry-run`, **Then** system displays specific validation errors for each problem found

---

### User Story 3 - Delete Simulation (Priority: P2)

A user wants to remove a simulation they no longer need from the NVIDIA Air platform to free up resources and reduce costs.

**Why this priority**: Simulation lifecycle management is important for resource efficiency, but not required for initial deployment scenarios.

**Independent Test**: Can be fully tested by running delete command with Various simulation names and verifying API calls and confirmation messages.

**Acceptance Scenarios**:

1. **Given** a simulation exists on the platform, **When** user runs `nvcli delete simulation my-simulation`, **Then** system requests confirmation and deletes the simulation after user confirms
2. **Given** a simulation exists, **When** user runs `nvcli delete simulation my-simulation` and confirms, **Then** system displays success message and simulation no longer appears in listing
3. **Given** a simulation does not exist, **When** user runs `nvcli delete simulation nonexistent`, **Then** system displays error message indicating simulation not found

---

### User Story 4 - Delete Service Forwarding Rule (Priority: P3)

A user wants to remove a specific service forwarding rule from their simulation without deleting the entire simulation.

**Why this priority**: Fine-grained service lifecycle management is nice-to-have but not critical for core simulation workflows.

**Independent Test**: Can be fully tested by running delete service command and verifying API calls and user confirmations.

**Acceptance Scenarios**:

1. **Given** a service forwarding rule exists, **When** user runs `nvcli delete service my-service`, **Then** system requests confirmation and deletes the service after confirmation
2. **Given** a service does not exist, **When** user runs `nvcli delete service nonexistent`, **Then** system displays error message indicating service not found

---

### Edge Cases

- What happens when topology directory has incomplete files (missing `topology.json`)?
- How does system handle network errors during simulation creation or deletion?
- What happens if user cancels confirmation prompt during deletion?
- How does system handle user interrupting long-running create operation (Ctrl+C)?
- What happens when user supplies invalid simulation or service names (special characters, empty string)?
- How does system handle duplicate simulation names?

## Requirements *(mandatory)*

### Functional Requirements

#### Create Command

- **FR-001**: System MUST accept `nvcli create -d <directory>` to create simulation from topology directory
- **FR-002**: System MUST support `--dry-run` flag to validate topology without creating simulation
- **FR-003**: System MUST load and parse `topology.json` from specified directory
- **FR-004**: System MUST validate that all required topology files exist and are properly formatted
- **FR-005**: System MUST authenticate using stored bearer token from local configuration
- **FR-006**: System MUST submit topology to NVIDIA Air API and receive simulation ID
- **FR-007**: System MUST display clear success message with simulation ID after creation
- **FR-008**: System MUST handle API errors gracefully and display specific error messages to user
- **FR-009**: System MUST support verbose mode to display API request/response details and validation steps
- **FR-010**: System MUST validate topology files without creating simulation when `--dry-run` is used

#### Delete Simulation Command

- **FR-011**: System MUST accept `nvcli delete simulation <name>` to delete a simulation
- **FR-012**: System MUST request user confirmation before deleting simulation (safety mechanism)
- **FR-013**: System MUST authenticate using stored bearer token from local configuration  
- **FR-014**: System MUST submit delete request to NVIDIA Air API with simulation name
- **FR-015**: System MUST display success message after successful deletion
- **FR-016**: System MUST display appropriate error message if simulation is not found
- **FR-017**: System MUST allow user to cancel operation by declining confirmation

#### Delete Service Command

- **FR-018**: System MUST accept `nvcli delete service <name>` to delete a service forwarding rule
- **FR-019**: System MUST request user confirmation before deleting service (safety mechanism)
- **FR-020**: System MUST authenticate using stored bearer token from local configuration
- **FR-021**: System MUST submit delete request to NVIDIA Air API with service name
- **FR-022**: System MUST display success message after successful deletion
- **FR-023**: System MUST display appropriate error message if service is not found
- **FR-024**: System MUST allow user to cancel operation by declining confirmation

#### Cross-cutting

- **FR-025**: All commands MUST validate that user is authenticated before attempting API operations
- **FR-026**: All commands MUST handle network timeouts and display appropriate error messages
- **FR-027**: All commands MUST support verbose logging with timestamps and detailed request/response information
- **FR-028**: All commands MUST use consistent error formatting across all error scenarios

### Key Entities

- **Topology**: A configuration directory containing `topology.json` and supporting network configuration files that define the simulation structure
- **Simulation**: A deployed network environment on NVIDIA Air platform identified by unique ID and name, containing nodes and services
- **Service**: A network endpoint exposed within a Simulation (e.g., via port forwarding or Kubernetes NodePort)
- **Confirmation**: User acknowledgment required before destructive operations (delete) to prevent accidental resource removal

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can create a simulation from a valid topology directory and receive confirmation within 10 seconds of command execution
- **SC-002**: Dry-run validation completes and displays all validation errors within 5 seconds without making API calls
- **SC-003**: Users can delete a simulation successfully after confirming the destructive action, with confirmation message displayed immediately
- **SC-004**: All error messages clearly indicate the reason for failure (e.g., "Simulation not found", "Invalid topology file", "Network timeout")
- **SC-005**: Verbose mode displays detailed logs including API endpoint URLs, request bodies (with sensitive data masked), response codes, and timing information
- **SC-006**: System correctly handles 95% of topology files with proper validation feedback within expected timeframe
- **SC-007**: Users successfully complete simulation creation task on first attempt (after providing correct topology directory) without needing to retry
- **SC-008**: Confirmation prompts prevent accidental deletion - at least 99% of destructive operations require explicit user confirmation

## Assumptions

- **Topology Format**: Topology directories contain a required `topology.json` file; additional YAML/config files may be optional or required based on platform standards (validation rules to be defined by API contract)
- **Authentication State**: User must have successfully run `nvcli login` before using create/delete commands; system will error gracefully if bearer token is missing or expired
- **Confirmation Mechanism**: Confirmation prompts are interactive, expecting user to type "yes" or "no"; suitable for CLI environments but not suitable for automated/scripted contexts without stdin
- **Error Handling**: Network errors trigger immediate failure with clear message; no automatic retry logic (retry logic exists in API client but not for these commands specifically)
- **Naming Constraints**: Simulation and service names follow platform conventions (case-sensitive, no special characters); validation delegated to API which will return errors for invalid names
- **API Endpoint**: Default API endpoint is configured in stored configuration after login; create/delete commands use this endpoint without requiring it as a parameter
