package export

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/major0/keybase-export/keybase"
)

// Integration test fixtures

func integrationConvs() []keybase.ConvSummary {
	return []keybase.ConvSummary{
		{ID: "dm1", Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"}},
		{ID: "group1", Channel: keybase.ChatChannel{Name: "self,alice,bob,charlie", MembersType: "impteamupgrade"}},
		{ID: "team1", Channel: keybase.ChatChannel{Name: "engineering", MembersType: "team", TopicName: "general"}},
	}
}

func integrationMsgs(convID string) []keybase.MsgSummary {
	switch convID {
	case "dm1":
		return []keybase.MsgSummary{
			{ID: 2, SentAtMs: 2000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hi alice"}}, Prev: []keybase.Prev{{ID: 1}}},
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello"}}},
		}
	case "group1":
		return []keybase.MsgSummary{
			{ID: 3, SentAtMs: 3000, Content: keybase.MsgContent{Type: "attachment", Attachment: &keybase.AttachmentContent{
				Object: keybase.AttachmentObject{Filename: "photo.jpg"},
			}}, Prev: []keybase.Prev{{ID: 2}}},
			{ID: 2, SentAtMs: 2000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "check this out"}}, Prev: []keybase.Prev{{ID: 1}}},
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hey everyone"}}},
		}
	case "team1":
		return []keybase.MsgSummary{
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "headline", Headline: &keybase.HeadlineContent{Headline: "Welcome"}}},
		}
	}
	return nil
}

type integrationClient struct{}

func (c *integrationClient) ReadConversation(convID string, known func(int) bool) ([]keybase.MsgSummary, error) {
	msgs := integrationMsgs(convID)
	var result []keybase.MsgSummary
	for _, m := range msgs {
		if known != nil && known(m.ID) {
			break
		}
		result = append(result, m)
	}
	return result, nil
}

func (c *integrationClient) DownloadAttachment(channel keybase.ChatChannel, msgID int, outPath string) error {
	return os.WriteFile(outPath, []byte(fmt.Sprintf("attachment-%s-%d", channel.Name, msgID)), 0644)
}

func (c *integrationClient) Close() error { return nil }

func TestIntegration_FullExport(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DestDir: dir, Parallel: 2, SelfUsername: "self"}
	lc := &mockListClient{convs: integrationConvs()}
	newClient := func() (ClientAPI, error) { return &integrationClient{}, nil }

	s, err := Run(cfg, lc, newClient)
	if err != nil {
		t.Fatal(err)
	}
	if s.Conversations != 3 {
		t.Errorf("conversations = %d, want 3", s.Conversations)
	}
	if s.Messages != 6 {
		t.Errorf("messages = %d, want 6", s.Messages)
	}
	if s.Attachments != 1 {
		t.Errorf("attachments = %d, want 1", s.Attachments)
	}

	// Verify DM directory
	dmDir := filepath.Join(dir, "Chats", "alice")
	if !MsgExists(dmDir, 1) || !MsgExists(dmDir, 2) {
		t.Error("DM messages missing")
	}

	// Verify group directory
	groupDir := filepath.Join(dir, "Chats", "alice,bob,charlie")
	if !MsgExists(groupDir, 1) || !MsgExists(groupDir, 2) || !MsgExists(groupDir, 3) {
		t.Error("group messages missing")
	}

	// Verify team directory
	teamDir := filepath.Join(dir, "Teams", "engineering", "general")
	if !MsgExists(teamDir, 1) {
		t.Error("team message missing")
	}
}

func TestIntegration_IncrementalExport(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DestDir: dir, Parallel: 1, SelfUsername: "self", SkipAttachments: true}
	lc := &mockListClient{convs: []keybase.ConvSummary{
		{ID: "dm1", Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"}},
	}}
	newClient := func() (ClientAPI, error) { return &integrationClient{}, nil }

	// First run
	s1, _ := Run(cfg, lc, newClient)
	if s1.Messages != 2 {
		t.Fatalf("first run: messages = %d, want 2", s1.Messages)
	}

	// Second run with new messages
	lc2 := &mockListClient{convs: lc.convs}
	newClient2 := func() (ClientAPI, error) {
		return &mockClient{msgs: []keybase.MsgSummary{
			{ID: 4, SentAtMs: 4000, Content: keybase.MsgContent{Type: "text"}, Prev: []keybase.Prev{{ID: 3}}},
			{ID: 3, SentAtMs: 3000, Content: keybase.MsgContent{Type: "text"}, Prev: []keybase.Prev{{ID: 2}}},
			{ID: 2, SentAtMs: 2000, Content: keybase.MsgContent{Type: "text"}}, // known from first run
		}}, nil
	}

	s2, _ := Run(cfg, lc2, newClient2)
	if s2.Messages != 2 {
		t.Errorf("second run: messages = %d, want 2 (only new)", s2.Messages)
	}

	// All 4 messages should exist
	dmDir := filepath.Join(dir, "Chats", "alice")
	for _, id := range []int{1, 2, 3, 4} {
		if !MsgExists(dmDir, id) {
			t.Errorf("message %d missing after incremental", id)
		}
	}
}

func TestIntegration_FilterExport(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DestDir: dir, Parallel: 1, SelfUsername: "self", SkipAttachments: true, Filters: []string{"Chat/alice"}}
	lc := &mockListClient{convs: integrationConvs()}
	newClient := func() (ClientAPI, error) { return &integrationClient{}, nil }

	s, _ := Run(cfg, lc, newClient)
	// Only the DM with alice should be exported (Chat/alice matches "self,alice")
	if s.Conversations != 1 {
		t.Errorf("conversations = %d, want 1", s.Conversations)
	}
	if s.Messages != 2 {
		t.Errorf("messages = %d, want 2", s.Messages)
	}
}

func TestIntegration_SkipAttachments(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DestDir: dir, Parallel: 1, SelfUsername: "self", SkipAttachments: true}
	lc := &mockListClient{convs: integrationConvs()}
	newClient := func() (ClientAPI, error) { return &integrationClient{}, nil }

	s, _ := Run(cfg, lc, newClient)
	if s.Attachments != 0 {
		t.Errorf("attachments = %d, want 0 with skip-attachments", s.Attachments)
	}

	// Verify no attachment files in the group conversation
	groupDir := filepath.Join(dir, "Chats", "alice,bob,charlie", "attachments")
	entries, _ := os.ReadDir(groupDir)
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("unexpected attachment file: %s", e.Name())
		}
	}
}
