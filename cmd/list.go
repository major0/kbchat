package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/store"
	"github.com/major0/optargs"
	"golang.org/x/term"
)

// outputMode selects the list output format.
type outputMode int

const (
	modeSingleColumn outputMode = iota // -1
	modeColumns                        // -C
	modeLong                           // -l
	modeCustom                         // --format=<string>
)

// listOpts holds parsed options for the list subcommand.
type listOpts struct {
	Mode      outputMode
	FormatStr string // raw --format value (only when Mode == modeCustom)
	Verbose   bool
}

// token represents a parsed element of a custom format string.
type token struct {
	Kind    tokenKind
	Literal string // for tokenLiteral: the text; for tokenField: the field name
}

// tokenKind classifies a format token.
type tokenKind int

const (
	tokenLiteral  tokenKind = iota // raw text
	tokenPercent                   // %%
	tokenType                      // %t
	tokenName                      // %n
	tokenCount                     // %c
	tokenCreated                   // %C
	tokenModified                  // %M
	tokenHead                      // %h
	tokenField                     // %{field}
)

// parseFormatString parses a custom format string into a token slice.
// Recognized escapes: %% (literal %), %t (type), %n (name), %c (count),
// %C (created timestamp), %M (modified timestamp), %h (head ID),
// %{field} (named field). Unknown %x passes through as literal "%x".
// Unterminated %{ is treated as literal.
func parseFormatString(format string) []token {
	var tokens []token
	var lit strings.Builder

	flushLit := func() {
		if lit.Len() > 0 {
			tokens = append(tokens, token{Kind: tokenLiteral, Literal: lit.String()})
			lit.Reset()
		}
	}

	i := 0
	for i < len(format) {
		if format[i] != '%' {
			lit.WriteByte(format[i])
			i++
			continue
		}

		// We have a '%'. Need at least one more character.
		if i+1 >= len(format) {
			// Bare '%' at end of string — treat as literal.
			lit.WriteByte('%')
			i++
			continue
		}

		next := format[i+1]
		switch next {
		case '%':
			flushLit()
			tokens = append(tokens, token{Kind: tokenPercent})
			i += 2
		case 't':
			flushLit()
			tokens = append(tokens, token{Kind: tokenType})
			i += 2
		case 'n':
			flushLit()
			tokens = append(tokens, token{Kind: tokenName})
			i += 2
		case 'c':
			flushLit()
			tokens = append(tokens, token{Kind: tokenCount})
			i += 2
		case 'C':
			flushLit()
			tokens = append(tokens, token{Kind: tokenCreated})
			i += 2
		case 'M':
			flushLit()
			tokens = append(tokens, token{Kind: tokenModified})
			i += 2
		case 'h':
			flushLit()
			tokens = append(tokens, token{Kind: tokenHead})
			i += 2
		case '{':
			// Look for closing '}'.
			end := strings.IndexByte(format[i+2:], '}')
			if end < 0 {
				// Unterminated %{ — treat as literal.
				lit.WriteString("%{")
				i += 2
			} else {
				flushLit()
				field := format[i+2 : i+2+end]
				tokens = append(tokens, token{Kind: tokenField, Literal: field})
				i = i + 2 + end + 1
			}
		default:
			// Unknown %x — pass through as literal "%x".
			lit.WriteByte('%')
			lit.WriteByte(next)
			i += 2
		}
	}

	flushLit()
	return tokens
}

// convName returns the display name for a conversation.
// Chats use the participant name; Teams use "team/channel".
func convName(conv store.ConvInfo) string {
	if conv.Channel != "" {
		return conv.Name + "/" + conv.Channel
	}
	return conv.Name
}

// formatConv formats a single conversation using a parsed token slice.
// timeFmt is a Go time layout string for timestamp formatting.
func formatConv(tokens []token, conv store.ConvInfo, timeFmt string) string {
	var b strings.Builder

	// Lazy caches: computed at most once per call.
	var tsLoaded bool
	var created, modified time.Time
	var rangeLoaded bool
	var minID, maxID int

	loadRange := func() {
		if !rangeLoaded {
			minID, maxID = convMsgRange(conv)
			rangeLoaded = true
		}
	}
	loadTS := func() {
		if tsLoaded {
			return
		}
		loadRange()
		if maxID >= 0 {
			msgsDir := filepath.Join(conv.Dir, "messages")
			created = readMsgTime(msgsDir, minID)
			if minID == maxID {
				modified = created
			} else {
				modified = readMsgTime(msgsDir, maxID)
			}
		}
		tsLoaded = true
	}

	for _, tok := range tokens {
		switch tok.Kind {
		case tokenLiteral:
			b.WriteString(tok.Literal)
		case tokenPercent:
			b.WriteByte('%')
		case tokenType:
			b.WriteString(conv.Type)
		case tokenName:
			b.WriteString(convName(conv))
		case tokenCount:
			b.WriteString(strconv.Itoa(conv.MsgCount))
		case tokenCreated:
			loadTS()
			if created.IsZero() {
				b.WriteByte('-')
			} else {
				b.WriteString(created.Format(timeFmt))
			}
		case tokenModified:
			loadTS()
			if modified.IsZero() {
				b.WriteByte('-')
			} else {
				b.WriteString(modified.Format(timeFmt))
			}
		case tokenHead:
			loadRange()
			if maxID < 0 {
				b.WriteByte('-')
			} else {
				b.WriteString(strconv.Itoa(maxID))
			}
		case tokenField:
			b.WriteString(resolveField(tok.Literal, conv))
		}
	}
	return b.String()
}

// resolveField maps a named field to its ConvInfo value.
// Unknown fields produce an empty string.
func resolveField(name string, conv store.ConvInfo) string {
	switch name {
	case "type":
		return conv.Type
	case "name":
		return convName(conv)
	case "channel":
		return conv.Channel
	case "dir":
		return conv.Dir
	case "count":
		return strconv.Itoa(conv.MsgCount)
	default:
		return ""
	}
}

// convMsgRange scans the messages/ directory once and returns the min and max
// numeric message IDs. Returns (-1, -1) if no messages are found.
func convMsgRange(conv store.ConvInfo) (int, int) {
	msgsDir := filepath.Join(conv.Dir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return -1, -1
	}

	lo := math.MaxInt
	hi := -1
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if id < lo {
			lo = id
		}
		if id > hi {
			hi = id
		}
	}

	if hi < 0 {
		return -1, -1
	}
	return lo, hi
}

// convTimestamps returns the first and last message timestamps for a conversation.
// Scans from the lowest ID upward to find the first message with a non-zero
// sent_at (skipping deleted placeholders). Returns zero times on failure.
func convTimestamps(conv store.ConvInfo) (time.Time, time.Time) {
	msgsDir := filepath.Join(conv.Dir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return time.Time{}, time.Time{}
	}

	// Collect and sort all numeric IDs.
	ids := make([]int, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return time.Time{}, time.Time{}
	}
	sort.Ints(ids)

	// Scan forward from lowest to find first real timestamp.
	var created time.Time
	for _, id := range ids {
		t := readMsgTime(msgsDir, id)
		if !t.IsZero() {
			created = t
			break
		}
	}

	// Scan backward from highest to find last real timestamp.
	var modified time.Time
	for i := len(ids) - 1; i >= 0; i-- {
		t := readMsgTime(msgsDir, ids[i])
		if !t.IsZero() {
			modified = t
			break
		}
	}

	return created, modified
}

// readMsgTime reads a single message.json and returns its sent_at as a time.Time.
// Returns zero time on any failure.
func readMsgTime(msgsDir string, id int) time.Time {
	p := filepath.Join(msgsDir, strconv.Itoa(id), "message.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}
	}
	var msg keybase.MsgSummary
	if err := json.Unmarshal(data, &msg); err != nil {
		return time.Time{}
	}
	if msg.SentAt == 0 {
		return time.Time{}
	}
	return time.Unix(msg.SentAt, 0)
}

// formatSingleColumn writes one conversation per line to w.
func formatSingleColumn(w io.Writer, convs []store.ConvInfo) {
	for _, conv := range convs {
		fmt.Fprintln(w, store.ConvInfoPath(conv))
	}
}

// formatColumns arranges conversations in columns fitting the given width,
// filling top-to-bottom, left-to-right (like ls).
func formatColumns(w io.Writer, convs []store.ConvInfo, width int) {
	if len(convs) == 0 {
		return
	}

	// Compute display strings and find the longest.
	paths := make([]string, len(convs))
	maxLen := 0
	for i, conv := range convs {
		paths[i] = store.ConvInfoPath(conv)
		if len(paths[i]) > maxLen {
			maxLen = len(paths[i])
		}
	}

	// Column width = longest entry + 2 spaces padding.
	colWidth := maxLen + 2
	if colWidth > width {
		// Entries wider than terminal — fall back to single column.
		for _, p := range paths {
			fmt.Fprintln(w, p)
		}
		return
	}

	numCols := width / colWidth
	numCols = max(numCols, 1)
	numRows := (len(paths) + numCols - 1) / numCols

	for row := range numRows {
		for col := range numCols {
			idx := col*numRows + row
			if idx >= len(paths) {
				continue
			}
			if col < numCols-1 && (col+1)*numRows+row < len(paths) {
				fmt.Fprintf(w, "%-*s", colWidth, paths[idx])
			} else {
				fmt.Fprint(w, paths[idx])
			}
		}
		fmt.Fprintln(w)
	}
}

// formatLong writes one conversation per line in long format:
// <type>\t<count>\t<created>\t<modified>\t<name>.
func formatLong(w io.Writer, convs []store.ConvInfo, timeFmt string) {
	for _, conv := range convs {
		created, modified := convTimestamps(conv)
		createdStr := "-"
		if !created.IsZero() {
			createdStr = created.Format(timeFmt)
		}
		modifiedStr := "-"
		if !modified.IsZero() {
			modifiedStr = modified.Format(timeFmt)
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			conv.Type, conv.MsgCount, createdStr, modifiedStr, convName(conv))
	}
}

// terminalWidth returns the terminal width for stdout, or 80 as fallback.
func terminalWidth() int {
	fd := os.Stdout.Fd()
	w, _, err := term.GetSize(int(fd)) //nolint:gosec // fd is stdout, no overflow risk
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// isStdoutTerminal reports whether stdout is connected to a terminal.
func isStdoutTerminal() bool {
	fd := os.Stdout.Fd()
	return term.IsTerminal(int(fd)) //nolint:gosec // fd is stdout, no overflow risk
}

// parseListArgs parses list-specific flags from args using optargs.
func parseListArgs(args []string) (*listOpts, []string, error) {
	opts := &listOpts{}
	modeSet := false

	singleColFlag := &optargs.Flag{
		Name:   "1",
		HasArg: optargs.NoArgument,
		Help:   "One conversation per line",
	}
	columnsFlag := &optargs.Flag{
		Name:   "columns",
		HasArg: optargs.NoArgument,
		Help:   "Column format",
	}
	longFlag := &optargs.Flag{
		Name:   "long",
		HasArg: optargs.NoArgument,
		Help:   "Long format",
	}
	formatFlag := &optargs.Flag{
		Name:   "format",
		HasArg: optargs.RequiredArgument,
		Help:   "Output format (single-column, columns, long, or custom)",
	}
	verboseFlag := &optargs.Flag{
		Name:   "verbose",
		HasArg: optargs.NoArgument,
		Help:   "Verbose output (alias for -l)",
	}

	shortOpts := map[byte]*optargs.Flag{
		'1': singleColFlag,
		'C': columnsFlag,
		'l': longFlag,
	}
	longOpts := map[string]*optargs.Flag{
		"format":  formatFlag,
		"verbose": verboseFlag,
	}

	p, err := optargs.NewParser(optargs.ParserConfig{}, shortOpts, longOpts, args)
	if err != nil {
		return nil, nil, fmt.Errorf("creating list parser: %w", err)
	}

	for opt, err := range p.Options() {
		if err != nil {
			return nil, nil, fmt.Errorf("parsing list flags: %w", err)
		}
		switch opt.Name {
		case "1":
			opts.Mode = modeSingleColumn
			modeSet = true
		case "columns", "C":
			opts.Mode = modeColumns
			modeSet = true
		case "long", "l":
			opts.Mode = modeLong
			modeSet = true
		case "verbose":
			opts.Verbose = true
			opts.Mode = modeLong
			modeSet = true
		case "format":
			modeSet = true
			switch opt.Arg {
			case "single-column":
				opts.Mode = modeSingleColumn
			case "columns":
				opts.Mode = modeColumns
			case "long":
				opts.Mode = modeLong
			default:
				opts.Mode = modeCustom
				opts.FormatStr = opt.Arg
			}
		}
	}

	// Default mode: -C when TTY, -1 otherwise.
	if !modeSet {
		if isStdoutTerminal() {
			opts.Mode = modeColumns
		} else {
			opts.Mode = modeSingleColumn
		}
	}

	return opts, p.Args, nil
}

// RunList executes the list subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunList(args []string, cfg *config.Config) error {
	opts, filters, err := parseListArgs(args)
	if err != nil {
		return err
	}

	convs, err := store.ScanAndFilter(cfg.StorePath, filters)
	if err != nil {
		return err
	}

	if len(convs) == 0 {
		fmt.Fprintln(os.Stderr, "no conversations found")
		return nil
	}

	w := os.Stdout
	switch opts.Mode {
	case modeSingleColumn:
		formatSingleColumn(w, convs)
	case modeColumns:
		formatColumns(w, convs, terminalWidth())
	case modeLong:
		formatLong(w, convs, cfg.TimeFmt())
	case modeCustom:
		tokens := parseFormatString(opts.FormatStr)
		timeFmt := cfg.TimeFmt()
		for _, conv := range convs {
			fmt.Fprintln(w, formatConv(tokens, conv, timeFmt))
		}
	}

	return nil
}
