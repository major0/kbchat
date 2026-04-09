// Package cmd implements kbchat subcommand handlers.
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/export"
	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/logfile"
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

// exportFunc is the signature for the export runner, matching export.Run.
type exportFunc func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error)

// RunExport executes the export subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunExport(args []string, cfg *config.Config, selfUsername string) error {
	return runExport(args, cfg, selfUsername, keybase.NewClient, nil, nil)
}

// runExport is the internal implementation of RunExport, accepting injectable
// dependencies for testing. sleepFunc overrides time-based sleeping when
// non-nil; it receives the context and interval, returning an error if the
// context was cancelled during the sleep. runFunc overrides export.Run when
// non-nil.
func runExport(
	args []string,
	cfg *config.Config,
	selfUsername string,
	newKeybaseClient func() (*keybase.Client, error),
	sleepFunc func(ctx context.Context, d time.Duration) error,
	runFunc exportFunc,
) error {
	opts, err := parseExportArgs(args)
	if err != nil {
		return err
	}

	// Set up log file if --log-file is specified (Req 3.9–3.12)
	if opts.LogFile != "" {
		lf, err := logfile.Open(opts.LogFile)
		if err != nil {
			return fmt.Errorf("setting up log file: %w", err)
		}
		defer func() { _ = lf.Close() }()

		// Disable default log timestamp; LogFile.Write prepends ISO 8601
		log.SetFlags(0)
		log.SetOutput(lf)

		// SIGHUP reopens the log file (logrotate support)
		sighup := make(chan os.Signal, 1)
		signal.Notify(sighup, syscall.SIGHUP)
		go func() {
			for range sighup {
				if err := lf.Reopen(); err != nil {
					// Best effort: write to stderr since log file may be broken
					fmt.Fprintf(os.Stderr, "SIGHUP reopen failed: %v\n", err)
				}
			}
		}()
		defer signal.Stop(sighup)
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

	// Worker factory: each worker gets its own keybase client
	newClient := func() (export.ClientAPI, error) {
		return newKeybaseClient()
	}

	run := runFunc
	if run == nil {
		run = export.Run
	}

	if !opts.Continuous {
		// Single-shot export
		listClient, err := newKeybaseClient()
		if err != nil {
			return fmt.Errorf("creating keybase client: %w", err)
		}
		_, err = run(exportCfg, listClient, newClient)
		return err
	}

	// Continuous mode: loop with signal-based cancellation (Req 3.5, 3.8)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sleep := sleepFunc
	if sleep == nil {
		sleep = defaultSleep
	}

	for {
		listClient, err := newKeybaseClient()
		if err != nil {
			return fmt.Errorf("creating keybase client: %w", err)
		}

		// Run one export cycle; let it complete even if signal arrived
		_, err = run(exportCfg, listClient, newClient)
		if err != nil {
			log.Printf("export cycle error: %v", err)
		}

		// Check if cancelled before sleeping
		if ctx.Err() != nil {
			return nil
		}

		// Sleep between cycles; wake early on signal
		if err := sleep(ctx, opts.Interval); err != nil {
			return nil // context cancelled during sleep
		}
	}
}

// defaultSleep sleeps for d or until ctx is cancelled, whichever comes first.
func defaultSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
