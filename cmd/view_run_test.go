package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
)

// captureRunView calls runView and captures output.
// Returns the output string and any error.
func captureRunView(t *testing.T, args []string, storePath string, now time.Time) (string, error) {
	t.Helper()
	cfg := &config.Config{StorePath: storePath}

	var buf bytes.Buffer
	err := runView(args, cfg, &buf, now)
	return buf.String(), err
}

func TestRunView(t *testing.T) {
	// Base time: 2024-06-15 12:00:00 UTC (Unix: 1718452800)
	baseTime := int64(1718452800)
	now := time.Unix(baseTime+30*60, 0) // 30 min after first message

	// Create a store with 25 messages for the main conversation.
	storePath := makeTestStoreOneConv(t, textMsgs(25, baseTime))

	// Create a store with multiple conversations for multi-match tests.
	multiStore := makeTestStore(t, map[string][]keybase.MsgSummary{
		"alice,bob":   textMsgs(5, baseTime),
		"alice,carol": textMsgs(5, baseTime),
	})

	// Create an empty store for zero-match tests.
	emptyStore := t.TempDir()
	if err := os.MkdirAll(filepath.Join(emptyStore, "Chats"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		args      []string
		store     string
		now       time.Time
		wantErr   string // substring of error message; empty = no error
		wantLines int    // expected number of output lines; -1 = don't check
		wantSub   string // substring that must appear in output; empty = don't check
	}{
		{
			name:    "no argument → error",
			args:    nil,
			store:   storePath,
			now:     now,
			wantErr: "missing required <conversation>",
		},
		{
			name:    "zero matches → error",
			args:    []string{"Chats/nonexistent"},
			store:   emptyStore,
			now:     now,
			wantErr: "no matching conversations",
		},
		{
			name:      "multiple matches → multi-conversation output",
			args:      []string{"Chats/*alice*"},
			store:     multiStore,
			now:       now,
			wantLines: -1,
			wantSub:   "==> Chats/alice,bob <==",
		},
		{
			name:      "valid filter → default 20 messages",
			args:      []string{"Chats/alice,bob"},
			store:     storePath,
			now:       now,
			wantLines: 20,
		},
		{
			name:      "--count 5",
			args:      []string{"--count", "5", "Chats/alice,bob"},
			store:     storePath,
			now:       now,
			wantLines: 5,
		},
		{
			name:      "--count 0 (all messages)",
			args:      []string{"--count", "0", "Chats/alice,bob"},
			store:     storePath,
			now:       now,
			wantLines: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := captureRunView(t, tt.args, tt.store, tt.now)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			if output == "" {
				lines = nil
			}

			if tt.wantLines >= 0 && len(lines) != tt.wantLines {
				t.Errorf("got %d lines, want %d", len(lines), tt.wantLines)
			}
			if tt.wantSub != "" && !strings.Contains(output, tt.wantSub) {
				t.Errorf("output does not contain %q:\n%s", tt.wantSub, output)
			}
		})
	}
}

func TestRunViewTimestampFilters(t *testing.T) {
	// 2024-06-15 00:00:00 UTC
	dayStart := int64(1718409600)
	// Create 24 messages, one per hour across the day.
	msgs := make([]keybase.MsgSummary, 24)
	for i := range msgs {
		msgs[i] = keybase.MsgSummary{
			ID:     i + 1,
			SentAt: dayStart + int64(i*3600),
			Sender: keybase.MsgSender{Username: "alice", DeviceName: "laptop"},
			Content: keybase.MsgContent{
				Type: "text",
				Text: &keybase.TextContent{Body: fmt.Sprintf("hour %d", i)},
			},
		}
	}
	storePath := makeTestStoreOneConv(t, msgs)
	// now = end of day + 1 hour
	now := time.Unix(dayStart+25*3600, 0)

	tests := []struct {
		name      string
		args      []string
		wantLines int
	}{
		{
			name:      "--date 2024-06-15 (all messages from that day)",
			args:      []string{"--date", "2024-06-15", "Chats/alice,bob"},
			wantLines: 24,
		},
		{
			name: "--after (first 20 after hour 10)",
			args: []string{
				"--after", time.Unix(dayStart+10*3600, 0).Format(time.RFC3339),
				"Chats/alice,bob",
			},
			wantLines: 14, // hours 10..23 = 14 messages, but default count=20 so all 14
		},
		{
			name: "--before (last 20 before hour 12)",
			args: []string{
				"--before", time.Unix(dayStart+12*3600, 0).Format(time.RFC3339),
				"Chats/alice,bob",
			},
			wantLines: 12, // hours 0..11 = 12 messages, count=20 so all 12
		},
		{
			name: "--after + --before (range mode, all in range)",
			args: []string{
				"--after", time.Unix(dayStart+5*3600, 0).Format(time.RFC3339),
				"--before", time.Unix(dayStart+15*3600, 0).Format(time.RFC3339),
				"Chats/alice,bob",
			},
			wantLines: 10, // hours 5..14 = 10 messages
		},
		{
			name: "--after with --count 3",
			args: []string{
				"--after", time.Unix(dayStart+10*3600, 0).Format(time.RFC3339),
				"--count", "3",
				"Chats/alice,bob",
			},
			wantLines: 3, // first 3 after hour 10
		},
		{
			name: "--before with --count 3",
			args: []string{
				"--before", time.Unix(dayStart+12*3600, 0).Format(time.RFC3339),
				"--count", "3",
				"Chats/alice,bob",
			},
			wantLines: 3, // last 3 before hour 12
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := captureRunView(t, tt.args, storePath, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			if output == "" {
				lines = nil
			}
			if len(lines) != tt.wantLines {
				t.Errorf("got %d lines, want %d\noutput:\n%s", len(lines), tt.wantLines, output)
			}
		})
	}
}

func TestRunViewVerbose(t *testing.T) {
	baseTime := int64(1718452800)
	msgs := []keybase.MsgSummary{{
		ID:     1,
		SentAt: baseTime,
		Sender: keybase.MsgSender{Username: "alice", DeviceName: "laptop"},
		Content: keybase.MsgContent{
			Type: "text",
			Text: &keybase.TextContent{Body: "hello"},
		},
	}}
	storePath := makeTestStoreOneConv(t, msgs)
	now := time.Unix(baseTime+3600, 0)

	output, err := captureRunView(t, []string{"--verbose", "Chats/alice,bob"}, storePath, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "[id=1]") {
		t.Errorf("verbose output missing [id=1]: %s", output)
	}
	if !strings.Contains(output, "(laptop)") {
		t.Errorf("verbose output missing (laptop): %s", output)
	}
}

func TestRunViewMultiConversation(t *testing.T) {
	baseTime := int64(1718452800)
	now := time.Unix(baseTime+3600, 0)

	multiStore := makeTestStore(t, map[string][]keybase.MsgSummary{
		"alice,bob": {
			{ID: 1, SentAt: baseTime, Sender: keybase.MsgSender{Username: "alice"},
				Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello from bob chat"}}},
		},
		"alice,carol": {
			{ID: 1, SentAt: baseTime, Sender: keybase.MsgSender{Username: "carol"},
				Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello from carol chat"}}},
		},
	})

	t.Run("glob matching multiple conversations shows headers and separator", func(t *testing.T) {
		output, err := captureRunView(t, []string{"Chats/*alice*"}, multiStore, now)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output, "==> Chats/alice,bob <==") {
			t.Errorf("missing alice,bob header:\n%s", output)
		}
		if !strings.Contains(output, "==> Chats/alice,carol <==") {
			t.Errorf("missing alice,carol header:\n%s", output)
		}
		if !strings.Contains(output, "--\n") {
			t.Errorf("missing -- separator between conversations:\n%s", output)
		}
	})

	t.Run("single match has no header", func(t *testing.T) {
		output, err := captureRunView(t, []string{"Chats/alice,bob"}, multiStore, now)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output, "==>") {
			t.Errorf("single conversation should not have header:\n%s", output)
		}
		if strings.Contains(output, "--") {
			t.Errorf("single conversation should not have separator:\n%s", output)
		}
	})

	t.Run("multiple filter args", func(t *testing.T) {
		output, err := captureRunView(t, []string{"Chats/alice,bob", "Chats/alice,carol"}, multiStore, now)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output, "==> Chats/alice,bob <==") {
			t.Errorf("missing alice,bob header:\n%s", output)
		}
		if !strings.Contains(output, "==> Chats/alice,carol <==") {
			t.Errorf("missing alice,carol header:\n%s", output)
		}
	})
}

func TestRunViewMessageFormats(t *testing.T) {
	baseTime := int64(1718452800)
	msgs := []keybase.MsgSummary{
		{
			ID: 1, SentAt: baseTime,
			Sender:  keybase.MsgSender{Username: "alice"},
			Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello world"}},
		},
		{
			ID: 2, SentAt: baseTime + 60,
			Sender:  keybase.MsgSender{Username: "bob"},
			Content: keybase.MsgContent{Type: "edit", Edit: &keybase.EditContent{Body: "edited text"}},
		},
		{
			ID: 3, SentAt: baseTime + 120,
			Sender:  keybase.MsgSender{Username: "carol"},
			Content: keybase.MsgContent{Type: "reaction", Reaction: &keybase.ReactionContent{Body: "👍"}},
		},
		{
			ID: 4, SentAt: baseTime + 180,
			Sender: keybase.MsgSender{Username: "dave"},
			Content: keybase.MsgContent{
				Type:       "attachment",
				Attachment: &keybase.AttachmentContent{Object: keybase.AttachmentObject{Filename: "photo.jpg"}},
			},
		},
		{
			ID: 5, SentAt: baseTime + 240,
			Sender:  keybase.MsgSender{Username: "eve"},
			Content: keybase.MsgContent{Type: "delete", Delete: &keybase.DeleteContent{MessageIDs: []int{1}}},
		},
		{
			ID: 6, SentAt: baseTime + 300,
			Sender:  keybase.MsgSender{Username: "frank"},
			Content: keybase.MsgContent{Type: "headline", Headline: &keybase.HeadlineContent{Headline: "New topic"}},
		},
		{
			ID: 7, SentAt: baseTime + 360,
			Sender:  keybase.MsgSender{Username: "grace"},
			Content: keybase.MsgContent{Type: "metadata", Metadata: &keybase.MetadataContent{ConversationTitle: "Project X"}},
		},
		{
			ID: 8, SentAt: baseTime + 420,
			Sender:  keybase.MsgSender{Username: "heidi"},
			Content: keybase.MsgContent{Type: "system"},
		},
	}
	storePath := makeTestStoreOneConv(t, msgs)
	now := time.Unix(baseTime+3600, 0)

	output, err := captureRunView(t, []string{"--count", "0", "Chats/alice,bob"}, storePath, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		desc string
		sub  string
	}{
		{"text message", "<alice> hello world"},
		{"edit message", "* bob edit: edited text"},
		{"reaction message", "* carol reaction: 👍"},
		{"attachment message", "* dave attachment: photo.jpg"},
		{"delete message", "* eve delete: deleted message 1"},
		{"headline message", "* frank headline: New topic"},
		{"metadata message", "* grace metadata: Project X"},
		{"system message", "* heidi system: (no summary)"},
	}

	for _, c := range checks {
		if !strings.Contains(output, c.sub) {
			t.Errorf("%s: output missing %q\nfull output:\n%s", c.desc, c.sub, output)
		}
	}
}
