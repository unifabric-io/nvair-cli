package topology

import (
	"testing"
)

func TestValidateTopology_ValidTopology(t *testing.T) {
	topo := &RawTopology{
		Title: "test-topology",
		Content: RawTopologyContent{
			Nodes: map[string]interface{}{
				"node-1": map[string]interface{}{
					"name": "node-1",
					"type": "compute",
				},
				"node-2": map[string]interface{}{
					"name": "node-2",
					"type": "compute",
				},
			},
		},
	}

	result := ValidateTopology(topo)
	if !result.Valid {
		t.Errorf("Expected valid topology, got errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(result.Errors))
	}
}

func TestValidateTopology_MissingTitle(t *testing.T) {
	topo := &RawTopology{
		Title: "",
		Content: RawTopologyContent{
			Nodes: map[string]interface{}{
				"node-1": map[string]interface{}{
					"name": "node-1",
				},
			},
		},
	}

	result := ValidateTopology(topo)
	if result.Valid {
		t.Errorf("Expected invalid topology for missing title")
	}

	hasTitleError := false
	for _, err := range result.Errors {
		if err.Field == "title" {
			hasTitleError = true
			break
		}
	}
	if !hasTitleError {
		t.Errorf("Expected error for 'title' field")
	}
}

func TestValidateTopology_EmptyNodes(t *testing.T) {
	topo := &RawTopology{
		Title: "test-topology",
		Content: RawTopologyContent{
			Nodes: map[string]interface{}{},
		},
	}

	result := ValidateTopology(topo)
	if result.Valid {
		t.Errorf("Expected invalid topology for empty nodes")
	}

	hasNodeError := false
	for _, err := range result.Errors {
		if err.Field == "content.nodes" {
			hasNodeError = true
			break
		}
	}
	if !hasNodeError {
		t.Errorf("Expected error for 'content.nodes' field")
	}
}

func TestValidateTopology_MultipleErrors(t *testing.T) {
	topo := &RawTopology{
		Title: "",
		Content: RawTopologyContent{
			Nodes: map[string]interface{}{},
		},
	}

	result := ValidateTopology(topo)
	if result.Valid {
		t.Errorf("Expected invalid topology with multiple errors")
	}

	if len(result.Errors) < 2 {
		t.Errorf("Expected at least 2 errors, got %d", len(result.Errors))
	}
}

func TestValidateTopology_NilTopology(t *testing.T) {
	result := ValidateTopology(nil)
	if result.Valid {
		t.Errorf("Expected invalid topology for nil input")
	}

	if len(result.Errors) == 0 {
		t.Errorf("Expected errors for nil topology")
	}
}

func TestFormatValidationErrors(t *testing.T) {
	errors := []ValidationError{
		{Field: "title", Message: "title is required"},
		{Field: "content.nodes", Message: "nodes must not be empty"},
	}

	formatted := FormatValidationErrors(errors)
	if formatted == "" {
		t.Errorf("Expected formatted error string, got empty")
	}

	if !contains(formatted, "title") {
		t.Errorf("Expected 'title' in formatted output")
	}

	if !contains(formatted, "content.nodes") {
		t.Errorf("Expected 'content.nodes' in formatted output")
	}
}

func contains(str, substr string) bool {
	for i := 0; i < len(str)-len(substr)+1; i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
