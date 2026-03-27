package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/major0/keybase-export/export"
	"github.com/major0/keybase-export/keybase"
)

var errHelp = errors.New("help requested")

type Config struct {
	DestDir         string
	Filters         []string
	Parallel        int
	Verbose         bool
	SkipAttachments bool
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: keybase-export [options] <destdir> [filters...]

Export Keybase chat history to a local directory.

Arguments:
  <destdir>       Destination directory for exported data
  [filters...]    Optional conversation filters (Chat/<participants> or Team/<team_name>)

Options:
  -P, --parallel=<n>    Number of concurrent workers (default: 4)
  --verbose             Enable detailed logging
  --skip-attachments    Skip downloading attachments
  --help                Show this help message
`)
}

func parseParallel(val string) (int, error) {
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid parallel value: %s", val)
	}
	return n, nil
}

func parseArgs(args []string) (*Config, error) {
	cfg := &Config{Parallel: 4}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return nil, errHelp
		case arg == "--verbose":
			cfg.Verbose = true
		case arg == "--skip-attachments":
			cfg.SkipAttachments = true
		case arg == "-P":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("-P requires a value")
			}
			n, err := parseParallel(args[i])
			if err != nil {
				return nil, err
			}
			cfg.Parallel = n
		case strings.HasPrefix(arg, "--parallel="):
			n, err := parseParallel(arg[len("--parallel="):])
			if err != nil {
				return nil, err
			}
			cfg.Parallel = n
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
			if cfg.DestDir == "" {
				cfg.DestDir = arg
			} else {
				cfg.Filters = append(cfg.Filters, arg)
			}
		}
		i++
	}

	if cfg.DestDir == "" {
		return nil, fmt.Errorf("destdir is required")
	}
	return cfg, nil
}

// getSelfUsername retrieves the authenticated Keybase username via "keybase status --json".
func getSelfUsername() (string, error) {
	out, err := exec.Command("keybase", "status", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("keybase status: %w", err)
	}
	// Parse just the Username field from the status JSON.
	var status struct {
		Username string `json:"Username"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return "", fmt.Errorf("parse keybase status: %w", err)
	}
	if status.Username == "" {
		return "", fmt.Errorf("no authenticated keybase user")
	}
	return status.Username, nil
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, errHelp) {
			usage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		usage()
		os.Exit(1)
	}

	if _, err := exec.LookPath("keybase"); err != nil {
		fmt.Fprintf(os.Stderr, "error: keybase CLI not found in PATH\n")
		os.Exit(1)
	}

	// Get the authenticated username
	selfUsername, err := getSelfUsername()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	exportCfg := export.Config{
		DestDir:         cfg.DestDir,
		Filters:         cfg.Filters,
		Parallel:        cfg.Parallel,
		Verbose:         cfg.Verbose,
		SkipAttachments: cfg.SkipAttachments,
		SelfUsername:     selfUsername,
	}

	listClient, err := keybase.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	newClient := func() (export.ClientAPI, error) {
		return keybase.NewClient()
	}

	if _, err := export.Run(exportCfg, listClient, newClient); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
