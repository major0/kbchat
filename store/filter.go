package store

import (
	"path"
	"strings"
)

// FilterConvInfos returns conversations matching the given filters.
// Filters use the same syntax as export.FilterConversations:
// "Chat/<participants>" or "Team/<team_name>". An empty filter list
// returns all conversations.
func FilterConvInfos(convs []ConvInfo, filters []string) []ConvInfo {
	if len(filters) == 0 {
		return convs
	}

	var result []ConvInfo
	for _, conv := range convs {
		p := convInfoPath(conv)
		for _, f := range filters {
			if matchesConvFilter(p, f) {
				result = append(result, conv)
				break
			}
		}
	}
	return result
}

// convInfoPath builds the relative path for a ConvInfo, matching the
// directory layout used by export.ConvDirPath:
//
//	Chat → "Chats/<Name>"
//	Team → "Teams/<Name>/<Channel>"
func convInfoPath(conv ConvInfo) string {
	switch conv.Type {
	case "Team":
		return path.Join("Teams", conv.Name, conv.Channel)
	default:
		return path.Join("Chats", conv.Name)
	}
}

// matchesConvFilter checks if a conversation path matches a filter string.
// This mirrors export.matchesFilter: "Chat/" maps to "Chats/", "Team/" maps
// to "Teams/", then checks exact match or prefix with trailing "/".
func matchesConvFilter(convPath, filter string) bool {
	normalized := filter
	if strings.HasPrefix(filter, "Chat/") {
		normalized = "Chats/" + filter[len("Chat/"):]
	} else if strings.HasPrefix(filter, "Team/") {
		normalized = "Teams/" + filter[len("Team/"):]
	}

	return convPath == normalized || strings.HasPrefix(convPath, normalized+"/")
}
