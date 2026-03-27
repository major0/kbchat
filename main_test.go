package main

import (
	"errors"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Config
		wantErr string
	}{
		{
			name: "destdir only",
			args: []string{"/tmp/export"},
			want: &Config{DestDir: "/tmp/export", Parallel: 4},
		},
		{
			name: "destdir with filters",
			args: []string{"/tmp/export", "Chat/alice,bob", "Team/myteam"},
			want: &Config{
				DestDir:  "/tmp/export",
				Filters:  []string{"Chat/alice,bob", "Team/myteam"},
				Parallel: 4,
			},
		},
		{
			name: "parallel short flag",
			args: []string{"-P", "8", "/tmp/export"},
			want: &Config{DestDir: "/tmp/export", Parallel: 8},
		},
		{
			name: "parallel long flag",
			args: []string{"--parallel=16", "/tmp/export"},
			want: &Config{DestDir: "/tmp/export", Parallel: 16},
		},
		{
			name: "default parallel is 4",
			args: []string{"/tmp/export"},
			want: &Config{DestDir: "/tmp/export", Parallel: 4},
		},
		{
			name: "verbose flag",
			args: []string{"--verbose", "/tmp/export"},
			want: &Config{DestDir: "/tmp/export", Parallel: 4, Verbose: true},
		},
		{
			name: "skip-attachments flag",
			args: []string{"--skip-attachments", "/tmp/export"},
			want: &Config{DestDir: "/tmp/export", Parallel: 4, SkipAttachments: true},
		},
		{
			name: "all flags combined",
			args: []string{"--verbose", "--skip-attachments", "-P", "2", "/tmp/export", "Chat/alice"},
			want: &Config{
				DestDir:         "/tmp/export",
				Filters:         []string{"Chat/alice"},
				Parallel:        2,
				Verbose:         true,
				SkipAttachments: true,
			},
		},
		{
			name:    "missing destdir",
			args:    []string{},
			wantErr: "destdir is required",
		},
		{
			name:    "help flag",
			args:    []string{"--help"},
			wantErr: "help",
		},
		{
			name:    "short help flag",
			args:    []string{"-h"},
			wantErr: "help",
		},
		{
			name:    "unknown flag",
			args:    []string{"--unknown", "/tmp/export"},
			wantErr: "unknown flag: --unknown",
		},
		{
			name:    "parallel missing value",
			args:    []string{"-P"},
			wantErr: "-P requires a value",
		},
		{
			name:    "parallel invalid value",
			args:    []string{"-P", "abc", "/tmp/export"},
			wantErr: "invalid parallel value: abc",
		},
		{
			name:    "parallel zero",
			args:    []string{"-P", "0", "/tmp/export"},
			wantErr: "invalid parallel value: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if tt.wantErr == "help" {
					if !errors.Is(err, errHelp) {
						t.Fatalf("expected errHelp, got %q", err.Error())
					}
					return
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.DestDir != tt.want.DestDir {
				t.Errorf("DestDir = %q, want %q", got.DestDir, tt.want.DestDir)
			}
			if got.Parallel != tt.want.Parallel {
				t.Errorf("Parallel = %d, want %d", got.Parallel, tt.want.Parallel)
			}
			if got.Verbose != tt.want.Verbose {
				t.Errorf("Verbose = %v, want %v", got.Verbose, tt.want.Verbose)
			}
			if got.SkipAttachments != tt.want.SkipAttachments {
				t.Errorf("SkipAttachments = %v, want %v", got.SkipAttachments, tt.want.SkipAttachments)
			}
			if len(got.Filters) != len(tt.want.Filters) {
				t.Errorf("Filters len = %d, want %d", len(got.Filters), len(tt.want.Filters))
			} else {
				for i := range got.Filters {
					if got.Filters[i] != tt.want.Filters[i] {
						t.Errorf("Filters[%d] = %q, want %q", i, got.Filters[i], tt.want.Filters[i])
					}
				}
			}
		})
	}
}
