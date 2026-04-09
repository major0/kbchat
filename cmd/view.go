package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/major0/dateparse"
	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/store"
	"github.com/major0/optargs"
)

// viewOpts holds parsed options for the view subcommand.
type viewOpts struct {
	Filters []string // conversation filters (glob patterns)
	Count   int      // default 20; 0 = unlimited
	After   string   // raw --after value
	Before  string   // raw --before value
	Date    string   // raw --date value
	Verbose bool
}

// resolveQuery resolves raw flag values into a normalized query.
// now is passed explicitly for testability.
//
// The caller (flag parser) sets opts.Count to 20 as default. Passing
// --count 0 explicitly sets it to 0, meaning unlimited.
func resolveQuery(opts viewOpts, now time.Time) (*time.Time, *time.Time, int, bool, error) {
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
	after, err := parseTimestamp(opts.After, "--after", now)
	if err != nil {
		return nil, nil, 0, false, err
	}

	// Parse --before if set.
	before, err := parseTimestamp(opts.Before, "--before", now)
	if err != nil {
		return nil, nil, 0, false, err
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

// parseViewArgs parses view-specific flags from args using optargs.
// Returns the parsed options or an error.
func parseViewArgs(args []string) (*viewOpts, error) {
	opts := &viewOpts{Count: 20}

	countFlag := &optargs.Flag{
		Name:   "count",
		HasArg: optargs.RequiredArgument,
		Help:   "Number of messages (default: 20; 0 for all)",
	}
	afterFlag := &optargs.Flag{
		Name:   "after",
		HasArg: optargs.RequiredArgument,
		Help:   "Show messages after timestamp",
	}
	beforeFlag := &optargs.Flag{
		Name:   "before",
		HasArg: optargs.RequiredArgument,
		Help:   "Show messages before timestamp",
	}
	dateFlag := &optargs.Flag{
		Name:   "date",
		HasArg: optargs.RequiredArgument,
		Help:   "Show messages from a specific day (YYYY-MM-DD)",
	}
	verboseFlag := &optargs.Flag{
		Name:   "verbose",
		HasArg: optargs.NoArgument,
		Help:   "Include message IDs and metadata",
	}

	shortOpts := map[byte]*optargs.Flag{
		'n': countFlag,
	}
	longOpts := map[string]*optargs.Flag{
		"count":   countFlag,
		"after":   afterFlag,
		"before":  beforeFlag,
		"date":    dateFlag,
		"verbose": verboseFlag,
	}

	p, err := optargs.NewParser(optargs.ParserConfig{}, shortOpts, longOpts, args)
	if err != nil {
		return nil, fmt.Errorf("creating view parser: %w", err)
	}

	for opt, err := range p.Options() {
		if err != nil {
			return nil, fmt.Errorf("parsing view flags: %w", err)
		}
		switch opt.Name {
		case "count", "n":
			n, perr := strconv.Atoi(opt.Arg)
			if perr != nil {
				return nil, fmt.Errorf("invalid --count value: %q", opt.Arg)
			}
			if n < 0 {
				return nil, fmt.Errorf("invalid --count value: %q (must be >= 0)", opt.Arg)
			}
			opts.Count = n
		case "after":
			opts.After = opt.Arg
		case "before":
			opts.Before = opt.Arg
		case "date":
			opts.Date = opt.Arg
		case "verbose":
			opts.Verbose = true
		}
	}

	// Positional <filter> arguments required.
	if len(p.Args) == 0 {
		return nil, errors.New("missing required <filter> argument")
	}
	opts.Filters = p.Args

	return opts, nil
}

// RunView executes the view subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunView(args []string, cfg *config.Config) error {
	return runView(args, cfg, os.Stdout, time.Now())
}

// runView is the internal implementation of RunView, accepting injectable
// dependencies for testing. w is the output writer, now is the reference
// time for query resolution.
func runView(args []string, cfg *config.Config, w io.Writer, now time.Time) error {
	opts, err := parseViewArgs(args)
	if err != nil {
		return err
	}

	after, before, count, rangeMode, err := resolveQuery(*opts, now)
	if err != nil {
		return err
	}

	// Scan all conversations once.
	allConvs, err := store.ScanConversations(cfg.StorePath)
	if err != nil {
		return fmt.Errorf("scanning conversations: %w", err)
	}

	matches := store.FilterConvInfos(allConvs, opts.Filters)
	if len(matches) == 0 {
		return errors.New("no matching conversations")
	}

	// Sort for deterministic output.
	sort.Slice(matches, func(i, j int) bool {
		return store.ConvInfoPath(matches[i]) < store.ConvInfoPath(matches[j])
	})

	multiConv := len(matches) > 1
	timeFmt := cfg.TimeFmt()

	for ci, conv := range matches {
		// Read messages: optimized path when no timestamp filters.
		var msgs []keybase.MsgSummary
		if after == nil && before == nil && !rangeMode {
			msgs, err = store.ReadMessages(conv.Dir, count)
		} else {
			msgs, err = store.ReadMessages(conv.Dir, 0)
		}
		if err != nil {
			return fmt.Errorf("reading messages from %s: %w", store.ConvInfoPath(conv), err)
		}

		msgs = filterByTimestamp(msgs, after, before)
		msgs = applyCountLimit(msgs, count, after != nil)

		// Separator between conversation blocks.
		if multiConv && ci > 0 {
			fmt.Fprintln(w, "--")
		}

		// Header when viewing multiple conversations.
		if multiConv {
			fmt.Fprintf(w, "==> %s <==\n", store.ConvInfoPath(conv))
		}

		for _, msg := range msgs {
			fmt.Fprintln(w, FormatMsg(msg, timeFmt, opts.Verbose))
		}
	}

	return nil
}
