package create

import "github.com/spf13/cobra"

// Command represents the create subcommand for creating simulations.
type Command struct {
	Directory      string
	DryRun         bool
	APIEndpoint    string
	Verbose        bool
	DeleteIfExists bool
}

// NewCommand creates a new Command instance with defaults.
func NewCommand() *Command {
	return &Command{
		APIEndpoint:    "https://air.nvidia.com/api",
		DeleteIfExists: false, // Default to false
	}
}

// Register registers the create command flags.
func (cc *Command) Register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVarP(&cc.Directory, "directory", "d", cc.Directory, "Directory path containing topology.json (required)")
	flags.BoolVar(&cc.DryRun, "dry-run", cc.DryRun, "Validate topology without creating simulation")
	flags.StringVar(&cc.APIEndpoint, "api-endpoint", cc.APIEndpoint, "API endpoint URL")
	flags.BoolVar(&cc.DeleteIfExists, "delete-if-exists", cc.DeleteIfExists, "Delete existing simulation with the same name before creating (default: true)")
}
