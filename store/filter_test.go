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
