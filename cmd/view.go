package cmd

import (
	"fmt"
	"time"

	"github.com/major0/dateparse"
	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
)

// viewOpts holds parsed options for the view subcommand.
type viewOpts struct {
	Filter  string
	Count   int    // default 20; 0 = unlimited
	After   string // raw --after value
	Before  string // raw --before value
	Date    string // raw --date value
	Verbose bool
}

// resolveQuery resolves raw flag values into a normalized query.
// now is passed explicitly for testability.
//
// The caller (flag parser) sets opts.Count to 20 as default. Passing
// --count 0 explicitly sets it to 0, meaning unlimited.
func resolveQuery(opts viewOpts, now time.Time) (*time.Time, *time.Time, int, bool, error) {
	var after, before *time.Time
	count := opts.Count

	// --date takes priority: expands to after+before range.
	if opts.Date != "" {
		t, parseErr := dateparse.Parse(opts.Date, now)
		if parseErr != nil {
			return nil, nil, 0, false, fmt.Errorf("parsing --date: %w", parseErr)
		}
		// Truncate to start of day in UTC.
		dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		dayEnd := dayStart.AddDate(0, 0, 1)
		return &dayStart, &dayEnd, 0, true, nil
	}

	// Parse --after if set.
	if opts.After != "" {
		t, parseErr := dateparse.Parse(opts.After, now)
		if parseErr != nil {
			return nil, nil, 0, false, fmt.Errorf("parsing --after: %w", parseErr)
		}
		after = &t
	}

	// Parse --before if set.
	if opts.Before != "" {
		t, parseErr := dateparse.Parse(opts.Before, now)
		if parseErr != nil {
			return nil, nil, 0, false, fmt.Errorf("parsing --before: %w", parseErr)
		}
		before = &t
	}

	// Both after+before → range mode, count ignored.
	if after != nil && before != nil {
		return after, before, 0, true, nil
	}

	// No flags at all → before=now, use count from opts (default 20).
	if after == nil && before == nil {
		before = &now
	}

	return after, before, count, false, nil
}

// filterByTimestamp returns messages where sent_at >= after (if set) and
// sent_at < before (if set). The original slice is not modified.
func filterByTimestamp(msgs []keybase.MsgSummary, after, before *time.Time) []keybase.MsgSummary {
	if after == nil && before == nil {
		// No filtering needed; return a copy to avoid aliasing.
		result := make([]keybase.MsgSummary, len(msgs))
		copy(result, msgs)
		return result
	}

	result := make([]keybase.MsgSummary, 0, len(msgs))
	for _, m := range msgs {
		if after != nil && m.SentAt < after.Unix() {
			continue
		}
		if before != nil && m.SentAt >= before.Unix() {
			continue
		}
		result = append(result, m)
	}
	return result
}

// applyCountLimit limits the message slice to count messages.
// count == 0 or len(msgs) <= count returns all.
// afterSet true returns the first count (head); false returns the last count (tail).
func applyCountLimit(msgs []keybase.MsgSummary, count int, afterSet bool) []keybase.MsgSummary {
	if count == 0 || len(msgs) <= count {
		return msgs
	}
	if afterSet {
		return msgs[:count]
	}
	return msgs[len(msgs)-count:]
}

// RunView executes the view subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunView(_ []string, _ *config.Config) error {
	fmt.Println("view: not implemented")
	return nil
}
