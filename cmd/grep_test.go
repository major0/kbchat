package cmd

import (
	"math/rand"
	"regexp"
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
