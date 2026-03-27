package export

import (
	"strings"

	"github.com/major0/keybase-export/keybase"
)

// FilterConversations returns conversations matching the given filters.
// Filters are path prefixes like "Chat/alice,bob" or "Team/myteam".
// An empty filter list returns all conversations.
func FilterConversations(convs []keybase.ConvSummary, filters []string, selfUsername string) []keybase.ConvSummary {
	if len(filters) == 0 {
		return convs
	}

	var result []keybase.ConvSummary
	for _, conv := range convs {
		path := ConvDirPath("", conv, selfUsername)
		for _, f := range filters {
			// Normalize: ConvDirPath with empty destDir gives "/Chats/..." or "/Teams/..."
			// Filter format is "Chat/..." or "Team/..."
			// Match by checking if the path contains the filter as a prefix component
			if matchesFilter(path, f) {
				result = append(result, conv)
				break
			}
		}
	}
	return result
}

// matchesFilter checks if a conversation path matches a filter string.
// path is like "/Chats/alice,bob" or "/Teams/myteam/general"
// filter is like "Chat/alice,bob" or "Team/myteam"
func matchesFilter(path, filter string) bool {
	// Normalize path: strip leading separator
	path = strings.TrimPrefix(path, "/")

	// Map filter prefixes to directory names
	// "Chat/" → "Chats/", "Team/" → "Teams/"
	normalized := filter
	if strings.HasPrefix(filter, "Chat/") {
		normalized = "Chats/" + filter[len("Chat/"):]
	} else if strings.HasPrefix(filter, "Team/") {
		normalized = "Teams/" + filter[len("Team/"):]
	}

	return strings.HasPrefix(path, normalized)
}

// FilterMessagesByTimestamp returns messages with SentAtMs strictly greater
// than the cutoff timestamp, preserving relative order.
func FilterMessagesByTimestamp(msgs []keybase.MsgSummary, cutoff int64) []keybase.MsgSummary {
	var result []keybase.MsgSummary
	for _, m := range msgs {
		if m.SentAtMs > cutoff {
			result = append(result, m)
		}
	}
	return result
}
