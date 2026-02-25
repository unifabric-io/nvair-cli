package topology

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadTopologyFromDirectory loads and parses the topology.json file from a directory
func LoadTopologyFromDirectory(dirPath string) (*RawTopology, error) {
	// Check if directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory not found: %s", dirPath)
		}
		return nil, fmt.Errorf("failed to access directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dirPath)
	}

	// Construct path to topology.json
	topologyPath := filepath.Join(dirPath, "topology.json")

	// Check if topology.json exists
	if _, err := os.Stat(topologyPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("topology.json not found in directory: %s", dirPath)
		}
		return nil, fmt.Errorf("failed to access topology.json: %w", err)
	}

	// Read the file
	data, err := os.ReadFile(topologyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read topology.json: %w", err)
	}

	return parseTopologyFile(data)
}

// LoadTopologyFromPath loads a topology file from a specific file path
func LoadTopologyFromPath(filePath string) (*RawTopology, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, fmt.Errorf("failed to access file: %w", err)
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return parseTopologyFile(data)
}

// parseTopologyFile parses topology JSON and returns RawTopology
func parseTopologyFile(data []byte) (*RawTopology, error) {
	var topology RawTopology
	if err := json.Unmarshal(data, &topology); err != nil {
		return nil, fmt.Errorf("failed to parse topology: %w", err)
	}

	return &topology, nil
}
