package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErr        bool
		errSubstr      string
		wantParallel   int
		wantVerbose    bool
		wantSkipAttach bool
		wantContinuous bool
		wantInterval   time.Duration
		wantLogFile    string
		wantDestDir    string
		wantFilters    []string
	}{
		{
			name:         "defaults with no flags",
			args:         []string{},
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:         "-P flag",
			args:         []string{"-P", "8"},
			wantParallel: 8,
			wantInterval: DefaultInterval,
		},
		{
			name:         "--verbose flag",
			args:         []string{"--verbose"},
			wantVerbose:  true,
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:           "--skip-attachments flag",
			args:           []string{"--skip-attachments"},
			wantSkipAttach: true,
			wantParallel:   DefaultParallel,
			wantInterval:   DefaultInterval,
		},
		{
			name:           "--continuous flag",
			args:           []string{"--continuous"},
			wantContinuous: true,
			wantParallel:   DefaultParallel,
			wantInterval:   DefaultInterval,
		},
		{
			name:         "--interval with valid duration",
			args:         []string{"--interval", "10m"},
			wantParallel: DefaultParallel,
			wantInterval: 10 * time.Minute,
		},
		{
			name:      "--interval with invalid duration",
			args:      []string{"--interval", "notaduration"},
			wantErr:   true,
			errSubstr: "invalid --interval value",
		},
		{
			name:         "--log-file flag",
			args:         []string{"--log-file", "/var/log/kbchat.log"},
			wantLogFile:  "/var/log/kbchat.log",
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:         "destdir from positional arg",
			args:         []string{"/tmp/export"},
			wantDestDir:  "/tmp/export",
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:         "filters from remaining positional args",
			args:         []string{"/tmp/export", "Chat/alice,bob", "Team/eng"},
			wantDestDir:  "/tmp/export",
			wantFilters:  []string{"Chat/alice,bob", "Team/eng"},
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:           "combined flags and positional args",
			args:           []string{"-P", "2", "--verbose", "--continuous", "--interval", "30s", "--log-file", "out.log", "/data/backup", "Team/ops"},
			wantParallel:   2,
			wantVerbose:    true,
			wantContinuous: true,
			wantInterval:   30 * time.Second,
			wantLogFile:    "out.log",
			wantDestDir:    "/data/backup",
			wantFilters:    []string{"Team/ops"},
		},
		{
			name:      "invalid --parallel value",
			args:      []string{"-P", "abc"},
			wantErr:   true,
			errSubstr: "invalid --parallel value",
		},
		{
			name:      "--parallel zero",
			args:      []string{"-P", "0"},
			wantErr:   true,
			errSubstr: "invalid --parallel value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseExportArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.Parallel != tt.wantParallel {
				t.Errorf("Parallel = %d, want %d", opts.Parallel, tt.wantParallel)
			}
			if opts.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v, want %v", opts.Verbose, tt.wantVerbose)
			}
			if opts.SkipAttachments != tt.wantSkipAttach {
				t.Errorf("SkipAttachments = %v, want %v", opts.SkipAttachments, tt.wantSkipAttach)
			}
			if opts.Continuous != tt.wantContinuous {
				t.Errorf("Continuous = %v, want %v", opts.Continuous, tt.wantContinuous)
			}
			if opts.Interval != tt.wantInterval {
				t.Errorf("Interval = %v, want %v", opts.Interval, tt.wantInterval)
			}
			if opts.LogFile != tt.wantLogFile {
				t.Errorf("LogFile = %q, want %q", opts.LogFile, tt.wantLogFile)
			}
			if opts.DestDir != tt.wantDestDir {
				t.Errorf("DestDir = %q, want %q", opts.DestDir, tt.wantDestDir)
			}
			if !sliceEqual(opts.Filters, tt.wantFilters) {
				t.Errorf("Filters = %v, want %v", opts.Filters, tt.wantFilters)
			}
		})
	}
}

// sliceEqual compares two string slices for equality.
// Both nil and empty slices are treated as equivalent.
func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
