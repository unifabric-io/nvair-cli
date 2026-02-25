package delete

import "testing"

func TestDeleteCommand_InvalidResourceType(t *testing.T) {
	dc := NewCommand()

	if err := dc.Execute([]string{"invalid", "name"}); err == nil {
		t.Fatalf("Expected error for invalid resource type, got nil")
	}
}

func TestDeleteCommand_MissingArgs(t *testing.T) {
	dc := NewCommand()

	if err := dc.Execute([]string{}); err == nil {
		t.Fatalf("Expected error for missing arguments, got nil")
	}
}
