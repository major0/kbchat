package cmd

import (
	"bytes"
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

	"github.com/major0/kbchat/config"
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

// Feature: keybase-chat-list, Property 5: Single-column output has exactly
// one entry per line.
//
// For any non-empty conversation list, the number of non-empty lines in
// single-column output equals the number of conversations.
//
// **Validates: Req 1.7**

// singleColumnInput holds a randomly generated list of ConvInfo values.
type singleColumnInput struct {
	Convs []store.ConvInfo
}

// Generate implements quick.Generator for singleColumnInput.
func (singleColumnInput) Generate(r *rand.Rand, size int) reflect.Value {
	types := []string{"Chat", "Team"}
	names := []string{"alice", "bob", "alice,bob", "engineering", "design"}
	channels := []string{"general", "random", "dev"}

	n := 1 + r.Intn(20) // at least 1 conversation
	convs := make([]store.ConvInfo, n)
	for i := range n {
		typ := types[r.Intn(len(types))]
		name := names[r.Intn(len(names))]
		var ch string
		if typ == "Team" {
			ch = channels[r.Intn(len(channels))]
		}
		convs[i] = store.ConvInfo{
			Type:     typ,
			Name:     name,
			Channel:  ch,
			MsgCount: r.Intn(100),
		}
	}
	return reflect.ValueOf(singleColumnInput{Convs: convs})
}

func TestPropertySingleColumnLineCount(t *testing.T) {
	f := func(input singleColumnInput) bool {
		var buf bytes.Buffer
		formatSingleColumn(&buf, input.Convs)

		output := buf.String()
		// Count non-empty lines.
		lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
		nonEmpty := 0
		for _, line := range lines {
			if line != "" {
				nonEmpty++
			}
		}

		if nonEmpty != len(input.Convs) {
			t.Logf("got %d non-empty lines, want %d convs", nonEmpty, len(input.Convs))
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// Feature: keybase-chat-list, Property 6: Long format contains all required
// fields.
//
// For any conversation formatted in long mode, the output line contains:
// the type string, the message count as a decimal, and the conversation name.
//
// **Validates: Req 1.9, 1.13, 1.14**

// longFormatInput holds a randomly generated ConvInfo for long format testing.
type longFormatInput struct {
	Conv store.ConvInfo
}

// Generate implements quick.Generator for longFormatInput.
func (longFormatInput) Generate(r *rand.Rand, size int) reflect.Value {
	types := []string{"Chat", "Team"}
	names := []string{"alice", "bob", "alice,bob", "engineering", "design"}
	channels := []string{"general", "random", "dev"}

	typ := types[r.Intn(len(types))]
	name := names[r.Intn(len(names))]
	var ch string
	if typ == "Team" {
		ch = channels[r.Intn(len(channels))]
	}

	conv := store.ConvInfo{
		Type:     typ,
		Name:     name,
		Channel:  ch,
		MsgCount: r.Intn(500),
	}
	return reflect.ValueOf(longFormatInput{Conv: conv})
}

func TestPropertyLongFormatContainsRequiredFields(t *testing.T) {
	const timeFmt = "2006-01-02 15:04:05"

	f := func(input longFormatInput) bool {
		var buf bytes.Buffer
		formatLong(&buf, []store.ConvInfo{input.Conv}, timeFmt)

		output := buf.String()

		// Must contain the type string.
		if !strings.Contains(output, input.Conv.Type) {
			t.Logf("output=%q missing type=%q", output, input.Conv.Type)
			return false
		}

		// Must contain the message count as a decimal.
		countStr := strconv.Itoa(input.Conv.MsgCount)
		if !strings.Contains(output, countStr) {
			t.Logf("output=%q missing count=%q", output, countStr)
			return false
		}

		// Must contain the conversation name.
		nameStr := convName(input.Conv)
		if !strings.Contains(output, nameStr) {
			t.Logf("output=%q missing name=%q", output, nameStr)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestParseListArgs covers table-driven tests for flag parsing.
func TestParseListArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantMode outputMode
		wantFmt  string
		wantVerb bool
	}{
		{
			name:     "-1 selects single-column",
			args:     []string{"-1"},
			wantMode: modeSingleColumn,
		},
		{
			name:     "-C selects columns",
			args:     []string{"-C"},
			wantMode: modeColumns,
		},
		{
			name:     "-l selects long",
			args:     []string{"-l"},
			wantMode: modeLong,
		},
		{
			name:     "--format=long selects long",
			args:     []string{"--format=long"},
			wantMode: modeLong,
		},
		{
			name:     "--format=%t %n selects custom",
			args:     []string{"--format=%t %n"},
			wantMode: modeCustom,
			wantFmt:  "%t %n",
		},
		{
			name:     "--verbose selects long and sets verbose",
			args:     []string{"--verbose"},
			wantMode: modeLong,
			wantVerb: true,
		},
		{
			name:     "--format=single-column",
			args:     []string{"--format=single-column"},
			wantMode: modeSingleColumn,
		},
		{
			name:     "--format=columns",
			args:     []string{"--format=columns"},
			wantMode: modeColumns,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseListArgs(tt.args)
			if err != nil {
				t.Fatalf("parseListArgs(%v) error: %v", tt.args, err)
			}
			if opts.Mode != tt.wantMode {
				t.Errorf("Mode = %d, want %d", opts.Mode, tt.wantMode)
			}
			if tt.wantFmt != "" && opts.FormatStr != tt.wantFmt {
				t.Errorf("FormatStr = %q, want %q", opts.FormatStr, tt.wantFmt)
			}
			if opts.Verbose != tt.wantVerb {
				t.Errorf("Verbose = %v, want %v", opts.Verbose, tt.wantVerb)
			}
		})
	}
}

// createTestStore builds a temporary store directory with the given
// conversations and returns the store path. Each conv entry specifies
// type ("Chat" or "Team"), name, channel (Teams only), and message count.
func createTestStore(t *testing.T, convs []store.ConvInfo) string {
	t.Helper()
	storeDir := t.TempDir()

	for _, conv := range convs {
		var convDir string
		switch conv.Type {
		case "Chat":
			convDir = filepath.Join(storeDir, "Chats", conv.Name)
		case "Team":
			convDir = filepath.Join(storeDir, "Teams", conv.Name, conv.Channel)
		}

		msgsDir := filepath.Join(convDir, "messages")
		if err := os.MkdirAll(msgsDir, 0o755); err != nil {
			t.Fatal(err)
		}

		// Create message directories with message.json files.
		for i := 1; i <= conv.MsgCount; i++ {
			msgDir := filepath.Join(msgsDir, strconv.Itoa(i))
			if err := os.MkdirAll(msgDir, 0o755); err != nil {
				t.Fatal(err)
			}
			msg := keybase.MsgSummary{
				ID:     i,
				SentAt: int64(1000000 + i*1000),
			}
			data, err := json.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(msgDir, "message.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	return storeDir
}

// TestFormatSingleColumn tests single-column output with known inputs.
func TestFormatSingleColumn(t *testing.T) {
	tests := []struct {
		name  string
		convs []store.ConvInfo
		want  string
	}{
		{
			name:  "empty list produces no output",
			convs: nil,
			want:  "",
		},
		{
			name: "single chat",
			convs: []store.ConvInfo{
				{Type: "Chat", Name: "alice,bob"},
			},
			want: "Chats/alice,bob\n",
		},
		{
			name: "single team",
			convs: []store.ConvInfo{
				{Type: "Team", Name: "engineering", Channel: "general"},
			},
			want: "Teams/engineering/general\n",
		},
		{
			name: "mixed chat and team",
			convs: []store.ConvInfo{
				{Type: "Chat", Name: "alice,bob"},
				{Type: "Team", Name: "engineering", Channel: "general"},
			},
			want: "Chats/alice,bob\nTeams/engineering/general\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatSingleColumn(&buf, tt.convs)
			if got := buf.String(); got != tt.want {
				t.Errorf("formatSingleColumn() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatLong tests long format output with known inputs.
func TestFormatLong(t *testing.T) {
	const timeFmt = "2006-01-02 15:04:05"

	// Create a temp store with messages for timestamp tests.
	storeDir := t.TempDir()
	chatDir := filepath.Join(storeDir, "Chats", "alice,bob")
	msgsDir := filepath.Join(chatDir, "messages")

	for _, m := range []struct {
		id     int
		sentAt int64
	}{
		{1, 1000000},
		{3, 2000000},
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

	tests := []struct {
		name  string
		convs []store.ConvInfo
		want  string
	}{
		{
			name:  "empty list",
			convs: nil,
			want:  "",
		},
		{
			name: "chat with messages",
			convs: []store.ConvInfo{
				{Type: "Chat", Name: "alice,bob", Dir: chatDir, MsgCount: 2},
			},
			want: "Chat\t2\t" +
				time.Unix(1000000, 0).Format(timeFmt) + "\t" +
				time.Unix(2000000, 0).Format(timeFmt) + "\t" +
				"alice,bob\n",
		},
		{
			name: "team without messages shows dashes",
			convs: []store.ConvInfo{
				{Type: "Team", Name: "engineering", Channel: "general", MsgCount: 0},
			},
			want: "Team\t0\t-\t-\tengineering/general\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatLong(&buf, tt.convs, timeFmt)
			if got := buf.String(); got != tt.want {
				t.Errorf("formatLong() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// TestRunListOutput tests RunList end-to-end with temp store directories.
// We test the formatters directly since RunList writes to os.Stdout which
// is harder to capture. parseListArgs is tested separately above.
func TestRunListOutput(t *testing.T) {
	// Build a store with known conversations.
	convs := []store.ConvInfo{
		{Type: "Chat", Name: "alice,bob", MsgCount: 3},
		{Type: "Team", Name: "engineering", Channel: "general", MsgCount: 5},
	}
	storeDir := createTestStore(t, convs)

	// Write a config file pointing to the store.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")
	cfgData, err := json.Marshal(config.Config{StorePath: storeDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	// Scan conversations from the store to get Dir fields populated.
	scanned, err := store.ScanConversations(cfg.StorePath)
	if err != nil {
		t.Fatalf("ScanConversations: %v", err)
	}

	t.Run("single-column output", func(t *testing.T) {
		var buf bytes.Buffer
		formatSingleColumn(&buf, scanned)
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != len(scanned) {
			t.Errorf("got %d lines, want %d", len(lines), len(scanned))
		}
	})

	t.Run("long format output", func(t *testing.T) {
		var buf bytes.Buffer
		formatLong(&buf, scanned, cfg.TimeFmt())
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != len(scanned) {
			t.Errorf("got %d lines, want %d", len(lines), len(scanned))
		}
		// Each line should contain the type and name.
		for i, conv := range scanned {
			if !strings.Contains(lines[i], conv.Type) {
				t.Errorf("line %d missing type %q: %q", i, conv.Type, lines[i])
			}
			if !strings.Contains(lines[i], convName(conv)) {
				t.Errorf("line %d missing name %q: %q", i, convName(conv), lines[i])
			}
		}
	})

	t.Run("columns output", func(t *testing.T) {
		var buf bytes.Buffer
		formatColumns(&buf, scanned, 80)
		output := buf.String()
		// All conversation paths should appear in the output.
		for _, conv := range scanned {
			p := store.ConvInfoPath(conv)
			if !strings.Contains(output, p) {
				t.Errorf("columns output missing %q", p)
			}
		}
	})

	t.Run("empty store", func(t *testing.T) {
		emptyDir := t.TempDir()
		// Create Chats/ and Teams/ but no conversations.
		if err := os.MkdirAll(filepath.Join(emptyDir, "Chats"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(emptyDir, "Teams"), 0o755); err != nil {
			t.Fatal(err)
		}

		emptyConvs, err := store.ScanConversations(emptyDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(emptyConvs) != 0 {
			t.Errorf("expected 0 conversations, got %d", len(emptyConvs))
		}

		var buf bytes.Buffer
		formatSingleColumn(&buf, emptyConvs)
		if buf.String() != "" {
			t.Errorf("expected empty output, got %q", buf.String())
		}
	})

	t.Run("no match message", func(t *testing.T) {
		filtered := store.FilterConvInfos(scanned, []string{"Chat/nobody"})
		if len(filtered) != 0 {
			t.Errorf("expected 0 filtered conversations, got %d", len(filtered))
		}
	})
}
