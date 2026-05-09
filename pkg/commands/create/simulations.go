package create

import (
	"fmt"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

func deleteDuplicateSimulations(apiClient *api.Client, topo *topology.RawTopology, deleteIfExists bool) error {
	logging.Verbose("Checking for existing simulations named: %s", topo.Title)
	existingSims, err := apiClient.GetSimulations()
	if err != nil {
		logging.Verbose("Failed to list simulations: %v", err)
		return fmt.Errorf("failed to list simulations: %w", err)
	}

	var duplicates []api.SimulationInfo
	for _, sim := range existingSims {
		if sim.Name == topo.Title {
			duplicates = append(duplicates, sim)
		}
	}

	if len(duplicates) == 0 {
		return nil
	}

	if !deleteIfExists {
		return fmt.Errorf("simulation with name '%s' already exists. Rerun with --delete-if-exists to replace it or choose a different name", topo.Title)
	}

	for _, sim := range duplicates {
		logging.Verbose("Deleting existing simulation with ID: %s", sim.ID)
		if err := apiClient.DeleteSimulationByID(sim.ID); err != nil {
			logging.Verbose("Failed to delete existing simulation %s: %v", sim.ID, err)
			return fmt.Errorf("failed to delete existing simulation '%s': %w", sim.Name, err)
		}
	}

	return nil
}
