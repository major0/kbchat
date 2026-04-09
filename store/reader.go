package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/major0/kbchat/keybase"
)

// ReadMessages reads messages from a conversation directory on disk.
// It lists messages/ subdirectories, sorts by numeric ID ascending,
// and reads each message.json file. If count > 0, only the last count
// messages are returned. If count == 0, all messages are returned.
func ReadMessages(convDir string, count int) ([]keybase.MsgSummary, error) {
	msgsDir := filepath.Join(convDir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read messages dir: %w", err)
	}

	// Collect numeric directory IDs.
	ids := make([]int, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // skip non-numeric directories
		}
		ids = append(ids, id)
	}

	sort.Ints(ids)

	// If count > 0, take only the last count IDs.
	if count > 0 && count < len(ids) {
		ids = ids[len(ids)-count:]
	}

	msgs := make([]keybase.MsgSummary, 0, len(ids))
	for _, id := range ids {
		msgPath := filepath.Join(msgsDir, strconv.Itoa(id), "message.json")
		data, err := os.ReadFile(msgPath)
		if err != nil {
			return nil, fmt.Errorf("read message %d: %w", id, err)
		}
		var msg keybase.MsgSummary
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal message %d: %w", id, err)
		}
		msgs = append(msgs, msg)
	}

	return msgs, nil
}
