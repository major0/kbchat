package cmd

import (
	"math/rand"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/quick"

	"github.com/major0/kbchat/keybase"
)

// ---------------------------------------------------------------------------
// Property-based tests (Task 1.3)
// ---------------------------------------------------------------------------

// TestPropertyPatternMatching verifies pattern matching correctness.
// Validates: Requirements 1.3, 1.4, 1.5.
func TestPropertyPatternMatching(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	// Sub-property: glob literal self-match.
	// Any non-empty string with glob metacharacters escaped must match itself.
	t.Run("glob_literal_self_match", func(t *testing.T) {
		f := func(s string) bool {
			if s == "" {
				return true // skip empty
			}
			// Escape glob metacharacters so the pattern is a literal.
			escaped := escapeGlobLiteral(s)
			matcher, err := compileMatcher(escaped, false, false)
			if err != nil {
				t.Logf("compileMatcher error for %q: %v", escaped, err)
				return false
			}
			return matcher(s)
		}
		if err := quick.Check(f, cfg); err != nil {
			t.Error(err)
		}
	})

	// Sub-property: case-insensitive matching.
	// Upper-cased pattern must match original string with icase=true.
	t.Run("case_insensitive", func(t *testing.T) {
		f := func(s string) bool {
			if s == "" {
				return true
			}
			escaped := escapeGlobLiteral(s)
			upper := strings.ToUpper(escaped)
			matcher, err := compileMatcher(upper, false, true)
			if err != nil {
				return false
			}
			return matcher(s)
		}
		if err := quick.Check(f, cfg); err != nil {
			t.Error(err)
		}
	})

	// Sub-property: regexp mode consistency.
	// A regexp that matches case-sensitively also matches with icase=true.
	t.Run("regexp_mode_consistency", func(t *testing.T) {
		f := func(s string) bool {
			if s == "" {
				return true
			}
			// Use QuoteMeta to build a valid regexp literal from s.
			pat := regexp.QuoteMeta(s)
			matcherCS, err := compileMatcher(pat, true, false)
			if err != nil {
				return false
			}
			matcherCI, err := compileMatcher(pat, true, true)
			if err != nil {
				return false
			}
			// If case-sensitive matches, case-insensitive must also match.
			if matcherCS(s) && !matcherCI(s) {
				return false
			}
			return true
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

// escapeGlobLiteral escapes glob metacharacters (* and ?) so the string
// is treated as a literal in glob mode.
func escapeGlobLiteral(s string) string {
	// In glob mode, * and ? are special. We escape them by wrapping
	// in a character class: * → [*], ? → [?].
	s = strings.ReplaceAll(s, `[`, `[[]`)
	s = strings.ReplaceAll(s, `*`, `[*]`)
	s = strings.ReplaceAll(s, `?`, `[?]`)
	return s
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

// TestCompileMatcher verifies compileMatcher for glob, regexp, and case modes.
func TestCompileMatcher(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		isRegexp bool
		icase    bool
		input    string
		wantErr  bool
		want     bool
	}{
		{
			name:    "glob star matches any string",
			pattern: "*",
			input:   "anything at all",
			want:    true,
		},
		{
			name:    "glob question matches single char",
			pattern: "h?llo",
			input:   "hello",
			want:    true,
		},
		{
			name:    "glob question rejects zero chars",
			pattern: "h?llo",
			input:   "hllo",
			want:    false,
		},
		{
			name:    "glob literal match",
			pattern: "exact",
			input:   "exact",
			want:    true,
		},
		{
			name:    "glob no match",
			pattern: "exact",
			input:   "other",
			want:    false,
		},
		{
			name:     "regexp valid pattern matches",
			pattern:  "hel+o",
			isRegexp: true,
			input:    "hello",
			want:     true,
		},
		{
			name:     "regexp invalid pattern returns error",
			pattern:  "[invalid",
			isRegexp: true,
			wantErr:  true,
		},
		{
			name:    "icase glob matches",
			pattern: "HELLO",
			icase:   true,
			input:   "hello",
			want:    true,
		},
		{
			name:     "icase regexp matches",
			pattern:  "HELLO",
			isRegexp: true,
			icase:    true,
			input:    "hello",
			want:     true,
		},
		{
			name:    "glob is anchored (full body match)",
			pattern: "hello",
			input:   "say hello world",
			want:    false,
		},
		{
			name:     "regexp is unanchored (substring match)",
			pattern:  "hello",
			isRegexp: true,
			input:    "say hello world",
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := compileMatcher(tc.pattern, tc.isRegexp, tc.icase)
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
