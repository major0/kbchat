package export

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/major0/kbchat/keybase"
)

// Feature: keybase-go-export, Property 4: Message storage preserves all fields.
func TestPropertyMessageStorageRoundTrip(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		dir := t.TempDir()

		msg := keybase.MsgSummary{
			ID:       r.Intn(100000) + 1,
			SentAtMs: r.Int63(),
			Content:  keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello"}},
			Sender:   keybase.MsgSender{UID: "uid1", Username: "alice"},
			Prev:     []keybase.Prev{{ID: r.Intn(1000), Hash: "abc"}},
		}

		if err := WriteMsg(dir, msg); err != nil {
			t.Logf("write error: %v", err)
			return false
		}
		got, err := ReadMsg(dir, msg.ID)
		if err != nil {
			t.Logf("read error: %v", err)
			return false
		}
		if !reflect.DeepEqual(msg, *got) {
			t.Logf("round-trip mismatch for msg %d", msg.ID)
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

// Feature: keybase-go-export, Property 5: No message collapsing.
func TestPropertyNoMessageCollapsing(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		dir := t.TempDir()
		n := r.Intn(20) + 1
		contentTypes := []string{"text", "edit", "delete", "reaction"}

		for i := range n {
			ct := contentTypes[r.Intn(len(contentTypes))]
			msg := keybase.MsgSummary{
				ID:       i + 1,
				SentAtMs: int64(i * 1000),
				Content:  keybase.MsgContent{Type: ct},
			}
			switch ct {
			case "text":
				msg.Content.Text = &keybase.TextContent{Body: "hello"}
			case "edit":
				msg.Content.Edit = &keybase.EditContent{Body: "edited", MessageID: 1}
			case "delete":
				msg.Content.Delete = &keybase.DeleteContent{MessageIDs: []int{1}}
			case "reaction":
				msg.Content.Reaction = &keybase.ReactionContent{Body: ":+1:", MessageID: 1}
			}
			if err := WriteMsg(dir, msg); err != nil {
				t.Logf("write error: %v", err)
				return false
			}
		}

		// Every message should exist as its own directory
		for i := range n {
			if !MsgExists(dir, i+1) {
				t.Logf("message %d missing", i+1)
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}
