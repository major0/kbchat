package main

import (
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErr         bool
		errContains     string
		wantDestDir     string
		wantFilters     []string
		wantParallel    int
		wantVerbose     bool
		wantSkipAttach  bool
	}{
		{
			name:         "destdir only",
			args:         []string{"/tmp/out"},
			wantDestDir:  "/tmp/out",
			wantParallel: 4,
		},
		{
			name:         "destdir with filters",
			args:         []string{"/tmp/out", "Chat/alice,bob", "Team/myteam"},
			wantDestDir:  "/tmp/out",
			wantFilters:  []string{"Chat/alice,bob", "Team/myteam"},
			wantParallel: 4,
		},
		{
			name:         "parallel short flag",
			args:         []string{"-P", "8", "/tmp/out"},
			wantDestDir:  "/tmp/out",
			wantParallel: 8,
		},
		{
			name:         "parallel long flag",
			args:         []string{"--parallel=16", "/tmp/out"},
			wantDestDir:  "/tmp/out",
			wantParallel: 16,
		},
		{
			name:         "default parallel is 4",
			args:         []string{"/tmp/out"},
			wantDestDir:  "/tmp/out",
			wantParallel: 4,
		},
		{
			name:        "verbose flag",
			args:        []string{"--verbose", "/tmp/out"},
			wantDestDir: "/tmp/out",
			wantVerbose: true,
			wantParallel: 4,
		},
		{
			name:           "skip-attachments flag",
			args:           []string{"--skip-attachments", "/tmp/out"},
			wantDestDir:    "/tmp/out",
			wantSkipAttach: true,
			wantParallel:   4,
		},
		{
			name:           "all flags combined",
			args:           []string{"--verbose", "--skip-attachments", "-P", "2", "/tmp/out", "Chat/alice"},
			wantDestDir:    "/tmp/out",
			wantFilters:    []string{"Chat/alice"},
			wantParallel:   2,
			wantVerbose:    true,
			wantSkipAttach: true,
		},
		{
			name:        "help short",
			args:        []string{"-h"},
			wantErr:     true,
			errContains: "help requested",
		},
		{
			name:        "help long",
			args:        []string{"--help"},
			wantErr:     true,
			errContains: "help requested",
		},
		{
			name:        "missing destdir",
			args:        []string{},
			wantErr:     true,
			errContains: "missing required argument",
		},
		{
			name:        "missing destdir with flags only",
			args:        []string{"--verbose"},
			wantErr:     true,
			errContains: "missing required argument",
		},
		{
			name:        "unknown flag",
			args:        []string{"--bogus", "/tmp/out"},
			wantErr:     true,
			errContains: "unknown flag",
		},
		{
			name:        "-P missing value",
			args:        []string{"-P"},
			wantErr:     true,
			errContains: "-P requires a value",
		},
		{
			name:        "-P invalid number",
			args:        []string{"-P", "abc", "/tmp/out"},
			wantErr:     true,
			errContains: "invalid number",
		},
		{
			name:        "--parallel invalid number",
			args:        []string{"--parallel=abc", "/tmp/out"},
			wantErr:     true,
			errContains: "invalid number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.DestDir != tt.wantDestDir {
				t.Errorf("DestDir = %q, want %q", cfg.DestDir, tt.wantDestDir)
			}
			if !sliceEqual(cfg.Filters, tt.wantFilters) {
				t.Errorf("Filters = %v, want %v", cfg.Filters, tt.wantFilters)
			}
			if cfg.Parallel != tt.wantParallel {
				t.Errorf("Parallel = %d, want %d", cfg.Parallel, tt.wantParallel)
			}
			if cfg.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v, want %v", cfg.Verbose, tt.wantVerbose)
			}
			if cfg.SkipAttachments != tt.wantSkipAttach {
				t.Errorf("SkipAttachments = %v, want %v", cfg.SkipAttachments, tt.wantSkipAttach)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

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
