package create

import (
	"fmt"
	"os"

	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

func loadTopology(directory string) (*topology.RawTopology, error) {
	logging.Verbose("Loading topology from directory: %s", directory)
	topo, err := topology.LoadTopologyFromDirectory(directory)
	if err != nil {
		logging.Verbose("Failed to load topology: %v", err)
		return nil, fmt.Errorf("failed to load topology: %w", err)
	}
	logging.Verbose("Topology loaded successfully: %s", topo.Title)

	logging.Verbose("Validating topology structure")
	result := topology.ValidateTopology(topo)
	if !result.Valid {
		logging.Verbose("Topology validation failed with %d errors", len(result.Errors))
		fmt.Fprintf(os.Stderr, "%s", topology.FormatValidationErrors(result.Errors))
		return nil, fmt.Errorf("topology validation failed")
	}
	logging.Verbose("Topology validation passed")

	return topo, nil
}
