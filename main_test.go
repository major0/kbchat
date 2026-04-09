package main

import (
	"testing"

	"github.com/major0/optargs"
)

func TestNewRootParser(t *testing.T) {
	p, verbose, err := newRootParser([]string{"--verbose", "help"})
	if err != nil {
		t.Fatalf("newRootParser: %v", err)
	}

	// Iterate to process flags and dispatch subcommand
	for _, err := range p.Options() {
		if err != nil {
			t.Fatalf("Options: %v", err)
		}
	}

	if !*verbose {
		t.Error("expected verbose=true after --verbose")
	}

	subcmd, _ := p.ActiveCommand()
	if subcmd != "help" {
		t.Errorf("ActiveCommand = %q, want %q", subcmd, "help")
	}
}

func TestNewRootParserSubcommands(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantCmd string
	}{
		{"export", []string{"export"}, "export"},
		{"list", []string{"list"}, "list"},
		{"ls alias", []string{"ls"}, "ls"},
		{"view", []string{"view"}, "view"},
		{"search", []string{"search"}, "search"},
		{"help", []string{"help"}, "help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _, err := newRootParser(tt.args)
			if err != nil {
				t.Fatalf("newRootParser: %v", err)
			}
			for _, err := range p.Options() {
				if err != nil {
					t.Fatalf("Options: %v", err)
				}
			}
			subcmd, _ := p.ActiveCommand()
			if subcmd != tt.wantCmd {
				t.Errorf("ActiveCommand = %q, want %q", subcmd, tt.wantCmd)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"help command", []string{"help"}, 0},
		{"help export", []string{"help", "export"}, 0},
		{"help list", []string{"help", "list"}, 0},
		{"help view", []string{"help", "view"}, 0},
		{"help search", []string{"help", "search"}, 0},
		{"help help", []string{"help", "help"}, 0},
		{"help unknown", []string{"help", "bogus"}, 1},
		{"root --help", []string{"--help"}, 0},
		{"root -h", []string{"-h"}, 0},
		{"export --help", []string{"export", "--help"}, 0},
		{"list --help", []string{"list", "--help"}, 0},
		{"view --help", []string{"view", "--help"}, 0},
		{"search --help", []string{"search", "--help"}, 0},
		{"--help export", []string{"--help", "export"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := run(tt.args)
			if code != tt.wantCode {
				t.Errorf("run(%v) = %d, want %d", tt.args, code, tt.wantCode)
			}
		})
	}
}

func TestRunNoSubcommand(t *testing.T) {
	code := run([]string{})
	if code != 1 {
		t.Errorf("run([]) = %d, want 1", code)
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	code := run([]string{"bogus"})
	if code != 1 {
		t.Errorf("run([bogus]) = %d, want 1", code)
	}
}

func TestHandleHelp(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"no args", nil, 0},
		{"export", []string{"export"}, 0},
		{"list", []string{"list"}, 0},
		{"view", []string{"view"}, 0},
		{"search", []string{"search"}, 0},
		{"unknown", []string{"bogus"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal parser to simulate subParser
			var subParser *optargs.Parser
			if tt.args != nil {
				subParser = &optargs.Parser{Args: tt.args}
			}
			code := handleHelp(subParser)
			if code != tt.wantCode {
				t.Errorf("handleHelp(%v) = %d, want %d", tt.args, code, tt.wantCode)
			}
		})
	}
}
