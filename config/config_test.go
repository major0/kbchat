package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
)

// Feature: keybase-chat-cli, Property 11: Config file round-trip
//
// For any valid Config struct, writing to JSON and reading back must produce
// an equivalent struct.
//
// **Validates: Requirements 1.2, 1.4**
func TestPropertyConfigRoundTrip(t *testing.T) {
	f := func(storePath, timeFormat string) bool {
		cfg := Config{
			StorePath:  storePath,
			TimeFormat: timeFormat,
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			return false
		}
		var got Config
		if err := json.Unmarshal(data, &got); err != nil {
			return false
		}
		return got.StorePath == cfg.StorePath && got.TimeFormat == cfg.TimeFormat
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("config round-trip property failed: %v", err)
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns config file path
		wantErr   bool
		errSubstr string
	}{
		{
			name: "missing file",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.json")
			},
			wantErr:   true,
			errSubstr: "config file not found",
		},
		{
			name: "bad JSON",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				p := filepath.Join(dir, "config.json")
				if err := os.WriteFile(p, []byte("{bad json}"), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr:   true,
			errSubstr: "parsing config file",
		},
		{
			name: "valid config",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				p := filepath.Join(dir, "config.json")
				data := []byte(`{"store_path":"` + dir + `","time_format":"15:04"}`)
				if err := os.WriteFile(p, data, 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			cfg, err := LoadFrom(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("expected non-nil config")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) Config
		wantErr   bool
		errSubstr string
	}{
		{
			name: "empty store_path",
			setup: func(t *testing.T) Config {
				return Config{StorePath: ""}
			},
			wantErr:   true,
			errSubstr: "store_path is required",
		},
		{
			name: "missing store_path dir",
			setup: func(t *testing.T) Config {
				return Config{StorePath: filepath.Join(t.TempDir(), "nonexistent")}
			},
			wantErr:   true,
			errSubstr: "store_path does not exist",
		},
		{
			name: "store_path is a file",
			setup: func(t *testing.T) Config {
				dir := t.TempDir()
				p := filepath.Join(dir, "afile")
				if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return Config{StorePath: p}
			},
			wantErr:   true,
			errSubstr: "not a directory",
		},
		{
			name: "valid config",
			setup: func(t *testing.T) Config {
				return Config{StorePath: t.TempDir()}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setup(t)
			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTimeFmt(t *testing.T) {
	tests := []struct {
		name       string
		timeFormat string
		want       string
	}{
		{
			name:       "default when empty",
			timeFormat: "",
			want:       DefaultTimeFormat,
		},
		{
			name:       "custom format",
			timeFormat: "15:04:05",
			want:       "15:04:05",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{TimeFormat: tt.timeFormat}
			if got := cfg.TimeFmt(); got != tt.want {
				t.Errorf("TimeFmt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
