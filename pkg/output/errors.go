package output

import (
	"fmt"

	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

// Error represents a categorized error for user display.
type Error struct {
	Category string // Type of error (Validation, Authentication, etc.)
	Message  string // User-facing message
	Cause    error  // Underlying error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Category, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Category, e.Message)
}

// ValidationError creates a validation error.
func NewValidationError(message string) error {
	return &Error{
		Category: "Validation",
		Message:  message,
	}
}

// AuthError creates an authentication error.
func NewAuthError(message string, cause error) error {
	return &Error{
		Category: "Authentication",
		Message:  message,
		Cause:    cause,
	}
}

// SSHKeyError creates an SSH key error.
func NewSSHKeyError(message string, cause error) error {
	return &Error{
		Category: "SSH",
		Message:  message,
		Cause:    cause,
	}
}

// FileError creates a file I/O error.
func NewFileError(message string, cause error) error {
	return &Error{
		Category: "File",
		Message:  message,
		Cause:    cause,
	}
}

// NetworkError creates a network error.
func NewNetworkError(message string, cause error) error {
	return &Error{
		Category: "Network",
		Message:  message,
		Cause:    cause,
	}
}

// ConfigError creates a configuration error.
func NewConfigError(message string, cause error) error {
	return &Error{
		Category: "Configuration",
		Message:  message,
		Cause:    cause,
	}
}

// PartialError represents a partially successful operation (e.g., login succeeded but key upload failed).
type PartialError struct {
	Message string // User-facing message
	Details string // Additional details
}

// Error implements the error interface.
func (pe *PartialError) Error() string {
	if pe.Details != "" {
		return fmt.Sprintf("%s: %s", pe.Message, pe.Details)
	}
	return pe.Message
}

// NewPartialError creates a partial error.
func NewPartialError(message, details string) error {
	return &PartialError{
		Message: message,
		Details: details,
	}
}

// FormatError formats an error for display to the user.
// Returns a user-friendly error message.
func FormatError(err error) string {
	if err == nil {
		return ""
	}

	switch e := err.(type) {
	case *Error:
		if e.Cause != nil {
			return fmt.Sprintf("✗ %s: %s\n   Details: %v", e.Category, e.Message, e.Cause)
		}
		return fmt.Sprintf("✗ %s: %s", e.Category, e.Message)

	case *PartialError:
		return fmt.Sprintf("✗ %s\n   %s", e.Message, e.Details)

	default:
		return fmt.Sprintf("✗ Error: %v", err)
	}
}

// FormatValidationErrors formats topology validation errors for user display
func FormatValidationErrors(errors []topology.ValidationError) string {
	if len(errors) == 0 {
		return ""
	}

	output := "✗ Topology validation failed:\n"
	for _, err := range errors {
		if err.Field != "" {
			output += fmt.Sprintf("  - %s: %s\n", err.Field, err.Message)
		} else {
			output += fmt.Sprintf("  - %s\n", err.Message)
		}
	}
	return output
}
