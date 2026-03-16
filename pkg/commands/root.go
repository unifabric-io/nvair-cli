package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	addcmd "github.com/unifabric-io/nvair-cli/pkg/commands/add"
	cpcmd "github.com/unifabric-io/nvair-cli/pkg/commands/cp"
	createcmd "github.com/unifabric-io/nvair-cli/pkg/commands/create"
	deletecmd "github.com/unifabric-io/nvair-cli/pkg/commands/delete"
	execcmd "github.com/unifabric-io/nvair-cli/pkg/commands/exec"
	getcmd "github.com/unifabric-io/nvair-cli/pkg/commands/get"
	logincmd "github.com/unifabric-io/nvair-cli/pkg/commands/login"
	logoutcmd "github.com/unifabric-io/nvair-cli/pkg/commands/logout"
	printsshcommandcmd "github.com/unifabric-io/nvair-cli/pkg/commands/printsshcommand"
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
		if message := output.FormatError(err); message != "" {
			fmt.Fprintln(os.Stderr, message)
		}
		if code, ok := output.ExitCodeFromError(err); ok {
			return code
		}
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
		rc.newAddCommand(),
		rc.newCreateCommand(),
		rc.newGetCommand(),
		rc.newPrintSSHCommand(),
		rc.newCopyCommand(),
		rc.newExecCommand(),
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

func (rc *RootCommand) newAddCommand() *cobra.Command {
	addCommand := addcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "add",
		Short:         "Add resources",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			addCommand.Verbose = rc.Verbose
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	addCommand.Register(cmd)
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
		Short:         "Delete resources",
		SilenceUsage:  true,
		SilenceErrors: true,
		ValidArgs:     []string{"simulation"},
		Args: func(cmd *cobra.Command, args []string) error {
			return deletecmd.ValidateArgs(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			deleteCmd.Verbose = rc.Verbose
			deleteCmd.Stderr = cmd.ErrOrStderr()
			deleteCmd.ResourceType = args[0]
			deleteCmd.ResourceName = args[1]
			return deleteCmd.Execute()
		},
	}
	deleteCmd.Register(cmd)

	forwardCmd := &cobra.Command{
		Use:           "forward",
		Aliases:       []string{"forwards"},
		Short:         "Delete a forward service by target",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deleteCmd.Verbose = rc.Verbose
			deleteCmd.Stderr = cmd.ErrOrStderr()
			deleteCmd.ResourceType = "forward"
			deleteCmd.SimulationName, _ = cmd.Flags().GetString("simulation")
			deleteCmd.TargetNode, _ = cmd.Flags().GetString("target-node")
			deleteCmd.TargetPort, _ = cmd.Flags().GetInt("target-port")
			return deleteCmd.Execute()
		},
	}
	forwardCmd.Flags().StringP("simulation", "s", "", "Simulation name (optional when only one simulation exists)")
	forwardCmd.Flags().StringVar(&deleteCmd.TargetNode, "target-node", "", "Target node name")
	forwardCmd.Flags().IntVar(&deleteCmd.TargetPort, "target-port", 0, "Target port on target node")
	cmd.AddCommand(forwardCmd)

	return cmd
}

func (rc *RootCommand) newGetCommand() *cobra.Command {
	getCommand := getcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "get",
		Short:         "Get simulations, nodes, and forwards",
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

func (rc *RootCommand) newPrintSSHCommand() *cobra.Command {
	printSSHCommand := printsshcommandcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "print-ssh-command",
		Aliases:       []string{"print-ssh", "ssh-command"},
		Short:         "Print SSH command for bastion host",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			printSSHCommand.Verbose = rc.Verbose
			return printSSHCommand.Execute(cmd)
		},
	}
	printSSHCommand.Register(cmd)
	return cmd
}

func (rc *RootCommand) newExecCommand() *cobra.Command {
	execCommand := execcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "exec <node-name>",
		Short:         "Execute commands on simulation nodes via SSH",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			execCommand.Verbose = rc.Verbose
			execCommand.Stderr = cmd.ErrOrStderr()
			return execCommand.Execute(args, cmd.ArgsLenAtDash())
		},
	}
	execCommand.Register(cmd)
	return cmd
}

func (rc *RootCommand) newCopyCommand() *cobra.Command {
	copyCommand := cpcmd.NewCommand()
	cmd := &cobra.Command{
		Use:           "cp <src> <dest>",
		Short:         "Copy files between local machine and simulation nodes",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			copyCommand.Verbose = rc.Verbose
			copyCommand.Stderr = cmd.ErrOrStderr()
			return copyCommand.Execute(args)
		},
	}
	copyCommand.Register(cmd)
	return cmd
}
