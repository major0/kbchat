package cmd

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/store"
)

// Feature: keybase-chat-list, Property 2: Format string round-trip.
//
// Parsing a format string into tokens and formatting a conversation with
// known field values produces a string where %t → type, %n → name,
// %c → count, %% → literal %. No token lost or duplicated.
//
// **Validates: Requirements 1.11**

// formatRoundTripInput holds a randomly generated format string and ConvInfo.
type formatRoundTripInput struct {
	Format string
	Conv   store.ConvInfo
}

// Generate implements quick.Generator for formatRoundTripInput.
func (formatRoundTripInput) Generate(r *rand.Rand, size int) reflect.Value {
	types := []string{"Chat", "Team"}
	names := []string{"alice", "bob", "alice,bob", "engineering"}
	channels := []string{"", "general", "random"}

	conv := store.ConvInfo{
		Type:     types[r.Intn(len(types))],
		Name:     names[r.Intn(len(names))],
		Channel:  channels[r.Intn(len(channels))],
		MsgCount: r.Intn(500),
	}

	// Build a random format string from known tokens and literal text.
	tokens := []string{"%t", "%n", "%c", "%%"}
	literals := []string{" ", "-", "/", ":", "hello", ""}

	var b strings.Builder
	n := 1 + r.Intn(8)
	for range n {
		if r.Intn(2) == 0 {
			b.WriteString(tokens[r.Intn(len(tokens))])
		} else {
			b.WriteString(literals[r.Intn(len(literals))])
		}
	}

	return reflect.ValueOf(formatRoundTripInput{
		Format: b.String(),
		Conv:   conv,
	})
}

func TestPropertyFormatStringRoundTrip(t *testing.T) {
	const timeFmt = "2006-01-02 15:04:05"

	f := func(input formatRoundTripInput) bool {
		tokens := parseFormatString(input.Format)
		result := formatConv(tokens, input.Conv, timeFmt)

		// Count expected occurrences of each field value in the format string.
		// Each %t should produce conv.Type, %n → convName, %c → count, %% → literal %.
		wantType := 0
		wantName := 0
		wantCount := 0
		wantPercent := 0

		for _, tok := range tokens {
			switch tok.Kind {
			case tokenType:
				wantType++
			case tokenName:
				wantName++
			case tokenCount:
				wantCount++
			case tokenPercent:
				wantPercent++
			}
		}

		typeStr := input.Conv.Type
		nameStr := convName(input.Conv)
		countStr := strconv.Itoa(input.Conv.MsgCount)

		// Verify each field value appears the expected number of times.
		if wantType > 0 && !strings.Contains(result, typeStr) {
			t.Logf("format=%q result=%q missing type=%q", input.Format, result, typeStr)
			return false
		}
		if wantName > 0 && !strings.Contains(result, nameStr) {
			t.Logf("format=%q result=%q missing name=%q", input.Format, result, nameStr)
			return false
		}
		if wantCount > 0 && !strings.Contains(result, countStr) {
			t.Logf("format=%q result=%q missing count=%q", input.Format, result, countStr)
			return false
		}
		if wantPercent > 0 && !strings.Contains(result, "%") {
			t.Logf("format=%q result=%q missing literal %%", input.Format, result)
			return false
		}

		// Verify no token is lost: the number of field substitutions in the
		// output must account for all tokens. We reconstruct what the output
		// should be by walking the tokens ourselves and compare.
		var expected strings.Builder
		for _, tok := range tokens {
			switch tok.Kind {
			case tokenLiteral:
				expected.WriteString(tok.Literal)
			case tokenPercent:
				expected.WriteByte('%')
			case tokenType:
				expected.WriteString(typeStr)
			case tokenName:
				expected.WriteString(nameStr)
			case tokenCount:
				expected.WriteString(countStr)
			case tokenCreated, tokenModified:
				// Conv has no Dir, so timestamps will be zero → "-"
				expected.WriteByte('-')
			case tokenHead:
				// Conv has no Dir, so head will be -1 → "-"
				expected.WriteByte('-')
			case tokenField:
				expected.WriteString(resolveField(tok.Literal, input.Conv))
			}
		}

		if result != expected.String() {
			t.Logf("format=%q\ngot:  %q\nwant: %q", input.Format, result, expected.String())
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestParseFormatString covers table-driven tests for parseFormatString.
func TestParseFormatString(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   []token
	}{
		{
			name:   "%t type token",
			format: "%t",
			want:   []token{{Kind: tokenType}},
		},
		{
			name:   "%n name token",
			format: "%n",
			want:   []token{{Kind: tokenName}},
		},
		{
			name:   "%c count token",
			format: "%c",
			want:   []token{{Kind: tokenCount}},
		},
		{
			name:   "%C created token",
			format: "%C",
			want:   []token{{Kind: tokenCreated}},
		},
		{
			name:   "%M modified token",
			format: "%M",
			want:   []token{{Kind: tokenModified}},
		},
		{
			name:   "%h head token",
			format: "%h",
			want:   []token{{Kind: tokenHead}},
		},
		{
			name:   "%{channel} field token",
			format: "%{channel}",
			want:   []token{{Kind: tokenField, Literal: "channel"}},
		},
		{
			name:   "%% literal percent",
			format: "%%",
			want:   []token{{Kind: tokenPercent}},
		},
		{
			name:   "%{unknown} field token with unknown name",
			format: "%{unknown}",
			want:   []token{{Kind: tokenField, Literal: "unknown"}},
		},
		{
			name:   "bare % at end of string",
			format: "hello%",
			want: []token{
				{Kind: tokenLiteral, Literal: "hello%"},
			},
		},
		{
			name:   "%{ unterminated brace",
			format: "%{oops",
			want: []token{
				{Kind: tokenLiteral, Literal: "%{oops"},
			},
		},
		{
			name:   "unknown %x passes through as literal",
			format: "%x",
			want: []token{
				{Kind: tokenLiteral, Literal: "%x"},
			},
		},
		{
			name:   "mixed format string",
			format: "%t: %n (%c) %%",
			want: []token{
				{Kind: tokenType},
				{Kind: tokenLiteral, Literal: ": "},
				{Kind: tokenName},
				{Kind: tokenLiteral, Literal: " ("},
				{Kind: tokenCount},
				{Kind: tokenLiteral, Literal: ") "},
				{Kind: tokenPercent},
			},
		},
		{
			name:   "empty format string",
			format: "",
			want:   nil,
		},
		{
			name:   "only literal text",
			format: "hello world",
			want: []token{
				{Kind: tokenLiteral, Literal: "hello world"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFormatString(tt.format)
			if !tokensEqual(got, tt.want) {
				t.Errorf("parseFormatString(%q)\n  got:  %v\n  want: %v", tt.format, fmtTokens(got), fmtTokens(tt.want))
			}
		})
	}
}

// TestFormatConv covers table-driven tests for formatConv.
func TestFormatConv(t *testing.T) {
	const timeFmt = "2006-01-02 15:04:05"

	// Create a temp store with messages for timestamp/head tests.
	tmpDir := t.TempDir()
	convDir := filepath.Join(tmpDir, "Chats", "alice,bob")
	msgsDir := filepath.Join(convDir, "messages")

	// Create two messages: ID 1 (sent_at=1000000) and ID 5 (sent_at=2000000).
	const sentAt1 int64 = 1000000
	const sentAt5 int64 = 2000000

	for _, m := range []struct {
		id     int
		sentAt int64
	}{
		{1, sentAt1},
		{5, sentAt5},
	} {
		dir := filepath.Join(msgsDir, strconv.Itoa(m.id))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		msg := keybase.MsgSummary{ID: m.id, SentAt: m.sentAt}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "message.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	convWithMsgs := store.ConvInfo{
		Type:     "Chat",
		Name:     "alice,bob",
		Dir:      convDir,
		MsgCount: 2,
	}

	convNoMsgs := store.ConvInfo{
		Type:     "Team",
		Name:     "engineering",
		Channel:  "general",
		MsgCount: 0,
	}

	tests := []struct {
		name   string
		format string
		conv   store.ConvInfo
		want   string
	}{
		{
			name:   "%t produces type",
			format: "%t",
			conv:   convWithMsgs,
			want:   "Chat",
		},
		{
			name:   "%n produces name for chat",
			format: "%n",
			conv:   convWithMsgs,
			want:   "alice,bob",
		},
		{
			name:   "%n produces team/channel for team",
			format: "%n",
			conv:   convNoMsgs,
			want:   "engineering/general",
		},
		{
			name:   "%c produces message count",
			format: "%c",
			conv:   convWithMsgs,
			want:   "2",
		},
		{
			name:   "%% produces literal percent",
			format: "%%",
			conv:   convWithMsgs,
			want:   "%",
		},
		{
			name:   "%{channel} resolves channel field",
			format: "%{channel}",
			conv:   convNoMsgs,
			want:   "general",
		},
		{
			name:   "%{unknown} resolves to empty string",
			format: "%{unknown}",
			conv:   convWithMsgs,
			want:   "",
		},
		{
			name:   "%{type} resolves type field",
			format: "%{type}",
			conv:   convWithMsgs,
			want:   "Chat",
		},
		{
			name:   "%{dir} resolves dir field",
			format: "%{dir}",
			conv:   convWithMsgs,
			want:   convDir,
		},
		{
			name:   "%{count} resolves count field",
			format: "%{count}",
			conv:   convWithMsgs,
			want:   "2",
		},
		{
			name:   "%h produces head message ID",
			format: "%h",
			conv:   convWithMsgs,
			want:   "5",
		},
		{
			name:   "%h with no messages produces dash",
			format: "%h",
			conv:   convNoMsgs,
			want:   "-",
		},
		{
			name:   "%C produces created timestamp from disk",
			format: "%C",
			conv:   convWithMsgs,
			want:   time.Unix(sentAt1, 0).Format(timeFmt),
		},
		{
			name:   "%M produces modified timestamp from disk",
			format: "%M",
			conv:   convWithMsgs,
			want:   time.Unix(sentAt5, 0).Format(timeFmt),
		},
		{
			name:   "%C with no messages produces dash",
			format: "%C",
			conv:   convNoMsgs,
			want:   "-",
		},
		{
			name:   "%M with no messages produces dash",
			format: "%M",
			conv:   convNoMsgs,
			want:   "-",
		},
		{
			name:   "mixed format with all tokens",
			format: "%t %n %c %%",
			conv:   convWithMsgs,
			want:   "Chat alice,bob 2 %",
		},
		{
			name:   "unknown %x passes through",
			format: "%x",
			conv:   convWithMsgs,
			want:   "%x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := parseFormatString(tt.format)
			got := formatConv(tokens, tt.conv, timeFmt)
			if got != tt.want {
				t.Errorf("formatConv(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

// tokensEqual compares two token slices for equality.
func tokensEqual(a, b []token) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].Literal != b[i].Literal {
			return false
		}
	}
	return true
}

// fmtTokens returns a human-readable representation of a token slice.
func fmtTokens(tokens []token) string {
	kindNames := map[tokenKind]string{
		tokenLiteral:  "Literal",
		tokenPercent:  "Percent",
		tokenType:     "Type",
		tokenName:     "Name",
		tokenCount:    "Count",
		tokenCreated:  "Created",
		tokenModified: "Modified",
		tokenHead:     "Head",
		tokenField:    "Field",
	}

	var parts []string
	for _, tok := range tokens {
		name := kindNames[tok.Kind]
		if tok.Literal != "" {
			parts = append(parts, name+"("+tok.Literal+")")
		} else {
			parts = append(parts, name)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
