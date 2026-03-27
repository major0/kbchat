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

func msgDir(convDir string, msgID int) string {
	return filepath.Join(convDir, "messages", strconv.Itoa(msgID))
}

// MsgExists checks if a message directory exists (O(1) stat).
func MsgExists(convDir string, msgID int) bool {
	_, err := os.Stat(msgDir(convDir, msgID))
	return err == nil
}

// WriteMsg writes a message to messages/<id>/message.json.
func WriteMsg(convDir string, msg keybase.MsgSummary) error {
	dir := msgDir(convDir, msg.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create message dir: %w", err)
	}
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "message.json"), data, 0644)
}

// ReadMsg reads a message from messages/<id>/message.json.
func ReadMsg(convDir string, msgID int) (*keybase.MsgSummary, error) {
	data, err := os.ReadFile(filepath.Join(msgDir(convDir, msgID), "message.json"))
	if err != nil {
		return nil, err
	}
	var msg keybase.MsgSummary
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	return &msg, nil
}

// ReadHead reads the current head message ID from the head file.
// Returns 0 if the file does not exist.
func ReadHead(convDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(convDir, "head"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, nil // treat corruption as missing
	}
	return id, nil
}

// WriteHead writes the head message ID to the head file.
func WriteHead(convDir string, msgID int) error {
	return os.WriteFile(filepath.Join(convDir, "head"), []byte(strconv.Itoa(msgID)+"\n"), 0644)
}

// WriteOrphans writes orphaned prev pointers to messages/<id>/orphans.json.
func WriteOrphans(convDir string, msgID int, orphans []keybase.Prev) error {
	dir := msgDir(convDir, msgID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create message dir: %w", err)
	}
	data, err := json.MarshalIndent(orphans, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal orphans: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "orphans.json"), data, 0644)
}

// ReadOrphans reads orphaned prev pointers from messages/<id>/orphans.json.
func ReadOrphans(convDir string, msgID int) ([]keybase.Prev, error) {
	data, err := os.ReadFile(filepath.Join(msgDir(convDir, msgID), "orphans.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var orphans []keybase.Prev
	if err := json.Unmarshal(data, &orphans); err != nil {
		return nil, fmt.Errorf("unmarshal orphans: %w", err)
	}
	return orphans, nil
}

// WriteMsgAttachments writes per-message attachment manifest to messages/<id>/attachments.json.
func WriteMsgAttachments(convDir string, msgID int, refs []AttachmentRef) error {
	dir := msgDir(convDir, msgID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create message dir: %w", err)
	}
	data, err := json.MarshalIndent(refs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal attachments: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "attachments.json"), data, 0644)
}
