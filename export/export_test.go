package export

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/major0/keybase-export/keybase"
)

// mockClient implements ClientAPI for testing.
type mockClient struct {
	msgs     []keybase.MsgSummary
	readErr  error
	dlErr    error
	dlCalled int
}

func (m *mockClient) ReadConversation(convID string, sinceTimestamp *int64) ([]keybase.MsgSummary, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if sinceTimestamp != nil {
		var filtered []keybase.MsgSummary
		for _, msg := range m.msgs {
			if msg.SentAtMs > *sinceTimestamp {
				filtered = append(filtered, msg)
			}
		}
		return filtered, nil
	}
	return m.msgs, nil
}

func (m *mockClient) DownloadAttachment(convID string, msgID int, outPath string) error {
	m.dlCalled++
	if m.dlErr != nil {
		return m.dlErr
	}
	// Write dummy content so hashing works
	return os.WriteFile(outPath, []byte(fmt.Sprintf("content-for-msg-%d", msgID)), 0644)
}

func testConv() keybase.ConvSummary {
	return keybase.ConvSummary{
		ID: "conv1",
		Channel: keybase.ChatChannel{
			Name:        "self,alice",
			MembersType: "impteamnative",
		},
	}
}

func testMsgs() []keybase.MsgSummary {
	return []keybase.MsgSummary{
		{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello"}}},
		{ID: 2, SentAtMs: 2000, Content: keybase.MsgContent{Type: "attachment", Attachment: &keybase.AttachmentContent{
			Object: keybase.AttachmentObject{Filename: "photo.jpg"},
		}}},
		{ID: 3, SentAtMs: 3000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "world"}}},
	}
}

func TestExportConversation_DirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	client := &mockClient{msgs: testMsgs()}
	result := ExportConversation(client, testConv(), dir, "self", false, false)

	convDir := filepath.Join(dir, "Chats", "alice")

	// messages.json exists
	if _, err := os.Stat(filepath.Join(convDir, "messages.json")); err != nil {
		t.Errorf("messages.json missing: %v", err)
	}
	// .timestamp exists
	if _, err := os.Stat(filepath.Join(convDir, ".timestamp")); err != nil {
		t.Errorf(".timestamp missing: %v", err)
	}
	// attachments/ directory exists
	if _, err := os.Stat(filepath.Join(convDir, "attachments")); err != nil {
		t.Errorf("attachments/ missing: %v", err)
	}
	// attachments.json exists (we have one attachment)
	if _, err := os.Stat(filepath.Join(convDir, "attachments.json")); err != nil {
		t.Errorf("attachments.json missing: %v", err)
	}

	if result.MessagesExported != 3 {
		t.Errorf("MessagesExported = %d, want 3", result.MessagesExported)
	}
	if result.AttachmentsDownloaded != 1 {
		t.Errorf("AttachmentsDownloaded = %d, want 1", result.AttachmentsDownloaded)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestExportConversation_SkipAttachments(t *testing.T) {
	dir := t.TempDir()
	client := &mockClient{msgs: testMsgs()}
	result := ExportConversation(client, testConv(), dir, "self", true, false)

	convDir := filepath.Join(dir, "Chats", "alice")

	// messages.json exists
	if _, err := os.Stat(filepath.Join(convDir, "messages.json")); err != nil {
		t.Errorf("messages.json missing: %v", err)
	}
	// attachments.json should NOT exist
	if _, err := os.Stat(filepath.Join(convDir, "attachments.json")); err == nil {
		t.Error("attachments.json should not exist with skip-attachments")
	}

	if result.AttachmentsDownloaded != 0 {
		t.Errorf("AttachmentsDownloaded = %d, want 0", result.AttachmentsDownloaded)
	}
	if client.dlCalled != 0 {
		t.Errorf("download called %d times, want 0", client.dlCalled)
	}
}

func TestExportConversation_APIFailure(t *testing.T) {
	dir := t.TempDir()
	client := &mockClient{readErr: fmt.Errorf("api down")}
	result := ExportConversation(client, testConv(), dir, "self", false, false)

	if len(result.Errors) == 0 {
		t.Error("expected errors for API failure")
	}
	if result.MessagesExported != 0 {
		t.Errorf("MessagesExported = %d, want 0", result.MessagesExported)
	}
}

func TestExportConversation_AttachmentFailureContinues(t *testing.T) {
	dir := t.TempDir()
	client := &mockClient{msgs: testMsgs(), dlErr: fmt.Errorf("download failed")}
	result := ExportConversation(client, testConv(), dir, "self", false, false)

	// Messages should still be exported
	if result.MessagesExported != 3 {
		t.Errorf("MessagesExported = %d, want 3", result.MessagesExported)
	}
	// Attachment download failed but export continued
	if result.AttachmentsDownloaded != 0 {
		t.Errorf("AttachmentsDownloaded = %d, want 0", result.AttachmentsDownloaded)
	}
	if len(result.Errors) == 0 {
		t.Error("expected attachment error")
	}
	// Timestamp should still be written
	convDir := filepath.Join(dir, "Chats", "alice")
	ts, err := ReadTimestamp(filepath.Join(convDir, ".timestamp"))
	if err != nil {
		t.Fatalf("read timestamp: %v", err)
	}
	if ts != 3000 {
		t.Errorf("timestamp = %d, want 3000", ts)
	}
}
