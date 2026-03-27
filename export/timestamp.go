package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/major0/keybase-export/keybase"
)

// ReadTimestamp reads a Unix millisecond timestamp from a plain text file.
// Returns 0 and nil error if the file does not exist or is corrupted.
func ReadTimestamp(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, nil // treat corruption as missing
	}
	return ts, nil
}

// WriteTimestampAtomic writes a Unix millisecond timestamp to a file atomically
// using write-to-temp + rename.
func WriteTimestampAtomic(path string, ts int64) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".timestamp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := fmt.Fprintf(tmp, "%d\n", ts); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write timestamp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// WriteMessages serializes messages to a JSON file with pretty-printing.
func WriteMessages(path string, msgs []keybase.MsgSummary) error {
	data, err := json.MarshalIndent(msgs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// ReadMessages deserializes messages from a JSON file.
func ReadMessages(path string) ([]keybase.MsgSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var msgs []keybase.MsgSummary
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return msgs, nil
}
