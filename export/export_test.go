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

func (m *mockClient) ReadConversation(convID string, known func(int) bool) ([]keybase.MsgSummary, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	var result []keybase.MsgSummary
	for _, msg := range m.msgs {
		if known != nil && known(msg.ID) {
			break
		}
		result = append(result, msg)
	}
	return result, nil
}

func (m *mockClient) DownloadAttachment(channel keybase.ChatChannel, msgID int, outPath string) error {
	m.dlCalled++
	if m.dlErr != nil {
		return m.dlErr
	}
	return os.WriteFile(outPath, []byte(fmt.Sprintf("content-for-msg-%d", msgID)), 0644)
}

func (m *mockClient) Close() error { return nil }

func testConv() keybase.ConvSummary {
	return keybase.ConvSummary{
		ID:      "conv1",
		Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"},
	}
}

func testMsgs() []keybase.MsgSummary {
	return []keybase.MsgSummary{
		{ID: 3, SentAtMs: 3000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "newest"}}},
		{ID: 2, SentAtMs: 2000, Content: keybase.MsgContent{Type: "attachment", Attachment: &keybase.AttachmentContent{
			Object: keybase.AttachmentObject{Filename: "photo.jpg"},
		}}, Prev: []keybase.Prev{{ID: 1}}},
		{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "oldest"}}},
	}
}

func TestExportConversation_DirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	client := &mockClient{msgs: testMsgs()}
	result := ExportConversation(client, testConv(), dir, "self", false, false)

	convDir := filepath.Join(dir, "Chats", "alice")

	// messages/<id>/message.json exists for each message
	for _, id := range []int{1, 2, 3} {
		p := filepath.Join(convDir, "messages", fmt.Sprintf("%d", id), "message.json")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("messages/%d/message.json missing: %v", id, err)
		}
	}
	// head file exists
	if _, err := os.Stat(filepath.Join(convDir, "head")); err != nil {
		t.Errorf("head missing: %v", err)
	}
	head, _ := ReadHead(convDir)
	if head != 3 {
		t.Errorf("head = %d, want 3", head)
	}
	// attachments/ directory exists
	if _, err := os.Stat(filepath.Join(convDir, "attachments")); err != nil {
		t.Errorf("attachments/ missing: %v", err)
	}
	// per-message attachments.json for msg 2
	if _, err := os.Stat(filepath.Join(convDir, "messages", "2", "attachments.json")); err != nil {
		t.Errorf("messages/2/attachments.json missing: %v", err)
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
	// messages exist
	if _, err := os.Stat(filepath.Join(convDir, "messages", "1", "message.json")); err != nil {
		t.Errorf("message.json missing: %v", err)
	}
	// per-message attachments.json should NOT exist
	if _, err := os.Stat(filepath.Join(convDir, "messages", "2", "attachments.json")); err == nil {
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

	if result.MessagesExported != 3 {
		t.Errorf("MessagesExported = %d, want 3", result.MessagesExported)
	}
	if result.AttachmentsDownloaded != 0 {
		t.Errorf("AttachmentsDownloaded = %d, want 0", result.AttachmentsDownloaded)
	}
	if len(result.Errors) == 0 {
		t.Error("expected attachment error")
	}
}

func TestExportConversation_Incremental(t *testing.T) {
	dir := t.TempDir()
	// First run: export messages 1-3
	client := &mockClient{msgs: testMsgs()}
	result := ExportConversation(client, testConv(), dir, "self", true, false)
	if result.MessagesExported != 3 {
		t.Fatalf("first run: MessagesExported = %d, want 3", result.MessagesExported)
	}

	// Second run: messages 4,5 are new, 3 is known → stops
	client2 := &mockClient{msgs: []keybase.MsgSummary{
		{ID: 5, SentAtMs: 5000, Content: keybase.MsgContent{Type: "text"}, Prev: []keybase.Prev{{ID: 4}}},
		{ID: 4, SentAtMs: 4000, Content: keybase.MsgContent{Type: "text"}, Prev: []keybase.Prev{{ID: 3}}},
		{ID: 3, SentAtMs: 3000, Content: keybase.MsgContent{Type: "text"}}, // known
	}}
	result2 := ExportConversation(client2, testConv(), dir, "self", true, false)
	if result2.MessagesExported != 2 {
		t.Errorf("second run: MessagesExported = %d, want 2", result2.MessagesExported)
	}
	head, _ := ReadHead(filepath.Join(dir, "Chats", "alice"))
	if head != 5 {
		t.Errorf("head = %d, want 5", head)
	}
}

func TestExportConversation_OrphanDetection(t *testing.T) {
	dir := t.TempDir()
	// Export message 5 which has prev pointing to message 3 (which doesn't exist locally)
	client := &mockClient{msgs: []keybase.MsgSummary{
		{ID: 5, SentAtMs: 5000, Content: keybase.MsgContent{Type: "text"},
			Prev: []keybase.Prev{{ID: 3, Hash: "abc"}}},
	}}
	result := ExportConversation(client, testConv(), dir, "self", true, false)
	if result.MessagesExported != 1 {
		t.Fatalf("MessagesExported = %d, want 1", result.MessagesExported)
	}

	convDir := filepath.Join(dir, "Chats", "alice")
	orphans, err := ReadOrphans(convDir, 5)
	if err != nil {
		t.Fatalf("read orphans: %v", err)
	}
	if len(orphans) != 1 || orphans[0].ID != 3 {
		t.Errorf("orphans = %+v, want [{ID:3 Hash:abc}]", orphans)
	}
}
