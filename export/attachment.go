package export

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/major0/keybase-export/keybase"
)

// DeduplicateFilename returns a unique filename given a set of already-used names.
// The first occurrence keeps the original name; duplicates get a numeric suffix.
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

// DownloadAttachment downloads an attachment with filename deduplication.
// Returns the actual filename used on disk.
func DownloadAttachment(client *keybase.Client, convID string, msgID int, filename string, attachDir string, used map[string]bool) (string, error) {
	actual := DeduplicateFilename(filename, used)
	outPath := filepath.Join(attachDir, actual)
	if err := client.DownloadAttachment(convID, msgID, outPath); err != nil {
		return "", err
	}
	return actual, nil
}
