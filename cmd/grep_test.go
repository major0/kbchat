package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
)

// ---------------------------------------------------------------------------
// Property-based tests (Task 1.3)
// ---------------------------------------------------------------------------

// TestPropertyPatternMatching verifies regex matching correctness.
func TestPropertyPatternMatching(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	t.Run("self_match", func(t *testing.T) {
		f := func(s string) bool {
			if s == "" {
				return true
			}
			matcher, err := compileMatcher(regexp.QuoteMeta(s), false)
			if err != nil {
				return false
			}
			return matcher(s)
		}
		if err := quick.Check(f, cfg); err != nil {
			t.Error(err)
		}
	})

	t.Run("case_insensitive", func(t *testing.T) {
		f := func(s string) bool {
			if s == "" {
				return true
			}
			matcher, err := compileMatcher(regexp.QuoteMeta(strings.ToUpper(s)), true)
			if err != nil {
				return false
			}
			return matcher(s)
		}
		if err := quick.Check(f, cfg); err != nil {
			t.Error(err)
		}
	})
}

// TestPropertyMatchableTypes verifies only text/edit/headline produce a body.
// Validates: Requirements 1.6.
func TestPropertyMatchableTypes(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	matchableTypes := map[string]bool{
		"text":     true,
		"edit":     true,
		"headline": true,
	}

	allTypes := []string{
		"text", "edit", "headline",
		"delete", "reaction", "attachment", "metadata", "system",
		"send_payment", "request_payment", "unfurl", "flip",
	}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		msgType := allTypes[rng.Intn(len(allTypes))]

		msg := keybase.MsgSummary{
			Content: keybase.MsgContent{Type: msgType},
		}

		// Populate the relevant content pointer for matchable types.
		switch msgType {
		case "text":
			msg.Content.Text = &keybase.TextContent{Body: "hello"}
		case "edit":
			msg.Content.Edit = &keybase.EditContent{Body: "edited"}
		case "headline":
			msg.Content.Headline = &keybase.HeadlineContent{Headline: "news"}
		}

		body, ok := msgBody(msg)

		if matchableTypes[msgType] {
			// Must produce a body.
			if !ok || body == "" {
				t.Logf("type %q should produce body, got ok=%v body=%q", msgType, ok, body)
				return false
			}
		} else {
			// Must not produce a body.
			if ok {
				t.Logf("type %q should not produce body, got ok=%v body=%q", msgType, ok, body)
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests (Task 1.4)
// ---------------------------------------------------------------------------

// TestMsgBody verifies msgBody for all message types and nil content pointers.
func TestMsgBody(t *testing.T) {
	tests := []struct {
		name     string
		msg      keybase.MsgSummary
		wantBody string
		wantOK   bool
	}{
		{
			name: "text returns body",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type: "text",
					Text: &keybase.TextContent{Body: "hello world"},
				},
			},
			wantBody: "hello world",
			wantOK:   true,
		},
		{
			name: "edit returns body",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type: "edit",
					Edit: &keybase.EditContent{Body: "edited text"},
				},
			},
			wantBody: "edited text",
			wantOK:   true,
		},
		{
			name: "headline returns body",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type:     "headline",
					Headline: &keybase.HeadlineContent{Headline: "breaking news"},
				},
			},
			wantBody: "breaking news",
			wantOK:   true,
		},
		{
			name: "reaction returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type:     "reaction",
					Reaction: &keybase.ReactionContent{Body: ":+1:"},
				},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "delete returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type:   "delete",
					Delete: &keybase.DeleteContent{MessageIDs: []int{1}},
				},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "attachment returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type:       "attachment",
					Attachment: &keybase.AttachmentContent{},
				},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "metadata returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{
					Type:     "metadata",
					Metadata: &keybase.MetadataContent{ConversationTitle: "title"},
				},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "system returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{Type: "system"},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "nil text pointer returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{Type: "text", Text: nil},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "nil edit pointer returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{Type: "edit", Edit: nil},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "nil headline pointer returns empty",
			msg: keybase.MsgSummary{
				Content: keybase.MsgContent{Type: "headline", Headline: nil},
			},
			wantBody: "",
			wantOK:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, ok := msgBody(tc.msg)
			if body != tc.wantBody || ok != tc.wantOK {
				t.Errorf("msgBody() = (%q, %v), want (%q, %v)", body, ok, tc.wantBody, tc.wantOK)
			}
		})
	}
}

// TestCompileMatcher verifies compileMatcher for regex and case modes.
func TestCompileMatcher(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		icase   bool
		input   string
		wantErr bool
		want    bool
	}{
		{
			name:    "literal substring match",
			pattern: "hello",
			input:   "say hello world",
			want:    true,
		},
		{
			name:    "no match",
			pattern: "xyz",
			input:   "hello world",
			want:    false,
		},
		{
			name:    "regex alternation",
			pattern: "error|fail",
			input:   "got an error",
			want:    true,
		},
		{
			name:    "regex quantifier",
			pattern: "hel+o",
			input:   "hello",
			want:    true,
		},
		{
			name:    "invalid regex returns error",
			pattern: "[invalid",
			wantErr: true,
		},
		{
			name:    "case sensitive no match",
			pattern: "HELLO",
			input:   "hello",
			want:    false,
		},
		{
			name:    "case insensitive match",
			pattern: "HELLO",
			icase:   true,
			input:   "hello",
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := compileMatcher(tc.pattern, tc.icase)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := matcher(tc.input)
			if got != tc.want {
				t.Errorf("matcher(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Property-based tests for contextWindows (Task 2.2)
// ---------------------------------------------------------------------------

// genMatchInput generates valid inputs for contextWindows property tests:
// sorted, unique match indices within [0, msgLen), and context values 0-5.
type genMatchInput struct {
	MatchIdxs []int
	MsgLen    int
	CtxB      int
	CtxA      int
}

// Generate implements quick.Generator for genMatchInput.
func (genMatchInput) Generate(rng *rand.Rand, _ int) reflect.Value {
	msgLen := rng.Intn(50) + 1 // 1..50
	ctxB := rng.Intn(6)        // 0..5
	ctxA := rng.Intn(6)        // 0..5

	// Generate a random subset of [0, msgLen) as sorted unique indices.
	count := rng.Intn(msgLen + 1) // 0..msgLen
	perm := rng.Perm(msgLen)
	chosen := perm[:count]
	sort.Ints(chosen)

	return reflect.ValueOf(genMatchInput{
		MatchIdxs: chosen,
		MsgLen:    msgLen,
		CtxB:      ctxB,
		CtxA:      ctxA,
	})
}

// TestPropertyContextWindows verifies context window correctness invariants.
// Validates: Requirements 3.4, 3.5, 3.6.
func TestPropertyContextWindows(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	f := func(input genMatchInput) bool {
		windows := contextWindows(input.MatchIdxs, input.MsgLen, input.CtxB, input.CtxA)

		if len(input.MatchIdxs) == 0 {
			return len(windows) == 0
		}

		// Build the set of indices that should be included: union of
		// [max(0, m-B), min(msgLen, m+1+A)) for each match m.
		expected := make(map[int]bool)
		for _, m := range input.MatchIdxs {
			lo := max(m-input.CtxB, 0)
			hi := min(m+1+input.CtxA, input.MsgLen)
			for i := lo; i < hi; i++ {
				expected[i] = true
			}
		}

		// Build the set of indices actually included by windows.
		actual := make(map[int]bool)
		for _, w := range windows {
			for i := w.Start; i < w.End; i++ {
				actual[i] = true
			}
		}

		// Every match index must be in exactly one window.
		for _, m := range input.MatchIdxs {
			if !actual[m] {
				t.Logf("match index %d not in any window", m)
				return false
			}
		}

		// Every expected index must be included.
		for idx := range expected {
			if !actual[idx] {
				t.Logf("expected index %d not in windows", idx)
				return false
			}
		}

		// No index outside expected ranges should be included.
		for idx := range actual {
			if !expected[idx] {
				t.Logf("unexpected index %d in windows", idx)
				return false
			}
		}

		// Windows must be sorted and non-overlapping.
		for i := 1; i < len(windows); i++ {
			if windows[i].Start < windows[i-1].End {
				t.Logf("windows overlap: [%d,%d) and [%d,%d)",
					windows[i-1].Start, windows[i-1].End,
					windows[i].Start, windows[i].End)
				return false
			}
		}

		return true
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestPropertyWindowSeparation verifies non-contiguous windows remain
// separate and overlapping/adjacent windows merge into one.
// Validates: Requirements 3.9.
func TestPropertyWindowSeparation(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	f := func(input genMatchInput) bool {
		windows := contextWindows(input.MatchIdxs, input.MsgLen, input.CtxB, input.CtxA)

		if len(input.MatchIdxs) == 0 {
			return len(windows) == 0
		}

		// Compute expanded ranges per match.
		type rng struct{ lo, hi int }
		ranges := make([]rng, len(input.MatchIdxs))
		for i, m := range input.MatchIdxs {
			ranges[i] = rng{
				lo: max(m-input.CtxB, 0),
				hi: min(m+1+input.CtxA, input.MsgLen),
			}
		}

		// Group matches by which window they belong to.
		windowIdx := func(m int) int {
			for wi, w := range windows {
				if m >= w.Start && m < w.End {
					return wi
				}
			}
			return -1
		}

		// Two matches whose expanded ranges overlap or are adjacent
		// must be in the same window.
		for i := range len(ranges) - 1 {
			if ranges[i].hi >= ranges[i+1].lo {
				wi := windowIdx(input.MatchIdxs[i])
				wj := windowIdx(input.MatchIdxs[i+1])
				if wi != wj {
					t.Logf("adjacent/overlapping matches %d and %d in different windows %d vs %d",
						input.MatchIdxs[i], input.MatchIdxs[i+1], wi, wj)
					return false
				}
			}
		}

		// Two matches whose expanded ranges are non-contiguous must be
		// in different windows.
		for i := range len(ranges) - 1 {
			if ranges[i].hi < ranges[i+1].lo {
				wi := windowIdx(input.MatchIdxs[i])
				wj := windowIdx(input.MatchIdxs[i+1])
				if wi == wj {
					t.Logf("non-contiguous matches %d and %d in same window %d",
						input.MatchIdxs[i], input.MatchIdxs[i+1], wi)
					return false
				}
			}
		}

		return true
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for contextWindows (Task 2.3)
// ---------------------------------------------------------------------------

// TestContextWindows verifies contextWindows with specific cases.
func TestContextWindows(t *testing.T) {
	tests := []struct {
		name      string
		matchIdxs []int
		msgLen    int
		ctxB      int
		ctxA      int
		want      []window
	}{
		{
			name:      "no matches empty input",
			matchIdxs: nil,
			msgLen:    10,
			want:      nil,
		},
		{
			name:      "single match no context",
			matchIdxs: []int{5},
			msgLen:    10,
			ctxB:      0,
			ctxA:      0,
			want:      []window{{Start: 5, End: 6}},
		},
		{
			name:      "single match with A=2",
			matchIdxs: []int{3},
			msgLen:    10,
			ctxB:      0,
			ctxA:      2,
			want:      []window{{Start: 3, End: 6}},
		},
		{
			name:      "single match with B=2",
			matchIdxs: []int{5},
			msgLen:    10,
			ctxB:      2,
			ctxA:      0,
			want:      []window{{Start: 3, End: 6}},
		},
		{
			name:      "two matches overlapping context merged",
			matchIdxs: []int{3, 5},
			msgLen:    10,
			ctxB:      1,
			ctxA:      1,
			want:      []window{{Start: 2, End: 7}},
		},
		{
			name:      "two matches non-contiguous",
			matchIdxs: []int{1, 8},
			msgLen:    10,
			ctxB:      0,
			ctxA:      0,
			want:      []window{{Start: 1, End: 2}, {Start: 8, End: 9}},
		},
		{
			name:      "match at index 0 with B=3 clamped",
			matchIdxs: []int{0},
			msgLen:    10,
			ctxB:      3,
			ctxA:      0,
			want:      []window{{Start: 0, End: 1}},
		},
		{
			name:      "match at last index with A=3 clamped",
			matchIdxs: []int{9},
			msgLen:    10,
			ctxB:      0,
			ctxA:      3,
			want:      []window{{Start: 9, End: 10}},
		},
		{
			name:      "C=2 equivalent both A=2 and B=2",
			matchIdxs: []int{5},
			msgLen:    10,
			ctxB:      2,
			ctxA:      2,
			want:      []window{{Start: 3, End: 8}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := contextWindows(tc.matchIdxs, tc.msgLen, tc.ctxB, tc.ctxA)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("contextWindows(%v, %d, %d, %d) = %v, want %v",
					tc.matchIdxs, tc.msgLen, tc.ctxB, tc.ctxA, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Property-based test for global count limit (Task 4.3)
// ---------------------------------------------------------------------------

// TestPropertyGlobalCountLimit verifies that --count K limits total match
// lines to at most K across all conversations.
// Validates: Requirements 2.6.
func TestPropertyGlobalCountLimit(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate 1-4 conversations with 1-10 messages each.
		numConvs := rng.Intn(4) + 1
		storeDir := t.TempDir()
		totalMatchable := 0

		for ci := range numConvs {
			convName := fmt.Sprintf("conv%d", ci)
			numMsgs := rng.Intn(10) + 1
			msgsDir := filepath.Join(storeDir, "Chats", convName, "messages")
			if err := os.MkdirAll(msgsDir, 0o755); err != nil {
				t.Logf("mkdir: %v", err)
				return false
			}
			for mi := range numMsgs {
				// All messages are text with body "match" so they all match.
				msg := keybase.MsgSummary{
					ID:     mi + 1,
					SentAt: int64(1000000000 + mi*60),
					Sender: keybase.MsgSender{Username: "user"},
					Content: keybase.MsgContent{
						Type: "text",
						Text: &keybase.TextContent{Body: "match"},
					},
				}
				msgDir := filepath.Join(msgsDir, strconv.Itoa(msg.ID))
				if err := os.MkdirAll(msgDir, 0o755); err != nil {
					t.Logf("mkdir: %v", err)
					return false
				}
				data, err := json.Marshal(msg)
				if err != nil {
					t.Logf("marshal: %v", err)
					return false
				}
				if err := os.WriteFile(filepath.Join(msgDir, "message.json"), data, 0o644); err != nil {
					t.Logf("write: %v", err)
					return false
				}
				totalMatchable++
			}
		}

		// Pick a count limit K between 1 and totalMatchable.
		k := rng.Intn(totalMatchable) + 1

		grepCfg := &config.Config{StorePath: storeDir}
		var buf bytes.Buffer
		args := []string{"--count", strconv.Itoa(k), "match"}
		err := runGrep(args, grepCfg, &buf, time.Now())
		if err != nil {
			t.Logf("runGrep error: %v", err)
			return false
		}

		// Count match lines: lines that aren't headers, separators, or blank.
		matchLines := 0
		for line := range strings.SplitSeq(buf.String(), "\n") {
			if line == "" || line == "--" || strings.HasPrefix(line, "==> ") {
				continue
			}
			matchLines++
		}

		if matchLines > k {
			t.Logf("matchLines=%d > k=%d", matchLines, k)
			return false
		}

		return true
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for parseGrepArgs (Task 4.4)
// ---------------------------------------------------------------------------

func TestParseGrepArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		want    *grepOpts
	}{
		{
			name:    "no args → error",
			args:    nil,
			wantErr: "missing required <pattern>",
		},
		{
			name: "pattern only",
			args: []string{"hello"},
			want: &grepOpts{Pattern: "hello"},
		},
		{
			name: "conversations + pattern",
			args: []string{"Chats/alice", "Teams/eng/*", "hello"},
			want: &grepOpts{
				Conversations: []string{"Chats/alice", "Teams/eng/*"},
				Pattern:       "hello",
			},
		},
		{
			name: "-i sets ICase",
			args: []string{"-i", "hello"},
			want: &grepOpts{Pattern: "hello", ICase: true},
		},
		{
			name: "-A 3",
			args: []string{"-A", "3", "hello"},
			want: &grepOpts{Pattern: "hello", CtxA: 3},
		},
		{
			name: "-B 2",
			args: []string{"-B", "2", "hello"},
			want: &grepOpts{Pattern: "hello", CtxB: 2},
		},
		{
			name: "-C 4 sets both",
			args: []string{"-C", "4", "hello"},
			want: &grepOpts{Pattern: "hello", CtxA: 4, CtxB: 4},
		},
		{
			name: "-C 4 -A 1 overrides CtxA",
			args: []string{"-C", "4", "-A", "1", "hello"},
			want: &grepOpts{Pattern: "hello", CtxA: 1, CtxB: 4},
		},
		{
			name: "--count 5",
			args: []string{"--count", "5", "hello"},
			want: &grepOpts{Pattern: "hello", Count: 5},
		},
		{
			name: "--after",
			args: []string{"--after", "2025-01-01", "hello"},
			want: &grepOpts{Pattern: "hello", After: "2025-01-01"},
		},
		{
			name: "--before",
			args: []string{"--before", "2025-12-31", "hello"},
			want: &grepOpts{Pattern: "hello", Before: "2025-12-31"},
		},
		{
			name: "--verbose",
			args: []string{"--verbose", "hello"},
			want: &grepOpts{Pattern: "hello", Verbose: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGrepArgs(tt.args)
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseGrepArgs(%v)\n  got  %+v\n  want %+v", tt.args, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for runGrep (Task 4.4)
// ---------------------------------------------------------------------------

func TestRunGrep(t *testing.T) {
	baseTime := int64(1718452800) // 2024-06-15 12:00:00 UTC
	now := time.Unix(baseTime+3600*24, 0)

	t.Run("single conversation match", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "hello world"),
				textMsg(2, baseTime+60, "bob", "goodbye"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"hello"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "==> Chats/alice,bob <==") {
			t.Errorf("missing header in output:\n%s", out)
		}
		if !strings.Contains(out, "hello world") {
			t.Errorf("missing matched message in output:\n%s", out)
		}
		if strings.Contains(out, "goodbye") {
			t.Errorf("non-matching message should not appear:\n%s", out)
		}
	})

	t.Run("multi-conversation with separator", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "match here"),
			},
			"alice,carol": {
				textMsg(1, baseTime, "carol", "match there"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"match"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "--\n") {
			t.Errorf("missing -- separator between conversations:\n%s", out)
		}
		// Both headers should appear.
		if !strings.Contains(out, "==> Chats/alice,bob <==") {
			t.Errorf("missing alice,bob header:\n%s", out)
		}
		if !strings.Contains(out, "==> Chats/alice,carol <==") {
			t.Errorf("missing alice,carol header:\n%s", out)
		}
	})

	t.Run("blank line between non-contiguous windows", func(t *testing.T) {
		// Messages 1,2,3,4,5 — matches at 1 and 5 with no context → two windows.
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "alpha target"),
				textMsg(2, baseTime+60, "bob", "filler one"),
				textMsg(3, baseTime+120, "alice", "filler two"),
				textMsg(4, baseTime+180, "bob", "filler three"),
				textMsg(5, baseTime+240, "alice", "omega target"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"target"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		lines := strings.Split(out, "\n")
		// Expect: header, match1, blank, match5, trailing empty
		foundBlank := false
		for i, line := range lines {
			if line == "" && i > 0 && i < len(lines)-1 {
				foundBlank = true
				break
			}
		}
		if !foundBlank {
			t.Errorf("expected blank line between non-contiguous windows:\n%s", out)
		}
	})

	t.Run("verbose mode", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(42, baseTime, "alice", "verbose test"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"--verbose", "verbose"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "[id=42]") {
			t.Errorf("verbose output missing [id=42]:\n%s", out)
		}
		if !strings.Contains(out, "(phone)") {
			t.Errorf("verbose output missing (phone):\n%s", out)
		}
	})

	t.Run("count 3 across 2 conversations", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "match 1"),
				textMsg(2, baseTime+60, "alice", "match 2"),
			},
			"alice,carol": {
				textMsg(1, baseTime, "carol", "match 3"),
				textMsg(2, baseTime+60, "carol", "match 4"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"--count", "3", "match"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		// Count match lines (not headers, separators, or blank lines).
		matchLines := 0
		for line := range strings.SplitSeq(buf.String(), "\n") {
			if line == "" || line == "--" || strings.HasPrefix(line, "==> ") {
				continue
			}
			matchLines++
		}
		if matchLines != 3 {
			t.Errorf("expected 3 match lines, got %d\noutput:\n%s", matchLines, buf.String())
		}
	})

	t.Run("count 0 unlimited", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "match a"),
				textMsg(2, baseTime+60, "alice", "match b"),
				textMsg(3, baseTime+120, "alice", "match c"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"--count", "0", "match"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		matchLines := 0
		for line := range strings.SplitSeq(buf.String(), "\n") {
			if line == "" || line == "--" || strings.HasPrefix(line, "==> ") {
				continue
			}
			matchLines++
		}
		if matchLines != 3 {
			t.Errorf("expected 3 match lines (unlimited), got %d\noutput:\n%s", matchLines, buf.String())
		}
	})

	t.Run("after/before timestamp filtering", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "match early"),
				textMsg(2, baseTime+3600, "alice", "match mid"),
				textMsg(3, baseTime+7200, "alice", "match late"),
			},
		})
		// Only match messages between baseTime+1800 and baseTime+5400.
		afterTS := time.Unix(baseTime+1800, 0).Format(time.RFC3339)
		beforeTS := time.Unix(baseTime+5400, 0).Format(time.RFC3339)
		var buf bytes.Buffer
		err := runGrep([]string{"--after", afterTS, "--before", beforeTS, "match"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "match mid") {
			t.Errorf("expected 'match mid' in output:\n%s", out)
		}
		if strings.Contains(out, "match early") {
			t.Errorf("'match early' should be filtered out:\n%s", out)
		}
		if strings.Contains(out, "match late") {
			t.Errorf("'match late' should be filtered out:\n%s", out)
		}
	})

	t.Run("regex substring match", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "we deployed today"),
				textMsg(2, baseTime+60, "bob", "sounds good"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"deploy"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "we deployed today") {
			t.Errorf("substring 'deploy' should match 'we deployed today':\n%s", out)
		}
		if strings.Contains(out, "sounds good") {
			t.Errorf("'sounds good' should not match:\n%s", out)
		}
	})

	t.Run("substring match", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "got an error"),
				textMsg(2, baseTime+60, "bob", "err happened"),
				textMsg(3, baseTime+120, "alice", "all good"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"err"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "got an error") {
			t.Errorf("should match 'got an error':\n%s", out)
		}
		if !strings.Contains(out, "err happened") {
			t.Errorf("should match 'err happened':\n%s", out)
		}
		if strings.Contains(out, "all good") {
			t.Errorf("'all good' should not match:\n%s", out)
		}
	})

	t.Run("no matches → empty output", func(t *testing.T) {
		store := makeTestStore(t, map[string][]keybase.MsgSummary{
			"alice,bob": {
				textMsg(1, baseTime, "alice", "hello"),
				textMsg(2, baseTime+60, "bob", "world"),
			},
		})
		var buf bytes.Buffer
		err := runGrep([]string{"zzzzz"}, &config.Config{StorePath: store}, &buf, now)
		if err != nil {
			t.Fatal(err)
		}
		if buf.String() != "" {
			t.Errorf("expected empty output for no matches, got:\n%s", buf.String())
		}
	})
}
