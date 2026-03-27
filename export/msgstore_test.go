package export

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"

	"github.com/major0/keybase-export/keybase"
)

// Feature: keybase-go-export, Property 7: Incremental export detects chain gaps and records orphans
func TestPropertyOrphanDetection(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		dir := t.TempDir()

		// Create some "existing" messages (IDs 1..n)
		existing := r.Intn(5) + 1
		for i := 1; i <= existing; i++ {
			msg := keybase.MsgSummary{ID: i, Content: keybase.MsgContent{Type: "text"}}
			if err := WriteMsg(dir, msg); err != nil {
				t.Logf("write existing msg: %v", err)
				return false
			}
		}

		// New message with prev pointers — some exist, some don't
		newID := existing + r.Intn(10) + 1
		var prevs []keybase.Prev
		numPrevs := r.Intn(4) + 1
		for i := 0; i < numPrevs; i++ {
			prevs = append(prevs, keybase.Prev{
				ID:   r.Intn(newID) + 1,
				Hash: "hash",
			})
		}

		// Detect orphans: prev IDs that don't exist locally
		var orphans []keybase.Prev
		for _, p := range prevs {
			if !MsgExists(dir, p.ID) {
				orphans = append(orphans, p)
			}
		}

		if len(orphans) == 0 {
			return true // no orphans to write
		}

		// Write the new message so its directory exists
		newMsg := keybase.MsgSummary{ID: newID, Content: keybase.MsgContent{Type: "text"}}
		if err := WriteMsg(dir, newMsg); err != nil {
			t.Logf("write new msg: %v", err)
			return false
		}

		if err := WriteOrphans(dir, newID, orphans); err != nil {
			t.Logf("write orphans: %v", err)
			return false
		}

		got, err := ReadOrphans(dir, newID)
		if err != nil {
			t.Logf("read orphans: %v", err)
			return false
		}
		if len(got) != len(orphans) {
			t.Logf("orphan count: got %d, want %d", len(got), len(orphans))
			return false
		}
		for i := range orphans {
			if got[i].ID != orphans[i].ID {
				t.Logf("orphan[%d].ID: got %d, want %d", i, got[i].ID, orphans[i].ID)
				return false
			}
		}

		// Existing messages must not be modified
		for i := 1; i <= existing; i++ {
			if !MsgExists(dir, i) {
				t.Logf("existing message %d was deleted", i)
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestMsgExists_Missing(t *testing.T) {
	dir := t.TempDir()
	if MsgExists(dir, 999) {
		t.Error("expected false for non-existent message")
	}
}

func TestMsgExists_Present(t *testing.T) {
	dir := t.TempDir()
	msg := keybase.MsgSummary{ID: 42, Content: keybase.MsgContent{Type: "text"}}
	if err := WriteMsg(dir, msg); err != nil {
		t.Fatal(err)
	}
	if !MsgExists(dir, 42) {
		t.Error("expected true for existing message")
	}
}

func TestReadHead_Missing(t *testing.T) {
	dir := t.TempDir()
	head, err := ReadHead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if head != 0 {
		t.Errorf("got %d, want 0 for missing head", head)
	}
}

func TestReadHead_Corrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "head"), []byte("not-a-number\n"), 0644); err != nil {
		t.Fatal(err)
	}
	head, err := ReadHead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if head != 0 {
		t.Errorf("got %d, want 0 for corrupt head", head)
	}
}

func TestWriteHead_Overwrite(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHead(dir, 10); err != nil {
		t.Fatal(err)
	}
	if err := WriteHead(dir, 20); err != nil {
		t.Fatal(err)
	}
	head, err := ReadHead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if head != 20 {
		t.Errorf("got %d, want 20", head)
	}
}

func TestReadOrphans_Missing(t *testing.T) {
	dir := t.TempDir()
	orphans, err := ReadOrphans(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if orphans != nil {
		t.Errorf("expected nil for missing orphans, got %v", orphans)
	}
}

func TestWriteMsg_ReadMsg_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	msg := keybase.MsgSummary{
		ID:       7,
		SentAtMs: 12345,
		Content:  keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "test"}},
		Sender:   keybase.MsgSender{UID: "u1", Username: "alice", DeviceID: "d1"},
		Prev:     []keybase.Prev{{ID: 6, Hash: "h6"}},
	}
	if err := WriteMsg(dir, msg); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMsg(dir, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != msg.ID || got.SentAtMs != msg.SentAtMs || got.Content.Type != msg.Content.Type {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestReadMsg_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadMsg(dir, 999)
	if err == nil {
		t.Error("expected error for missing message")
	}
}

func TestWriteMsgAttachments_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	msg := keybase.MsgSummary{ID: 5, Content: keybase.MsgContent{Type: "text"}}
	if err := WriteMsg(dir, msg); err != nil {
		t.Fatal(err)
	}
	refs := []AttachmentRef{
		{Filename: "a.jpg", StorageRef: "abc.jpg"},
		{Filename: "b.pdf", StorageRef: "def.pdf"},
	}
	if err := WriteMsgAttachments(dir, 5, refs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "messages", "5", "attachments.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("attachments.json is empty")
	}
}
