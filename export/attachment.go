package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/major0/keybase-export/keybase"
)

// DeduplicateFilename returns a unique filename given a set of already-used names.
// The first occurrence keeps the original name; duplicates get a numeric suffix.
// This is a pure name-based function; content-aware dedup is handled by DownloadAttachment.
func DeduplicateFilename(name string, used map[string]bool) string {
	if !used[name] {
		used[name] = true
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// filesEqual returns true if two files have identical content.
func filesEqual(a, b string) (bool, error) {
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(dataA, dataB), nil
}

// DownloadAttachment downloads an attachment with content-aware filename deduplication.
// If a file with the same name already exists and has identical content, the existing
// file is reused. If the content differs, a numeric suffix is appended.
// Returns the actual filename used on disk.
func DownloadAttachment(client *keybase.Client, convID string, msgID int, filename string, attachDir string) (string, error) {
	// Download to a temp file first
	tmp, err := os.CreateTemp(attachDir, ".download-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()

	if err := client.DownloadAttachment(convID, msgID, tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	// Check if the target filename already exists
	targetPath := filepath.Join(attachDir, filename)
	if _, err := os.Stat(targetPath); err == nil {
		// File exists — compare content
		equal, err := filesEqual(tmpPath, targetPath)
		if err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("compare files: %w", err)
		}
		if equal {
			// Identical content — reuse existing file
			os.Remove(tmpPath)
			return filename, nil
		}
		// Different content — find a unique name
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
			candidatePath := filepath.Join(attachDir, candidate)
			if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
				// Slot is free
				if err := os.Rename(tmpPath, candidatePath); err != nil {
					os.Remove(tmpPath)
					return "", fmt.Errorf("rename to %s: %w", candidate, err)
				}
				return candidate, nil
			}
			// Candidate exists — check if content matches
			equal, err := filesEqual(tmpPath, candidatePath)
			if err != nil {
				os.Remove(tmpPath)
				return "", fmt.Errorf("compare files: %w", err)
			}
			if equal {
				os.Remove(tmpPath)
				return candidate, nil
			}
		}
	}

	// File doesn't exist — use the original name
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename to %s: %w", filename, err)
	}
	return filename, nil
}
