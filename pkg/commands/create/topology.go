package create

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

type loadedTopology struct {
	parsed  *topology.RawTopology
	payload []byte
}

func loadTopology(directory string) (*loadedTopology, error) {
	logging.Verbose("Loading topology from directory: %s", directory)
	topo, err := topology.LoadTopologyFromDirectory(directory)
	if err != nil {
		logging.Verbose("Failed to load topology: %v", err)
		return nil, fmt.Errorf("failed to load topology: %w", err)
	}
	logging.Verbose("Topology loaded successfully: %s", topo.Name)

	topologyPath := filepath.Join(directory, "topology.json")
	payload, err := os.ReadFile(topologyPath)
	if err != nil {
		logging.Verbose("Failed to read topology.json: %v", err)
		return nil, fmt.Errorf("failed to read topology.json: %w", err)
	}

	logging.Verbose("Validating topology structure")
	result := topology.ValidateTopology(topo)
	if !result.Valid {
		logging.Verbose("Topology validation failed with %d errors", len(result.Errors))
		fmt.Fprintf(os.Stderr, "%s", topology.FormatValidationErrors(result.Errors))
		return nil, fmt.Errorf("topology validation failed")
	}
	logging.Verbose("Topology validation passed")

	return &loadedTopology{
		parsed:  topo,
		payload: payload,
	}, nil
}
