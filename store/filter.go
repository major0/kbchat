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
// "Chat/" maps to "Chats/", "Team/" maps to "Teams/". When the normalized
// filter contains glob metacharacters (* or ?), glob matching is used.
// Otherwise the original prefix behavior is preserved.
func matchesConvFilter(convPath, filter string) bool {
	normalized := filter
	if strings.HasPrefix(filter, "Chat/") {
		normalized = "Chats/" + filter[len("Chat/"):]
	} else if strings.HasPrefix(filter, "Team/") {
		normalized = "Teams/" + filter[len("Team/"):]
	}

	if hasGlobMeta(normalized) {
		return globMatch(convPath, normalized)
	}
	return convPath == normalized || strings.HasPrefix(convPath, normalized+"/")
}

// hasGlobMeta reports whether s contains glob metacharacters (* or ?).
func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?")
}

// globMatch matches a path against a glob pattern by splitting both on "/"
// and matching segment-by-segment. "**" matches zero or more segments via
// backtracking. Within a segment, "*" and "?" follow path.Match semantics.
func globMatch(p, pattern string) bool {
	pathSegs := strings.Split(p, "/")
	patSegs := strings.Split(pattern, "/")
	return matchSegments(pathSegs, 0, patSegs, 0)
}

// matchSegments recursively matches path segments against pattern segments.
// A "**" pattern segment matches zero or more path segments.
func matchSegments(pathSegs []string, pi int, patSegs []string, qi int) bool {
	for pi < len(pathSegs) && qi < len(patSegs) {
		if patSegs[qi] == "**" {
			// "**" matches zero or more segments — try consuming
			// 0..N path segments for this "**".
			qi++
			// Consecutive "**" collapse to one.
			for qi < len(patSegs) && patSegs[qi] == "**" {
				qi++
			}
			if qi == len(patSegs) {
				// Trailing "**" matches everything remaining.
				return true
			}
			// Try matching the rest of the pattern starting at each
			// remaining path position.
			for k := pi; k <= len(pathSegs); k++ {
				if matchSegments(pathSegs, k, patSegs, qi) {
					return true
				}
			}
			return false
		}
		if !segmentMatch(pathSegs[pi], patSegs[qi]) {
			return false
		}
		pi++
		qi++
	}
	// Consume any trailing "**" pattern segments.
	for qi < len(patSegs) && patSegs[qi] == "**" {
		qi++
	}
	return pi == len(pathSegs) && qi == len(patSegs)
}

// segmentMatch matches a single path segment against a pattern segment
// using path.Match semantics (*, ?, and character classes).
func segmentMatch(seg, pat string) bool {
	matched, err := path.Match(pat, seg)
	if err != nil {
		return false
	}
	return matched
}
