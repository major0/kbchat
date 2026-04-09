// Package cmd implements kbchat subcommand handlers.
package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/export"
	"github.com/major0/kbchat/keybase"
	"github.com/major0/optargs"
)

// ExportOptions holds parsed export-specific flags.
type ExportOptions struct {
	Parallel        int
	Verbose         bool
	SkipAttachments bool
	Continuous      bool
	Interval        time.Duration
	LogFile         string
	DestDir         string
	Filters         []string
}

// DefaultParallel is the default number of concurrent export workers.
const DefaultParallel = 4

// DefaultInterval is the default interval between continuous export cycles.
const DefaultInterval = 5 * time.Minute

// parseExportArgs parses export-specific flags from args using optargs.
// Returns the parsed options or an error.
func parseExportArgs(args []string) (*ExportOptions, error) {
	opts := &ExportOptions{
		Parallel: DefaultParallel,
		Interval: DefaultInterval,
	}

	parallelFlag := &optargs.Flag{
		Name:   "parallel",
		HasArg: optargs.RequiredArgument,
		Help:   "Number of concurrent workers",
	}
	verboseFlag := &optargs.Flag{
		Name:   "verbose",
		HasArg: optargs.NoArgument,
		Help:   "Enable detailed logging",
	}
	skipAttachmentsFlag := &optargs.Flag{
		Name:   "skip-attachments",
		HasArg: optargs.NoArgument,
		Help:   "Skip downloading attachments",
	}
	continuousFlag := &optargs.Flag{
		Name:   "continuous",
		HasArg: optargs.NoArgument,
		Help:   "Run in a loop",
	}
	intervalFlag := &optargs.Flag{
		Name:   "interval",
		HasArg: optargs.RequiredArgument,
		Help:   "Interval between cycles (requires --continuous)",
	}
	logFileFlag := &optargs.Flag{
		Name:   "log-file",
		HasArg: optargs.RequiredArgument,
		Help:   "Redirect log output to a file",
	}

	shortOpts := map[byte]*optargs.Flag{
		'P': parallelFlag,
	}
	longOpts := map[string]*optargs.Flag{
		"parallel":         parallelFlag,
		"verbose":          verboseFlag,
		"skip-attachments": skipAttachmentsFlag,
		"continuous":       continuousFlag,
		"interval":         intervalFlag,
		"log-file":         logFileFlag,
	}

	p, err := optargs.NewParser(optargs.ParserConfig{}, shortOpts, longOpts, args)
	if err != nil {
		return nil, fmt.Errorf("creating export parser: %w", err)
	}

	for opt, err := range p.Options() {
		if err != nil {
			return nil, fmt.Errorf("parsing export flags: %w", err)
		}
		switch opt.Name {
		case "parallel", "P":
			n, perr := parsePositiveInt(opt.Arg)
			if perr != nil {
				return nil, fmt.Errorf("invalid --parallel value: %w", perr)
			}
			opts.Parallel = n
		case "verbose":
			opts.Verbose = true
		case "skip-attachments":
			opts.SkipAttachments = true
		case "continuous":
			opts.Continuous = true
		case "interval":
			d, derr := time.ParseDuration(opt.Arg)
			if derr != nil {
				return nil, fmt.Errorf("invalid --interval value: %w", derr)
			}
			opts.Interval = d
		case "log-file":
			opts.LogFile = opt.Arg
		}
	}

	// Remaining positional args: [destdir] [filters...]
	if len(p.Args) > 0 {
		opts.DestDir = p.Args[0]
		opts.Filters = p.Args[1:]
	}

	return opts, nil
}

// parsePositiveInt parses a string as a positive integer.
func parsePositiveInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a positive integer", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%q must be a positive integer", s)
	}
	return n, nil
}

// RunExport executes the export subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunExport(args []string, cfg *config.Config, selfUsername string) error {
	opts, err := parseExportArgs(args)
	if err != nil {
		return err
	}

	// Resolve destdir: positional arg overrides config store_path (Req 3.2, 3.3)
	destDir := cfg.StorePath
	if opts.DestDir != "" {
		destDir = opts.DestDir
	}
	if destDir == "" {
		return fmt.Errorf("no destination directory: provide a positional argument or set store_path in config")
	}

	// Build export config
	exportCfg := export.Config{
		DestDir:         destDir,
		Filters:         opts.Filters,
		Parallel:        opts.Parallel,
		Verbose:         opts.Verbose,
		SkipAttachments: opts.SkipAttachments,
		SelfUsername:    selfUsername,
	}

	// Create discovery client
	listClient, err := keybase.NewClient()
	if err != nil {
		return fmt.Errorf("creating keybase client: %w", err)
	}

	// Worker factory: each worker gets its own keybase client
	newClient := func() (export.ClientAPI, error) {
		return keybase.NewClient()
	}

	// Single-shot export (continuous mode will be added in Task 6)
	_, err = export.Run(exportCfg, listClient, newClient)
	return err
}
