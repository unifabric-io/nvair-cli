package simulation

import (
	"fmt"
	"io"
	"strings"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/output"
)

// Resolution describes the selected simulation for a command.
type Resolution struct {
	ID           string
	Name         string
	AutoSelected bool
}

// Resolve finds a simulation by name, or auto-selects it when there is exactly one.
func Resolve(apiClient *api.Client, simulationName string) (Resolution, error) {
	simulations, err := apiClient.GetSimulations()
	if err != nil {
		return Resolution{}, err
	}

	simulationName = strings.TrimSpace(simulationName)
	if simulationName == "" {
		switch len(simulations) {
		case 1:
			return Resolution{
				ID:           simulations[0].ID,
				Name:         simulations[0].Name,
				AutoSelected: true,
			}, nil
		case 0:
			return Resolution{}, output.NewValidationError("--simulation <name> is required (no simulations found)")
		default:
			return Resolution{}, output.NewValidationError(
				fmt.Sprintf("--simulation <name> is required (%d simulations found)", len(simulations)),
			)
		}
	}

	for _, sim := range simulations {
		if sim.Name == simulationName {
			return Resolution{
				ID:   sim.ID,
				Name: sim.Name,
			}, nil
		}
	}

	return Resolution{}, output.NewNotFoundError(fmt.Sprintf("simulation not found: %s", simulationName))
}

// WriteDefaultSelectionNotice reports the auto-selected simulation to the user.
func WriteDefaultSelectionNotice(w io.Writer, simulationName string) error {
	if w == nil || strings.TrimSpace(simulationName) == "" {
		return nil
	}

	_, err := fmt.Fprintf(
		w,
		"Using simulation %q by default. Use -s/--simulation <name> to specify a different simulation.\n",
		simulationName,
	)
	return err
}
