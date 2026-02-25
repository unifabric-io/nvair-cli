package topology

import (
	"fmt"
)

// ValidateTopology validates the topology structure and returns all validation errors
func ValidateTopology(topology *RawTopology) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Errors: make([]ValidationError, 0),
	}

	if topology == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "topology",
			Message: "topology is nil",
		})
		return result
	}

	// Check title field (required)
	if topology.Title == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "title",
			Message: "title field is required and cannot be empty",
		})
	}

	// Check content field
	if topology.Content.Nodes == nil || len(topology.Content.Nodes) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "content.nodes",
			Message: "nodes must have at least one node",
		})
	}

	return result
}

// FormatValidationErrors formats validation errors for display
func FormatValidationErrors(errors []ValidationError) string {
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
