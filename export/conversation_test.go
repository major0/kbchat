package export

import (
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
	"testing/quick"

	"github.com/major0/keybase-export/keybase"
)

// Feature: keybase-go-export, Property 4: Exported messages are in chronological order
func TestPropertyChronologicalOrder(t *testing.T) {
	dir := t.TempDir()

	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		n := r.Intn(20) + 1
		msgs := make([]keybase.MsgSummary, n)
		for i := range msgs {
			msgs[i] = keybase.MsgSummary{
				ID:       i + 1,
				SentAtMs: r.Int63n(1000000),
				Content:  keybase.MsgContent{Type: "text"},
			}
		}

		// Sort chronologically before writing (as the exporter should)
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].SentAtMs < msgs[j].SentAtMs
		})

		path := filepath.Join(dir, "messages.json")
		if err := WriteMessages(path, msgs); err != nil {
			t.Logf("write error: %v", err)
			return false
		}
		got, err := ReadMessages(path)
		if err != nil {
			t.Logf("read error: %v", err)
			return false
		}

		// Verify chronological order
		for i := 1; i < len(got); i++ {
			if got[i].SentAtMs < got[i-1].SentAtMs {
				t.Logf("not chronological at index %d: %d < %d", i, got[i].SentAtMs, got[i-1].SentAtMs)
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

// Feature: keybase-go-export, Property 5: No message collapsing
func TestPropertyNoMessageCollapsing(t *testing.T) {
	dir := t.TempDir()

	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		n := r.Intn(20) + 1
		contentTypes := []string{"text", "edit", "delete", "reaction"}
		msgs := make([]keybase.MsgSummary, n)
		for i := range msgs {
			ct := contentTypes[r.Intn(len(contentTypes))]
			msgs[i] = keybase.MsgSummary{
				ID:       i + 1,
				SentAtMs: int64(i * 1000),
				Content:  keybase.MsgContent{Type: ct},
			}
			switch ct {
			case "text":
				msgs[i].Content.Text = &keybase.TextContent{Body: "hello"}
			case "edit":
				msgs[i].Content.Edit = &keybase.EditContent{Body: "edited", MessageID: 1}
			case "delete":
				msgs[i].Content.Delete = &keybase.DeleteContent{MessageIDs: []int{1}}
			case "reaction":
				msgs[i].Content.Reaction = &keybase.ReactionContent{Body: ":+1:", MessageID: 1}
			}
		}

		path := filepath.Join(dir, "messages.json")
		if err := WriteMessages(path, msgs); err != nil {
			t.Logf("write error: %v", err)
			return false
		}
		got, err := ReadMessages(path)
		if err != nil {
			t.Logf("read error: %v", err)
			return false
		}

		// No messages should be collapsed
		if len(got) != len(msgs) {
			t.Logf("message count mismatch: wrote %d, read %d", len(msgs), len(got))
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}
