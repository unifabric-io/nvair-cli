# Feature Specification: Status Command

**Feature Branch**: `005-status-command`  
**Created**: 2026-04-14  
**Status**: Draft  
**Input**: User description: "Implement the nvair status command: if a user is logged in, display the currently logged-in user and whether the user can connect to nvair; if no user is logged in, display that no user is logged in."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Check Login And Connectivity Status (Priority: P1)

As an operator, I want to run `nvair status` to see which user is currently logged in and whether that user can currently connect to nvair, so that I can quickly decide whether to continue with authenticated CLI operations or log in again.

**Why this priority**: This is the core and only user story. The value of the feature is immediate visibility into the current session state and whether the operator can use nvair right now.

**Independent Test**: Can be fully tested by running `nvair status` in three independent conditions: no local session, a valid session that can access nvair, and a valid session that cannot access nvair.

**Acceptance Scenarios**:

1. **Given** a user with a valid logged-in session and successful access to nvair, **When** the user runs `nvair status`, **Then** the CLI displays the current logged-in user and indicates that the user can connect to nvair.
2. **Given** a user with a valid logged-in session but failed access validation to nvair, **When** the user runs `nvair status`, **Then** the CLI displays the current logged-in user and indicates that the user cannot currently connect to nvair.
3. **Given** no local login session, **When** the user runs `nvair status`, **Then** the CLI displays that no user is logged in.
4. **Given** stored authentication data is incomplete, expired, or otherwise unusable, **When** the user runs `nvair status`, **Then** the CLI treats the session as not logged in and reports that no user is logged in.

---

### Edge Cases

- A local config file exists but required session fields are missing or empty.
- Stored credentials exist but can no longer be used to validate access to nvair.
- nvair is temporarily unreachable even though a user is logged in locally.
- Access validation fails because the current user is no longer authorized to connect to nvair.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide an `nvair status` command that reports the current login state and nvair connectivity state without requiring additional positional arguments.
- **FR-002**: When a usable logged-in session exists, system MUST display the identity of the current logged-in user.
- **FR-003**: When a usable logged-in session exists, system MUST verify and report whether that user can currently connect to nvair.
- **FR-004**: When no usable logged-in session exists, system MUST display that no user is logged in.
- **FR-005**: When no usable logged-in session exists, system MUST NOT display a stale username or a positive connectivity result.
- **FR-006**: System MUST clearly distinguish these user-visible states: logged in and can connect, logged in and cannot connect, and not logged in.
- **FR-007**: If connectivity verification fails for a logged-in user, system MUST present a clear user-facing indication that the user cannot currently connect to nvair.
- **FR-008**: Stored authentication data that is incomplete, expired beyond recovery, or otherwise unusable MUST be treated as not logged in for status reporting.
- **FR-009**: The command MUST preserve the currently active account context and MUST NOT switch the active user as part of status reporting.
- **FR-010**: The command output MUST be human-readable and understandable without requiring additional flags or follow-up commands.

### Key Entities

- **Local Session**: Authentication information stored on the user's machine that represents the current nvair CLI account context.
- **Current User**: The user identity associated with the active local session and displayed by `nvair status`.
- **Connection Status**: The current result of checking whether the active user context can access nvair at the time the command is run.

## Assumptions

- A user is considered logged in only if the CLI has usable local authentication data at the time `nvair status` is executed.
- If the CLI can identify a current user but access validation fails, the command should still display that user together with a not-connectable result.
- `nvair status` is an informational command and does not create a new login session.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In acceptance testing, 100% of the three core states (logged in and can connect, logged in and cannot connect, not logged in) produce distinct and unambiguous output from a single `nvair status` invocation.
- **SC-002**: Under normal service availability, logged-in users can determine both the active user identity and current connection availability within 3 seconds of running `nvair status`.
- **SC-003**: In 100% of tested no-session or unusable-session cases, users are told that no user is logged in without any stale identity being displayed.
- **SC-004**: In usability validation, at least 95% of operators can decide whether to continue with authenticated nvair commands or run `nvair login` after reading one `nvair status` result.
