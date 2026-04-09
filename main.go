// Package main is the entry point for the kbchat CLI.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/major0/kbchat/cmd"
	"github.com/major0/kbchat/config"
	"github.com/major0/optargs"
)

const programName = "kbchat"

// usage prints the top-level usage message to stderr.
func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s <command> [options]

Commands:
  export    Export Keybase chat history to a local directory
  list      List conversations in the export store
  view      View messages from a conversation
  grep      Search messages across conversations
  help      Show help for a command

Shared Options:
  --verbose    Enable detailed logging
  --help       Show help for a command

Run '%s help <command>' for subcommand-specific usage.
`, programName, programName)
}

// subcommandUsage prints usage for a specific subcommand.
func subcommandUsage(name string) {
	switch name {
	case "export":
		fmt.Fprintf(os.Stderr, `Usage: %s export [options] [destdir] [filters...]

Export Keybase chat history to a local directory.

Arguments:
  [destdir]       Destination directory (default: store_path from config)
  [filters...]    Conversation filters (Chat/<participants> or Team/<team_name>)

Options:
  -P, --parallel=<n>      Number of concurrent workers (default: 4)
  --verbose               Enable detailed logging
  --skip-attachments      Skip downloading attachments
  --continuous            Run in a loop
  --interval=<duration>   Interval between cycles (default: 5m; requires --continuous)
  --log-file=<path>       Redirect log output to a file
  --help                  Show this help message
`, programName)
	case "list", "ls":
		fmt.Fprintf(os.Stderr, `Usage: %s list [options] [patterns...]

List conversations in the export store.

Arguments:
  [patterns...]   Glob patterns to filter conversations

Options:
  -1              One conversation per line
  -C              Column format
  -l, --verbose   Long format (type, count, timestamps, name)
  --format=<fmt>  Output format (single-column, columns, long, or custom)
  --help          Show this help message
`, programName)
	case "view":
		fmt.Fprintf(os.Stderr, `Usage: %s view [options] <filter>

View messages from a conversation.

Arguments:
  <filter>    Conversation filter

Options:
  --count=<n>          Number of messages (default: 20; 0 for all)
  --date=<YYYY-MM-DD>  Show messages from a specific day
  --after=<timestamp>  Show messages after timestamp
  --before=<timestamp> Show messages before timestamp
  --verbose            Include message IDs and metadata
  --help               Show this help message
`, programName)
	case "grep":
		fmt.Fprintf(os.Stderr, `Usage: %s grep [options] [filters...] <pattern>

Search messages across conversations.

Arguments:
  [filters...]    Conversation filters
  <pattern>       Search pattern (glob by default)

Options:
  -E, --regexp            Regular expression (Go regexp syntax)
  -i                      Case-insensitive matching
  -A <n>                  Show n messages after each match
  -B <n>                  Show n messages before each match
  -C <n>                  Show n messages before and after each match
  --count=<n>             Limit total results
  --after=<timestamp>     Filter by start time
  --before=<timestamp>    Filter by end time
  --verbose               Include message IDs and conversation IDs
  --help                  Show this help message
`, programName)
	case "help":
		fmt.Fprintf(os.Stderr, `Usage: %s help [command]

Show help for a command.

Arguments:
  [command]    The subcommand to show help for

Run '%s help' with no arguments to see all available commands.
`, programName, programName)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command: %s\n", name)
		usage()
	}
}

// newRootParser creates the root optargs parser with shared flags and
// registered subcommands.
func newRootParser(args []string) (*optargs.Parser, *bool, error) {
	verbose := false

	verboseFlag := &optargs.Flag{
		Name:   "verbose",
		HasArg: optargs.NoArgument,
		Help:   "Enable detailed logging",
	}
	helpFlag := &optargs.Flag{
		Name:   "help",
		HasArg: optargs.NoArgument,
		Help:   "Show help for a command",
	}

	shortOpts := map[byte]*optargs.Flag{
		'v': verboseFlag,
		'h': helpFlag,
	}
	longOpts := map[string]*optargs.Flag{
		"verbose": verboseFlag,
		"help":    helpFlag,
	}

	p, err := optargs.NewParser(optargs.ParserConfig{}, shortOpts, longOpts, args)
	if err != nil {
		return nil, nil, fmt.Errorf("creating parser: %w", err)
	}
	p.Name = programName
	p.Description = "Multi-subcommand CLI for Keybase chat history"

	// Register subcommands with empty parsers. Subcommands parse their
	// own args from the remaining arguments after dispatch.
	emptyParser := func() *optargs.Parser {
		sp, _ := optargs.NewParser(optargs.ParserConfig{}, nil, nil, nil)
		return sp
	}
	p.AddCmd("export", emptyParser())
	p.AddCmd("view", emptyParser())
	p.AddCmd("grep", emptyParser())
	p.AddCmd("list", emptyParser())
	_ = p.AddAlias("ls", "list")
	p.AddCmd("help", emptyParser())

	// Set handlers for shared flags
	_ = p.SetHandler("--verbose", func(_, _ string) error {
		verbose = true
		return nil
	})

	return p, &verbose, nil
}

// getSelfUsername retrieves the authenticated Keybase username via "keybase status --json".
func getSelfUsername() (string, error) {
	out, err := exec.Command("keybase", "status", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("keybase status: %w", err)
	}
	var status struct {
		Username string `json:"Username"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return "", fmt.Errorf("parse keybase status: %w", err)
	}
	if status.Username == "" {
		return "", errors.New("no authenticated keybase user")
	}
	return status.Username, nil
}

// run is the main dispatch logic, separated from main() for testability.
// It returns the exit code.
func run(args []string) int {
	p, verbose, err := newRootParser(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	helpRequested := false
	_ = p.SetHandler("--help", func(_, _ string) error {
		helpRequested = true
		return nil
	})

	// Parse shared flags and dispatch to subcommand
	for _, err := range p.Options() {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			usage()
			return 1
		}
	}

	subcmd, subParser := p.ActiveCommand()

	// Handle --help at root level (no subcommand)
	if helpRequested && subcmd == "" {
		usage()
		return 0
	}

	// No subcommand and no help → error
	if subcmd == "" {
		// Check if there are remaining args that look like a subcommand
		if len(p.Args) > 0 {
			fmt.Fprintf(os.Stderr, "error: unknown command: %s\n", p.Args[0])
		}
		usage()
		return 1
	}

	// Handle "help" subcommand
	if subcmd == "help" {
		return handleHelp(subParser)
	}

	// Handle --help for a specific subcommand (before or after subcommand name)
	if helpRequested {
		subcommandUsage(subcmd)
		return 0
	}

	// Get subcommand args
	var subArgs []string
	if subParser != nil {
		subArgs = subParser.Args
	}

	// Check for --help in subcommand args (Req 2.6: each subcommand accepts --help)
	for _, a := range subArgs {
		if a == "--help" || a == "-h" {
			subcommandUsage(subcmd)
			return 0
		}
		if a == "--" {
			break
		}
	}

	// Load config for all non-help subcommands
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// Dispatch
	switch subcmd {
	case "export":
		return dispatchExport(subArgs, cfg, *verbose)

	case "list", "ls":
		if err := cmd.RunList(subArgs, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

	case "view":
		if err := cmd.RunView(subArgs, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

	case "grep":
		if err := cmd.RunGrep(subArgs, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}

	return 0
}

// dispatchExport handles the export subcommand, including keybase dependency
// check and authenticated username resolution (Req 2.7, 2.8).
func dispatchExport(args []string, cfg *config.Config, _ bool) int {
	// Check keybase dependency only for export (Req 2.7)
	if _, err := exec.LookPath("keybase"); err != nil {
		fmt.Fprintf(os.Stderr, "error: keybase CLI not found in PATH\n")
		return 1
	}

	// Resolve authenticated username only for export (Req 2.8)
	selfUsername, err := getSelfUsername()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if err := cmd.RunExport(args, cfg, selfUsername); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// handleHelp implements the help subcommand (Req 2.4, 2.5).
func handleHelp(subParser *optargs.Parser) int {
	if subParser == nil {
		usage()
		return 0
	}

	args := subParser.Args
	if len(args) == 0 {
		usage()
		return 0
	}

	name := args[0]
	// Check if it's a known subcommand
	switch name {
	case "export", "list", "ls", "view", "grep", "help":
		subcommandUsage(name)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command: %s\n", name)
		usage()
		return 1
	}
}

func main() {
	os.Exit(run(os.Args[1:]))
}
