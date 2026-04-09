// Package config loads and validates the kbchat configuration file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultTimeFormat is the Go time layout used when time_format is not set.
const DefaultTimeFormat = "2006-01-02 15:04:05.00 MST"

// DefaultConfigPath returns the default config file path (~/.config/kbchat/config.json).
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "kbchat", "config.json")
}

// Config holds the kbchat configuration.
type Config struct {
	StorePath  string `json:"store_path"`
	TimeFormat string `json:"time_format,omitempty"`
}

// Load reads the config from the default path (~/.config/kbchat/config.json).
func Load() (*Config, error) {
	return LoadFrom(DefaultConfigPath())
}

// LoadFrom reads and parses a config file at the given path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s\n\nCreate it with:\n  mkdir -p %s\n  cat > %s << 'EOF'\n  {\n    \"store_path\": \"/path/to/keybase-backup\"\n  }\n  EOF",
				path, filepath.Dir(path), path)
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	return &cfg, nil
}

// Validate checks that the config is usable: store_path must be non-empty
// and must exist on disk.
func (c *Config) Validate() error {
	if c.StorePath == "" {
		return fmt.Errorf("store_path is required in config file")
	}
	info, err := os.Stat(c.StorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("store_path does not exist: %s", c.StorePath)
		}
		return fmt.Errorf("checking store_path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("store_path is not a directory: %s", c.StorePath)
	}
	return nil
}

// TimeFmt returns the configured time format, or DefaultTimeFormat if unset.
func (c *Config) TimeFmt() string {
	if c.TimeFormat != "" {
		return c.TimeFormat
	}
	return DefaultTimeFormat
}
