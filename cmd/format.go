package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/major0/dateparse"
	"github.com/major0/kbchat/keybase"
)

// parseTimestamp parses a raw timestamp string using dateparse.
// Returns nil if raw is empty. Returns a wrapped error on parse failure.
func parseTimestamp(raw, flagName string, now time.Time) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := dateparse.Parse(raw, now)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", flagName, err)
	}
	return &t, nil
}

// FormatMsg formats a single message in IRC-log style.
// Text messages: [<timestamp>] <<username>> <body>
// Non-text messages: [<timestamp>] * <username> <type>: <summary>
// Verbose mode prefixes [id=<msgID>] and appends (<device_name>).
// No trailing newline.
func FormatMsg(msg keybase.MsgSummary, timeFmt string, verbose bool) string {
	ts := time.Unix(msg.SentAt, 0).Format(timeFmt)
	user := msg.Sender.Username

	var b strings.Builder

	if verbose {
		fmt.Fprintf(&b, "[id=%d] ", msg.ID)
	}

	if msg.Content.Type == "text" {
		body := ""
		if msg.Content.Text != nil {
			body = msg.Content.Text.Body
		}
		fmt.Fprintf(&b, "[%s] <%s> %s", ts, user, body)
	} else {
		summary := msgSummary(msg)
		fmt.Fprintf(&b, "[%s] * %s %s: %s", ts, user, msg.Content.Type, summary)
	}

	if verbose {
		fmt.Fprintf(&b, " (%s)", msg.Sender.DeviceName)
	}

	return b.String()
}

// msgSummary returns a human-readable summary for a non-text message.
func msgSummary(msg keybase.MsgSummary) string {
	switch msg.Content.Type {
	case "edit":
		if msg.Content.Edit != nil {
			return msg.Content.Edit.Body
		}
	case "delete":
		if msg.Content.Delete != nil {
			return formatDeleteSummary(msg.Content.Delete.MessageIDs)
		}
	case "reaction":
		if msg.Content.Reaction != nil {
			return msg.Content.Reaction.Body
		}
	case "attachment":
		if msg.Content.Attachment != nil {
			return msg.Content.Attachment.Object.Filename
		}
	case "headline":
		if msg.Content.Headline != nil {
			return msg.Content.Headline.Headline
		}
	case "metadata":
		if msg.Content.Metadata != nil {
			return msg.Content.Metadata.ConversationTitle
		}
	}
	return "(no summary)"
}

// formatDeleteSummary formats a delete message summary from message IDs.
func formatDeleteSummary(ids []int) string {
	if len(ids) == 0 {
		return "(no summary)"
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.Itoa(id)
	}
	if len(ids) == 1 {
		return "deleted message " + strs[0]
	}
	return "deleted messages " + strings.Join(strs, ", ")
}
