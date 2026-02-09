package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/output"
)

// RootCommand is the main CLI entry point.
type RootCommand struct {
	flagSet *flag.FlagSet
	Verbose bool
}

// NewRootCommand creates a new root command.
func NewRootCommand() *RootCommand {
	return &RootCommand{
		flagSet: flag.NewFlagSet("nvcli", flag.ContinueOnError),
	}
}

// Run executes the CLI with the given arguments.
// Returns 0 on success, non-zero on error.
func (rc *RootCommand) Run(args []string) int {
	// Handle help
	if len(args) == 0 || (len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help")) {
		rc.printHelp()
		return 0
	}

	// Check for global --verbose flag before routing
	verbose, remainingArgs := rc.extractVerboseFlag(args)
	if verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}

	// Route to subcommands
	subcommand := remainingArgs[0]
	subArgs := remainingArgs[1:]

	switch subcommand {
	case "login":
		return rc.handleLogin(subArgs, verbose)
	case "logout":
		return rc.handleLogout(subArgs, verbose)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcommand)
		rc.printHelp()
		return 1
	}
}

// extractVerboseFlag extracts the --verbose flag from args and returns the verbose state
// and the remaining arguments.
func (rc *RootCommand) extractVerboseFlag(args []string) (bool, []string) {
	verbose := false
	var remaining []string

	for i, arg := range args {
		if arg == "--verbose" || arg == "-v" {
			verbose = true
		} else {
			remaining = args[i:]
			break
		}
	}

	// If no other args after --verbose, keep the command if present
	if len(remaining) == 0 && len(args) > 0 {
		// Check if first arg was not --verbose, then it's the command
		if args[0] != "--verbose" && args[0] != "-v" {
			remaining = args
		}
	}

	return verbose, remaining
}

// handleLogin handles the login subcommand.
func (rc *RootCommand) handleLogin(args []string, verbose bool) int {
	loginCmd := NewLoginCommand()
	loginCmd.Verbose = verbose

	// Create a flag set for the login command
	fs := flag.NewFlagSet("nvcli login", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Printf("Usage: nvcli login [options]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	loginCmd.Register(fs)

	// Parse flags
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	// Execute login command
	if err := loginCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", output.FormatError(err))
		return 1
	}

	return 0
}

// handleLogout handles the logout subcommand.
func (rc *RootCommand) handleLogout(args []string, verbose bool) int {
	logoutCmd := NewLogoutCommand()
	logoutCmd.Verbose = verbose

	// Create a flag set for the logout command
	fs := flag.NewFlagSet("nvcli logout", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Printf("Usage: nvcli logout [options]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	logoutCmd.Register(fs)

	// Parse flags
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		return 1
	}

	// Execute logout command
	if err := logoutCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", output.FormatError(err))
		return 1
	}

	return 0
}

// printHelp prints the help message.
func (rc *RootCommand) printHelp() {
	help := `nvcli - NVIDIA Virtual Air CLI

Usage:
  nvcli [global options] [command] [options]

Commands:
  login       Authenticate with NVIDIA Virtual Air
  logout      Log out from NVIDIA Virtual Air
  help        Show this help message

Global Options:
  -v, --verbose  Enable verbose logging for detailed debugging
  -h, --help     Show help message

Examples:
  nvcli login -u user@example.com -p <api-token>
  nvcli --verbose login -u user@example.com -p <api-token>
  nvcli logout -f

Documentation:
  For more information, visit: https://docs.nvidia.com/nvair/

`
	fmt.Print(help)
}
