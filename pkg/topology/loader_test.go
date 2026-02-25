package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTopologyFromDirectory_ValidTopology(t *testing.T) {
	dir := "."
	topo, err := LoadTopologyFromDirectory(dir)
	if err != nil {
		t.Fatalf("LoadTopologyFromDirectory failed: %v", err)
	}

	// RawTopology should be successfully loaded
	if topo == nil {
		t.Errorf("Expected topology to not be nil")
	}

	// The topology.json in this directory has format and content fields
	// Check that at least something was loaded
	if len(topo.Content.Nodes) == 0 {
		t.Errorf("Expected topology to have nodes")
	}
}

func TestLoadTopologyFromDirectory_MissingDirectory(t *testing.T) {
	dir := "/nonexistent/directory"
	_, err := LoadTopologyFromDirectory(dir)
	if err == nil {
		t.Fatalf("Expected error for missing directory, got nil")
	}
}

func TestLoadTopologyFromDirectory_MissingTopologyFile(t *testing.T) {
	// Create a temporary directory
	dir, err := os.MkdirTemp("", "test_topo_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(dir)

	_, err = LoadTopologyFromDirectory(dir)
	if err == nil {
		t.Fatalf("Expected error for missing topology.json, got nil")
	}
}

func TestLoadTopologyFromDirectory_InvalidJSON(t *testing.T) {
	// Create a temporary directory with invalid JSON
	dir, err := os.MkdirTemp("", "test_topo_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(dir)

	filePath := filepath.Join(dir, "topology.json")
	invalidContent := `{invalid json`
	if err := os.WriteFile(filePath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = LoadTopologyFromDirectory(dir)
	if err == nil {
		t.Fatalf("Expected error for invalid JSON, got nil")
	}
}

func TestLoadTopologyFromPath_ValidFile(t *testing.T) {
	filePath := "valid_topology.json"
	topo, err := LoadTopologyFromPath(filePath)
	if err != nil {
		t.Fatalf("LoadTopologyFromPath failed: %v", err)
	}

	// RawTopology structure should be populated
	if topo == nil {
		t.Errorf("Expected topology to not be nil")
	}
}

func TestLoadTopologyFromPath_MissingFile(t *testing.T) {
	filePath := "/nonexistent/file.json"
	_, err := LoadTopologyFromPath(filePath)
	if err == nil {
		t.Fatalf("Expected error for missing file, got nil")
	}
}
