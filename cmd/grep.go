package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/keybase"
)

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
func RunGrep(_ []string, _ *config.Config) error {
	fmt.Println("grep: not implemented")
	return nil
}
