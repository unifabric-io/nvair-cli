package delete

import (
	"strings"
	"testing"
)

func TestValidateArgs_InvalidResourceType(t *testing.T) {
	err := ValidateArgs([]string{"invalid", "name"})
	if err == nil {
		t.Fatalf("expected error for invalid resource type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArgs_ServiceNotSupported(t *testing.T) {
	err := ValidateArgs([]string{"service", "name"})
	if err == nil {
		t.Fatalf("expected error for unsupported service resource type, got nil")
	}
	if !strings.Contains(err.Error(), "Must be 'simulation'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArgs_MissingArgs(t *testing.T) {
	err := ValidateArgs([]string{})
	if err == nil {
		t.Fatalf("expected error for missing arguments, got nil")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg(s), received 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_MissingMappedFields(t *testing.T) {
	dc := NewCommand()
	err := dc.Execute()
	if err == nil {
		t.Fatalf("expected error when required fields are not mapped")
	}
	if !strings.Contains(err.Error(), "usage: nvair delete") {
		t.Fatalf("unexpected error: %v", err)
	}
}
