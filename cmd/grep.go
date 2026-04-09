package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/major0/dateparse"
	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/store"
	"github.com/major0/optargs"
)

// grepOpts holds parsed options for the grep subcommand.
type grepOpts struct {
	Filters []string // conversation filters (all positional args except last)
	Pattern string   // last positional arg
	Regexp  bool     // -E
	ICase   bool     // -i
	After   string   // --after raw value
	Before  string   // --before raw value
	CtxA    int      // -A (after context lines)
	CtxB    int      // -B (before context lines)
	Count   int      // --count; 0 = unlimited
	Verbose bool
}

// parseGrepArgs parses grep-specific flags from args using optargs.
// Returns the parsed options or an error.
func parseGrepArgs(args []string) (*grepOpts, error) {
	opts := &grepOpts{}

	regexpFlag := &optargs.Flag{
		Name:   "regexp",
		HasArg: optargs.NoArgument,
		Help:   "Interpret pattern as a regular expression",
	}
	icaseFlag := &optargs.Flag{
		Name:   "i",
		HasArg: optargs.NoArgument,
		Help:   "Case-insensitive matching",
	}
	ctxAFlag := &optargs.Flag{
		Name:   "A",
		HasArg: optargs.RequiredArgument,
		Help:   "Show n messages after each match",
	}
	ctxBFlag := &optargs.Flag{
		Name:   "B",
		HasArg: optargs.RequiredArgument,
		Help:   "Show n messages before each match",
	}
	ctxCFlag := &optargs.Flag{
		Name:   "C",
		HasArg: optargs.RequiredArgument,
		Help:   "Show n messages before and after each match",
	}
	countFlag := &optargs.Flag{
		Name:   "count",
		HasArg: optargs.RequiredArgument,
		Help:   "Limit total results (default: 0 = unlimited)",
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
	verboseFlag := &optargs.Flag{
		Name:   "verbose",
		HasArg: optargs.NoArgument,
		Help:   "Include message IDs and metadata",
	}

	shortOpts := map[byte]*optargs.Flag{
		'E': regexpFlag,
		'i': icaseFlag,
		'A': ctxAFlag,
		'B': ctxBFlag,
		'C': ctxCFlag,
	}
	longOpts := map[string]*optargs.Flag{
		"regexp":  regexpFlag,
		"count":   countFlag,
		"after":   afterFlag,
		"before":  beforeFlag,
		"verbose": verboseFlag,
	}

	p, err := optargs.NewParser(optargs.ParserConfig{}, shortOpts, longOpts, args)
	if err != nil {
		return nil, fmt.Errorf("creating grep parser: %w", err)
	}

	for opt, err := range p.Options() {
		if err != nil {
			return nil, fmt.Errorf("parsing grep flags: %w", err)
		}
		switch opt.Name {
		case "regexp", "E":
			opts.Regexp = true
		case "i":
			opts.ICase = true
		case "A":
			n, perr := strconv.Atoi(opt.Arg)
			if perr != nil {
				return nil, fmt.Errorf("invalid -A value: %q", opt.Arg)
			}
			opts.CtxA = n
		case "B":
			n, perr := strconv.Atoi(opt.Arg)
			if perr != nil {
				return nil, fmt.Errorf("invalid -B value: %q", opt.Arg)
			}
			opts.CtxB = n
		case "C":
			n, perr := strconv.Atoi(opt.Arg)
			if perr != nil {
				return nil, fmt.Errorf("invalid -C value: %q", opt.Arg)
			}
			opts.CtxA = n
			opts.CtxB = n
		case "count":
			n, perr := strconv.Atoi(opt.Arg)
			if perr != nil {
				return nil, fmt.Errorf("invalid --count value: %q", opt.Arg)
			}
			opts.Count = n
		case "after":
			opts.After = opt.Arg
		case "before":
			opts.Before = opt.Arg
		case "verbose":
			opts.Verbose = true
		}
	}

	if len(p.Args) == 0 {
		return nil, errors.New("missing required <pattern> argument")
	}

	// Last positional arg is pattern; preceding args are filters.
	opts.Pattern = p.Args[len(p.Args)-1]
	if len(p.Args) > 1 {
		opts.Filters = p.Args[:len(p.Args)-1]
	}

	return opts, nil
}

// msgBody extracts the matchable text body from a message.
// Returns the body and true for text, edit, and headline messages.
// Returns ("", false) for all other types or nil content pointers.
func msgBody(msg keybase.MsgSummary) (string, bool) {
	switch msg.Content.Type {
	case "text":
		if msg.Content.Text == nil {
			return "", false
		}
		return msg.Content.Text.Body, true
	case "edit":
		if msg.Content.Edit == nil {
			return "", false
		}
		return msg.Content.Edit.Body, true
	case "headline":
		if msg.Content.Headline == nil {
			return "", false
		}
		return msg.Content.Headline.Headline, true
	default:
		return "", false
	}
}

// compileMatcher returns a match function for the given pattern.
// Glob mode (isRegexp=false): converts glob to regexp, anchored for full-body match.
// Regexp mode (isRegexp=true): uses pattern directly, unanchored (substring match).
// Case-insensitive (icase=true): prepends (?i) to the compiled regexp.
func compileMatcher(pattern string, isRegexp, icase bool) (func(string) bool, error) {
	var rePattern string

	if isRegexp {
		rePattern = pattern
	} else {
		// Escape all regexp metacharacters, then convert glob wildcards.
		escaped := regexp.QuoteMeta(pattern)
		escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
		escaped = strings.ReplaceAll(escaped, `\?`, `.`)
		rePattern = "^" + escaped + "$"
	}

	if icase {
		rePattern = "(?i)" + rePattern
	}

	re, err := regexp.Compile(rePattern)
	if err != nil {
		return nil, fmt.Errorf("compiling pattern %q: %w", pattern, err)
	}

	return re.MatchString, nil
}

// window represents a contiguous range of message indices.
// Start is inclusive, End is exclusive.
type window struct {
	Start int
	End   int
}

// contextWindows expands match indices by ctxB before and ctxA after,
// clamps to [0, msgLen), and merges overlapping/adjacent ranges into
// sorted, non-overlapping windows.
func contextWindows(matchIdxs []int, msgLen, ctxB, ctxA int) []window {
	if len(matchIdxs) == 0 {
		return nil
	}

	windows := make([]window, 0, len(matchIdxs))
	for _, idx := range matchIdxs {
		start := max(idx-ctxB, 0)
		end := min(idx+1+ctxA, msgLen)

		// Merge with previous window if overlapping or adjacent.
		if n := len(windows); n > 0 && start <= windows[n-1].End {
			if end > windows[n-1].End {
				windows[n-1].End = end
			}
		} else {
			windows = append(windows, window{Start: start, End: end})
		}
	}
	return windows
}

// RunGrep executes the grep subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunGrep(args []string, cfg *config.Config) error {
	return runGrep(args, cfg, os.Stdout, time.Now())
}

// runGrep is the internal implementation of RunGrep, accepting injectable
// dependencies for testing. w is the output writer, now is the reference
// time for timestamp parsing.
func runGrep(args []string, cfg *config.Config, w io.Writer, now time.Time) error {
	opts, err := parseGrepArgs(args)
	if err != nil {
		return err
	}

	matcher, err := compileMatcher(opts.Pattern, opts.Regexp, opts.ICase)
	if err != nil {
		return err
	}

	// Parse --after/--before timestamps.
	var after, before *time.Time
	if opts.After != "" {
		t, perr := dateparse.Parse(opts.After, now)
		if perr != nil {
			return fmt.Errorf("parsing --after: %w", perr)
		}
		after = &t
	}
	if opts.Before != "" {
		t, perr := dateparse.Parse(opts.Before, now)
		if perr != nil {
			return fmt.Errorf("parsing --before: %w", perr)
		}
		before = &t
	}

	// Scan and filter conversations.
	convs, err := store.ScanConversations(cfg.StorePath)
	if err != nil {
		return fmt.Errorf("scanning conversations: %w", err)
	}

	if len(opts.Filters) > 0 {
		convs = store.FilterConvInfos(convs, opts.Filters)
	}

	// Sort conversations by path for deterministic output.
	sort.Slice(convs, func(i, j int) bool {
		return store.ConvInfoPath(convs[i]) < store.ConvInfoPath(convs[j])
	})

	remaining := opts.Count // 0 = unlimited
	printed := false
	timeFmt := cfg.TimeFmt()

	for _, conv := range convs {
		msgs, rerr := store.ReadMessages(conv.Dir, 0)
		if rerr != nil {
			return fmt.Errorf("reading messages from %s: %w", store.ConvInfoPath(conv), rerr)
		}

		msgs = filterByTimestamp(msgs, after, before)

		// Collect match indices.
		var matchIdxs []int
		for i, msg := range msgs {
			body, ok := msgBody(msg)
			if !ok {
				continue
			}
			if matcher(body) {
				matchIdxs = append(matchIdxs, i)
			}
		}

		if len(matchIdxs) == 0 {
			continue
		}

		// Apply count limit: truncate match indices if needed.
		if remaining > 0 && len(matchIdxs) > remaining {
			matchIdxs = matchIdxs[:remaining]
		}

		windows := contextWindows(matchIdxs, len(msgs), opts.CtxB, opts.CtxA)

		// Print separator between conversation blocks.
		if printed {
			fmt.Fprintln(w, "--")
		}

		// Print conversation header.
		fmt.Fprintf(w, "==> %s <==\n", store.ConvInfoPath(conv))

		// Print windows with blank line between non-contiguous ones.
		for wi, win := range windows {
			if wi > 0 {
				fmt.Fprintln(w)
			}
			for i := win.Start; i < win.End; i++ {
				fmt.Fprintln(w, FormatMsg(msgs[i], timeFmt, opts.Verbose))
			}
		}

		// Decrement remaining by number of matches in this conversation.
		if remaining > 0 {
			remaining -= len(matchIdxs)
			if remaining <= 0 {
				break
			}
		}

		printed = true
	}

	return nil
}
