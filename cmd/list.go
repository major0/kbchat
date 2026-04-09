package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/store"
)

// outputMode selects the list output format.
type outputMode int

const (
	modeSingleColumn outputMode = iota // -1
	modeColumns                        // -C
	modeLong                           // -l
	modeCustom                         // --format=<string>
)

// Ensure outputMode constants are retained for RunList (task 4).
var _ = [...]outputMode{modeSingleColumn, modeColumns, modeLong, modeCustom}

// listOpts holds parsed options for the list subcommand.
type listOpts struct {
	Mode      outputMode
	FormatStr string // raw --format value (only when Mode == modeCustom)
	Verbose   bool
}

// Ensure listOpts is retained for RunList (task 4).
var _ listOpts

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

	// Lazy timestamp cache: computed at most once per call.
	var tsLoaded bool
	var created, modified time.Time
	loadTS := func() {
		if !tsLoaded {
			created, modified = convTimestamps(conv)
			tsLoaded = true
		}
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
			// Head is the highest message ID. We can derive it from
			// the messages dir without reading message.json.
			head := headMsgID(conv)
			if head < 0 {
				b.WriteByte('-')
			} else {
				b.WriteString(strconv.Itoa(head))
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

// convTimestamps returns the first and last message timestamps for a conversation.
// It reads the messages/ directory, finds the min and max numeric IDs, and reads
// only those two message.json files for their sent_at fields.
// Returns zero times on any read failure.
func convTimestamps(conv store.ConvInfo) (time.Time, time.Time) {
	msgsDir := filepath.Join(conv.Dir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return time.Time{}, time.Time{}
	}

	minID := math.MaxInt
	maxID := -1
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if id < minID {
			minID = id
		}
		if id > maxID {
			maxID = id
		}
	}

	if maxID < 0 {
		// No numeric directories found.
		return time.Time{}, time.Time{}
	}

	created := readMsgTime(msgsDir, minID)
	if minID == maxID {
		return created, created
	}
	return created, readMsgTime(msgsDir, maxID)
}

// readMsgTime reads a single message.json and returns its sent_at as a time.Time.
// Returns zero time on any failure.
func readMsgTime(msgsDir string, id int) time.Time {
	path := filepath.Join(msgsDir, strconv.Itoa(id), "message.json")
	data, err := os.ReadFile(path)
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

// headMsgID returns the highest numeric message ID in a conversation's
// messages/ directory, or -1 if none found.
func headMsgID(conv store.ConvInfo) int {
	msgsDir := filepath.Join(conv.Dir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return -1
	}
	head := -1
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if id > head {
			head = id
		}
	}
	return head
}

// RunList executes the list subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunList(_ []string, _ *config.Config) error {
	fmt.Println("list: not implemented")
	return nil
}
