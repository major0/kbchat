package store

import (
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"

	"github.com/major0/kbchat/export"
	"github.com/major0/kbchat/keybase"
)

// Feature: keybase-chat-cli, Property 16: Store filter matching is consistent with export filter matching.
//
// For any set of ConvInfo values and filter strings, store.FilterConvInfos
// must return the same conversations that export.FilterConversations would
// match for equivalent inputs.
//
// **Validates: Requirements 7.5**

// filterInput holds randomly generated conversations and filters for property testing.
type filterInput struct {
	Convs   []ConvInfo
	Filters []string
}

// Generate implements quick.Generator for filterInput.
func (filterInput) Generate(rand *rand.Rand, size int) reflect.Value {
	// Generate a small set of conversations.
	numConvs := rand.Intn(size + 1)
	convs := make([]ConvInfo, numConvs)

	names := []string{"alice", "bob", "carol", "dave", "eve"}
	teams := []string{"engineering", "devops", "platform", "security"}
	channels := []string{"general", "random", "alerts", "backend"}

	for i := range convs {
		if rand.Intn(2) == 0 {
			// Chat conversation: pick 1-3 participants, comma-joined.
			n := 1 + rand.Intn(3)
			picked := make([]string, n)
			for j := range picked {
				picked[j] = names[rand.Intn(len(names))]
			}
			sort.Strings(picked)
			convs[i] = ConvInfo{
				Type: "Chat",
				Name: strings.Join(picked, ","),
			}
		} else {
			// Team conversation.
			convs[i] = ConvInfo{
				Type:    "Team",
				Name:    teams[rand.Intn(len(teams))],
				Channel: channels[rand.Intn(len(channels))],
			}
		}
	}

	// Generate 0-3 filters, sometimes matching, sometimes not.
	numFilters := rand.Intn(4)
	filters := make([]string, numFilters)
	for i := range filters {
		if len(convs) > 0 && rand.Intn(2) == 0 {
			// Derive filter from an existing conversation for higher match rate.
			c := convs[rand.Intn(len(convs))]
			if c.Type == "Chat" {
				filters[i] = "Chat/" + c.Name
			} else {
				filters[i] = "Team/" + c.Name
			}
		} else {
			// Random filter that may or may not match.
			if rand.Intn(2) == 0 {
				filters[i] = "Chat/" + names[rand.Intn(len(names))]
			} else {
				filters[i] = "Team/" + teams[rand.Intn(len(teams))]
			}
		}
	}

	return reflect.ValueOf(filterInput{Convs: convs, Filters: filters})
}

// convInfoToConvSummary builds a keybase.ConvSummary equivalent to a ConvInfo.
// For Chat: MembersType="impteamnative", Name=participants (with a self user prepended).
// For Team: MembersType="team", Name=team name, TopicName=channel.
func convInfoToConvSummary(ci ConvInfo, selfUsername string) keybase.ConvSummary {
	switch ci.Type {
	case "Team":
		return keybase.ConvSummary{
			Channel: keybase.ChatChannel{
				Name:        ci.Name,
				MembersType: "team",
				TopicName:   ci.Channel,
			},
		}
	default:
		// Chat: the export path strips selfUsername and sorts. To reverse
		// this, prepend selfUsername so ConvDirPath("", conv, self) produces
		// "Chats/<ci.Name>" (with self removed and remainder sorted).
		name := selfUsername + "," + ci.Name
		return keybase.ConvSummary{
			Channel: keybase.ChatChannel{
				Name:        name,
				MembersType: "impteamnative",
			},
		}
	}
}

func TestPropertyStoreFilterConsistentWithExportFilter(t *testing.T) {
	const selfUsername = "testself"

	f := func(input filterInput) bool {
		// Run store filter.
		storeResult := FilterConvInfos(input.Convs, input.Filters)

		// Build equivalent ConvSummary slice.
		summaries := make([]keybase.ConvSummary, len(input.Convs))
		for i, ci := range input.Convs {
			summaries[i] = convInfoToConvSummary(ci, selfUsername)
		}

		// Run export filter.
		exportResult := export.FilterConversations(summaries, input.Filters, selfUsername)

		// Both should select the same indices.
		storeNames := convInfoKeys(storeResult)
		exportNames := summaryKeys(exportResult, selfUsername)

		sort.Strings(storeNames)
		sort.Strings(exportNames)

		if len(storeNames) != len(exportNames) {
			t.Logf("store returned %d, export returned %d", len(storeNames), len(exportNames))
			t.Logf("store: %v", storeNames)
			t.Logf("export: %v", exportNames)
			return false
		}
		for i := range storeNames {
			if storeNames[i] != exportNames[i] {
				t.Logf("mismatch at %d: store=%q export=%q", i, storeNames[i], exportNames[i])
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// convInfoKeys returns a canonical key for each ConvInfo for comparison.
func convInfoKeys(convs []ConvInfo) []string {
	keys := make([]string, len(convs))
	for i, c := range convs {
		if c.Type == "Team" {
			keys[i] = "Teams/" + c.Name + "/" + c.Channel
		} else {
			keys[i] = "Chats/" + c.Name
		}
	}
	return keys
}

// summaryKeys returns a canonical key for each ConvSummary using ConvDirPath.
func summaryKeys(convs []keybase.ConvSummary, selfUsername string) []string {
	keys := make([]string, len(convs))
	for i, c := range convs {
		// ConvDirPath with empty destDir gives "/Chats/..." or "/Teams/..."
		p := export.ConvDirPath("", c, selfUsername)
		keys[i] = strings.TrimPrefix(p, "/")
	}
	return keys
}

// Feature: keybase-chat-list, Property 1: Glob filter is a superset of prefix filter.
//
// For any conversation path and filter string containing no glob metacharacters,
// matchesConvFilter with glob support returns the same result as the original
// prefix-only implementation.
//
// **Validates: Req 1.4, CLI Req 5.5**

// prefixOnlyMatch is the original prefix-only implementation for comparison.
func prefixOnlyMatch(convPath, filter string) bool {
	normalized := filter
	if strings.HasPrefix(filter, "Chat/") {
		normalized = "Chats/" + filter[len("Chat/"):]
	} else if strings.HasPrefix(filter, "Team/") {
		normalized = "Teams/" + filter[len("Team/"):]
	}
	return convPath == normalized || strings.HasPrefix(convPath, normalized+"/")
}

// nonGlobFilter generates filter strings without glob metacharacters.
type nonGlobFilter struct {
	ConvPath string
	Filter   string
}

// Generate implements quick.Generator for nonGlobFilter.
func (nonGlobFilter) Generate(rand *rand.Rand, size int) reflect.Value {
	names := []string{"alice", "bob", "carol", "alice,bob", "bob,carol"}
	teams := []string{"engineering", "devops", "platform"}
	channels := []string{"general", "random", "alerts"}

	// Generate a conversation path.
	var convPath string
	if rand.Intn(2) == 0 {
		convPath = "Chats/" + names[rand.Intn(len(names))]
	} else {
		convPath = "Teams/" + teams[rand.Intn(len(teams))] + "/" + channels[rand.Intn(len(channels))]
	}

	// Generate a non-glob filter (no *, ?).
	var filter string
	switch rand.Intn(4) {
	case 0:
		// Exact match using Chat/ prefix.
		filter = "Chat/" + names[rand.Intn(len(names))]
	case 1:
		// Team prefix (matches all channels).
		filter = "Team/" + teams[rand.Intn(len(teams))]
	case 2:
		// Team exact channel.
		filter = "Team/" + teams[rand.Intn(len(teams))] + "/" + channels[rand.Intn(len(channels))]
	default:
		// Derived from the conversation path itself.
		if strings.HasPrefix(convPath, "Chats/") {
			filter = "Chat/" + convPath[len("Chats/"):]
		} else {
			// Use team prefix (drop channel).
			parts := strings.SplitN(convPath, "/", 3)
			filter = "Team/" + parts[1]
		}
	}

	return reflect.ValueOf(nonGlobFilter{ConvPath: convPath, Filter: filter})
}

func TestPropertyGlobFilterSupersetOfPrefixFilter(t *testing.T) {
	f := func(input nonGlobFilter) bool {
		globResult := matchesConvFilter(input.ConvPath, input.Filter)
		prefixResult := prefixOnlyMatch(input.ConvPath, input.Filter)
		if globResult != prefixResult {
			t.Logf("path=%q filter=%q glob=%v prefix=%v", input.ConvPath, input.Filter, globResult, prefixResult)
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// Feature: keybase-chat-list, Property 3: Empty filter returns all conversations.
//
// FilterConvInfos(convs, nil) returns all conversations unchanged.
//
// **Validates: Req 1.2**

func TestPropertyEmptyFilterReturnsAll(t *testing.T) {
	f := func(input filterInput) bool {
		result := FilterConvInfos(input.Convs, nil)
		if len(result) != len(input.Convs) {
			t.Logf("input=%d result=%d", len(input.Convs), len(result))
			return false
		}
		for i := range result {
			if result[i] != input.Convs[i] {
				t.Logf("mismatch at %d: got %+v want %+v", i, result[i], input.Convs[i])
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// Feature: keybase-chat-list, Property 4: Glob ** matches across segments.
//
// **/<lastSegment> matches iff path ends with that segment;
// **/x does not match paths without segment x.
//
// **Validates: CLI Req 5.4**

// doubleStarInput generates a conversation path and a last-segment for ** testing.
type doubleStarInput struct {
	ConvPath    string
	LastSegment string
}

// Generate implements quick.Generator for doubleStarInput.
func (doubleStarInput) Generate(rand *rand.Rand, size int) reflect.Value {
	names := []string{"alice", "bob", "carol", "alice,bob"}
	teams := []string{"engineering", "devops", "platform"}
	channels := []string{"general", "random", "alerts"}
	segments := make([]string, 0, len(names)+len(teams)+len(channels))
	segments = append(segments, names...)
	segments = append(segments, teams...)
	segments = append(segments, channels...)

	// Generate a conversation path.
	var convPath string
	if rand.Intn(2) == 0 {
		convPath = "Chats/" + names[rand.Intn(len(names))]
	} else {
		convPath = "Teams/" + teams[rand.Intn(len(teams))] + "/" + channels[rand.Intn(len(channels))]
	}

	// Pick a last segment — sometimes from the path, sometimes not.
	var lastSeg string
	if rand.Intn(2) == 0 {
		// Pick the actual last segment of the path.
		parts := strings.Split(convPath, "/")
		lastSeg = parts[len(parts)-1]
	} else {
		// Pick a random segment that may or may not appear.
		lastSeg = segments[rand.Intn(len(segments))]
	}

	return reflect.ValueOf(doubleStarInput{ConvPath: convPath, LastSegment: lastSeg})
}

func TestPropertyDoubleStarMatchesAcrossSegments(t *testing.T) {
	f := func(input doubleStarInput) bool {
		filter := "**/" + input.LastSegment
		result := matchesConvFilter(input.ConvPath, filter)

		// The path ends with the last segment iff the final segment equals it.
		parts := strings.Split(input.ConvPath, "/")
		endsWithSeg := parts[len(parts)-1] == input.LastSegment

		if result != endsWithSeg {
			t.Logf("path=%q filter=%q result=%v endsWithSeg=%v", input.ConvPath, filter, result, endsWithSeg)
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestGlobMatching covers table-driven glob pattern tests for matchesConvFilter.
func TestGlobMatching(t *testing.T) {
	tests := []struct {
		name     string
		convPath string
		filter   string
		want     bool
	}{
		// Chat/*bob* — glob within a segment
		{
			name:     "Chat/*bob* matches chat containing bob",
			convPath: "Chats/alice,bob",
			filter:   "Chat/*bob*",
			want:     true,
		},
		{
			name:     "Chat/*bob* does not match chat without bob",
			convPath: "Chats/alice,carol",
			filter:   "Chat/*bob*",
			want:     false,
		},

		// Team/eng/* — glob matches any channel
		{
			name:     "Team/eng/* matches team channel",
			convPath: "Teams/eng/general",
			filter:   "Team/eng/*",
			want:     true,
		},
		{
			name:     "Team/eng/* does not match different team",
			convPath: "Teams/devops/alerts",
			filter:   "Team/eng/*",
			want:     false,
		},

		// **/*bob* — recursive glob
		{
			name:     "**/*bob* matches chat with bob",
			convPath: "Chats/alice,bob",
			filter:   "**/*bob*",
			want:     true,
		},
		{
			name:     "**/*bob* does not match chat without bob",
			convPath: "Chats/carol",
			filter:   "**/*bob*",
			want:     false,
		},

		// Team/eng/gen?ral — single char wildcard
		{
			name:     "Team/eng/gen?ral matches general",
			convPath: "Teams/eng/general",
			filter:   "Team/eng/gen?ral",
			want:     true,
		},
		{
			name:     "Team/eng/gen?ral does not match random",
			convPath: "Teams/eng/random",
			filter:   "Team/eng/gen?ral",
			want:     false,
		},

		// **/general — recursive match on channel name
		{
			name:     "**/general matches team general channel",
			convPath: "Teams/eng/general",
			filter:   "**/general",
			want:     true,
		},
		{
			name:     "**/general does not match random channel",
			convPath: "Teams/eng/random",
			filter:   "**/general",
			want:     false,
		},

		// Prefix fallback: Team/engineering → Teams/engineering/general
		{
			name:     "prefix fallback Team/engineering matches Teams/engineering/general",
			convPath: "Teams/engineering/general",
			filter:   "Team/engineering",
			want:     true,
		},

		// No match: Chat/nobody*
		{
			name:     "Chat/nobody* does not match any existing chat",
			convPath: "Chats/alice,bob",
			filter:   "Chat/nobody*",
			want:     false,
		},
		{
			name:     "Chat/nobody* does not match carol",
			convPath: "Chats/carol",
			filter:   "Chat/nobody*",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesConvFilter(tt.convPath, tt.filter)
			if got != tt.want {
				t.Errorf("matchesConvFilter(%q, %q) = %v, want %v", tt.convPath, tt.filter, got, tt.want)
			}
		})
	}
}

func TestFilterConvInfos(t *testing.T) {
	convs := []ConvInfo{
		{Type: "Chat", Name: "alice,bob"},
		{Type: "Chat", Name: "carol"},
		{Type: "Team", Name: "myteam", Channel: "general"},
		{Type: "Team", Name: "myteam", Channel: "random"},
		{Type: "Team", Name: "other", Channel: "alerts"},
	}

	tests := []struct {
		name    string
		filters []string
		want    []string // canonical keys: "Chats/<name>" or "Teams/<team>/<channel>"
	}{
		{
			name:    "empty filters returns all",
			filters: nil,
			want:    []string{"Chats/alice,bob", "Chats/carol", "Teams/myteam/general", "Teams/myteam/random", "Teams/other/alerts"},
		},
		{
			name:    "chat filter",
			filters: []string{"Chat/alice,bob"},
			want:    []string{"Chats/alice,bob"},
		},
		{
			name:    "team filter matches all channels",
			filters: []string{"Team/myteam"},
			want:    []string{"Teams/myteam/general", "Teams/myteam/random"},
		},
		{
			name:    "no match",
			filters: []string{"Chat/nobody"},
			want:    nil,
		},
		{
			name:    "multiple filters",
			filters: []string{"Chat/carol", "Team/other"},
			want:    []string{"Chats/carol", "Teams/other/alerts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterConvInfos(convs, tt.filters)
			gotKeys := convInfoKeys(got)
			sort.Strings(gotKeys)
			sort.Strings(tt.want)

			if len(gotKeys) == 0 && len(tt.want) == 0 {
				return
			}

			if !reflect.DeepEqual(gotKeys, tt.want) {
				t.Errorf("got %v, want %v", gotKeys, tt.want)
			}
		})
	}
}
