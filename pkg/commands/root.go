package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	createcmd "github.com/unifabric-io/nvair-cli/pkg/commands/create"
	deletecmd "github.com/unifabric-io/nvair-cli/pkg/commands/delete"
	getcmd "github.com/unifabric-io/nvair-cli/pkg/commands/get"
	logincmd "github.com/unifabric-io/nvair-cli/pkg/commands/login"
	logoutcmd "github.com/unifabric-io/nvair-cli/pkg/commands/logout"
	"github.com/unifabric-io/nvair-cli/pkg/output"
)

// RootCommand is the main CLI entry point.
type RootCommand struct {
	Verbose bool
}

// NewRootCommand creates a new root command.
func NewRootCommand() *RootCommand {
	return &RootCommand{}
}

// Run executes the CLI with the given arguments.
// Returns 0 on success, non-zero on error.
func (rc *RootCommand) Run(args []string) int {
	cmd := rc.newCommand()
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, output.FormatError(err))
		return 1
	}

	return 0
}

func (rc *RootCommand) newCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "nvair",
		Short:         "NVIDIA Air CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	rootCmd.PersistentFlags().BoolVarP(&rc.Verbose, "verbose", "v", false, "Enable verbose logging for detailed debugging")
	rootCmd.AddCommand(
		rc.newLoginCommand(),
		rc.newLogoutCommand(),
		rc.newCreateCommand(),
		rc.newGetCommand(),
		rc.newDeleteCommand(),
	)

	return rootCmd
}

func (rc *RootCommand) newLoginCommand() *cobra.Command {
	loginCmd := logincmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "login",
		Short:         "Authenticate with NVIDIA Air",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loginCmd.Verbose = rc.Verbose
			return loginCmd.Execute()
		},
	}
	loginCmd.Register(cmd)
	return cmd
}

func (rc *RootCommand) newLogoutCommand() *cobra.Command {
	logoutCmd := logoutcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "logout",
		Short:         "Logout from NVIDIA Air",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logoutCmd.Verbose = rc.Verbose
			return logoutCmd.Execute()
		},
	}
	logoutCmd.Register(cmd)
	return cmd
}

func (rc *RootCommand) newCreateCommand() *cobra.Command {
	createCmd := createcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "create",
		Short:         "Create a simulation from topology",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			createCmd.Verbose = rc.Verbose
			return createCmd.Execute()
		},
	}
	createCmd.Register(cmd)
	return cmd
}

func (rc *RootCommand) newDeleteCommand() *cobra.Command {
	deleteCmd := deletecmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "delete <simulation> <name>",
		Short:         "Delete a simulation",
		SilenceUsage:  true,
		SilenceErrors: true,
		ValidArgs:     []string{"simulation"},
		Args: func(cmd *cobra.Command, args []string) error {
			return deletecmd.ValidateArgs(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			deleteCmd.Verbose = rc.Verbose
			deleteCmd.ResourceType = args[0]
			deleteCmd.ResourceName = args[1]
			return deleteCmd.Execute()
		},
	}
	deleteCmd.Register(cmd)
	return cmd
}

func (rc *RootCommand) newGetCommand() *cobra.Command {
	getCommand := getcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "get",
		Short:         "Get simulations and nodes",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			getCommand.Verbose = rc.Verbose
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	getCommand.Register(cmd)
	return cmd
}
