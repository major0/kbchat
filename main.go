package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// ErrHelp is returned by ParseArgs when -h or --help is requested.
var ErrHelp = errors.New("help requested")

// Config holds parsed CLI configuration.
type Config struct {
	DestDir         string
	Filters         []string
	Parallel        int
	Verbose         bool
	SkipAttachments bool
}

const usage = `Usage: keybase-export <destdir> [filters...]

Export Keybase chat history to a local directory.

Arguments:
  <destdir>       Destination directory for exported data
  [filters...]    Optional filters: Chat/<participants> or Team/<team_name>

Options:
  -P, --parallel=<n>   Number of concurrent workers (default: 4)
  --verbose            Enable detailed logging
  --skip-attachments   Skip downloading attachments
  -h, --help           Show this help message
`

// ParseArgs parses command-line arguments into a Config.
// Returns the config and any error encountered.
func ParseArgs(args []string) (Config, error) {
	cfg := Config{Parallel: 4}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			return cfg, ErrHelp
		case arg == "--verbose":
			cfg.Verbose = true
		case arg == "--skip-attachments":
			cfg.SkipAttachments = true
		case arg == "-P":
			i++
			if i >= len(args) {
				return cfg, fmt.Errorf("-P requires a value")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return cfg, fmt.Errorf("-P: invalid number: %s", args[i])
			}
			cfg.Parallel = n
		case len(arg) > len("--parallel=") && arg[:len("--parallel=")] == "--parallel=":
			val := arg[len("--parallel="):]
			n, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("--parallel: invalid number: %s", val)
			}
			cfg.Parallel = n
		case arg[0] == '-':
			return cfg, fmt.Errorf("unknown flag: %s", arg)
		default:
			if cfg.DestDir == "" {
				cfg.DestDir = arg
			} else {
				cfg.Filters = append(cfg.Filters, arg)
			}
		}
		i++
	}

	if cfg.DestDir == "" {
		return cfg, fmt.Errorf("missing required argument: <destdir>")
	}

	return cfg, nil
}

func main() {
	cfg, err := ParseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, ErrHelp) {
			fmt.Print(usage)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n%s", err, usage)
		os.Exit(1)
	}

	_ = cfg

	if _, err := exec.LookPath("keybase"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: keybase CLI not found in PATH\n")
		os.Exit(1)
	}
}
