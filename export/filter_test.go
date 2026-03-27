package export

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/major0/keybase-export/keybase"
)

// Feature: keybase-go-export, Property 8: Incremental export filters by timestamp
func TestPropertyIncrementalTimestampFilter(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		n := r.Intn(20) + 1
		cutoff := r.Int63n(10000)

		msgs := make([]keybase.MsgSummary, n)
		for i := range msgs {
			msgs[i] = keybase.MsgSummary{
				ID:       i + 1,
				SentAtMs: r.Int63n(20000),
				Content:  keybase.MsgContent{Type: "text"},
			}
		}

		got := FilterMessagesByTimestamp(msgs, cutoff)

		// Verify: all returned messages have SentAtMs > cutoff
		for _, m := range got {
			if m.SentAtMs <= cutoff {
				t.Logf("message %d has SentAtMs %d <= cutoff %d", m.ID, m.SentAtMs, cutoff)
				return false
			}
		}

		// Verify: count matches expected
		expected := 0
		for _, m := range msgs {
			if m.SentAtMs > cutoff {
				expected++
			}
		}
		if len(got) != expected {
			t.Logf("count mismatch: got %d, expected %d", len(got), expected)
			return false
		}

		// Verify: relative order preserved
		j := 0
		for _, m := range msgs {
			if m.SentAtMs > cutoff {
				if got[j].ID != m.ID {
					t.Logf("order mismatch at %d: got ID %d, want %d", j, got[j].ID, m.ID)
					return false
				}
				j++
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

// Feature: keybase-go-export, Property 9: Conversation filter matching
func TestPropertyConversationFilterMatching(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		selfUser := "self"

		// Generate a mix of conversations
		convs := []keybase.ConvSummary{
			{ID: "1", Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"}},
			{ID: "2", Channel: keybase.ChatChannel{Name: "self,bob", MembersType: "impteamnative"}},
			{ID: "3", Channel: keybase.ChatChannel{Name: "self,alice,charlie", MembersType: "impteamupgrade"}},
			{ID: "4", Channel: keybase.ChatChannel{Name: "engineering", MembersType: "team", TopicName: "general"}},
			{ID: "5", Channel: keybase.ChatChannel{Name: "engineering", MembersType: "team", TopicName: "random"}},
		}

		// Empty filters → all conversations
		got := FilterConversations(convs, nil, selfUser)
		if len(got) != len(convs) {
			t.Logf("empty filter: got %d, want %d", len(got), len(convs))
			return false
		}

		// Pick a random subset of filters
		allFilters := []string{"Chat/alice", "Chat/bob", "Chat/alice,charlie", "Team/engineering"}
		nFilters := r.Intn(len(allFilters)) + 1
		filters := make([]string, nFilters)
		for i := range filters {
			filters[i] = allFilters[r.Intn(len(allFilters))]
		}

		got = FilterConversations(convs, filters, selfUser)

		// Verify: every returned conversation matches at least one filter
		for _, conv := range got {
			path := ConvDirPath("", conv, selfUser)
			matched := false
			for _, f := range filters {
				if matchesFilter(path, f) {
					matched = true
					break
				}
			}
			if !matched {
				t.Logf("conv %s path %q matched no filter in %v", conv.ID, path, filters)
				return false
			}
		}

		// Verify: no conversation that should match was excluded
		for _, conv := range convs {
			path := ConvDirPath("", conv, selfUser)
			shouldMatch := false
			for _, f := range filters {
				if matchesFilter(path, f) {
					shouldMatch = true
					break
				}
			}
			if shouldMatch {
				found := false
				for _, g := range got {
					if g.ID == conv.ID {
						found = true
						break
					}
				}
				if !found {
					t.Logf("conv %s should match but was excluded", conv.ID)
					return false
				}
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestFilterConversations_Specific(t *testing.T) {
	selfUser := "self"
	convs := []keybase.ConvSummary{
		{ID: "1", Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"}},
		{ID: "2", Channel: keybase.ChatChannel{Name: "self,bob", MembersType: "impteamnative"}},
		{ID: "3", Channel: keybase.ChatChannel{Name: "engineering", MembersType: "team", TopicName: "general"}},
	}

	tests := []struct {
		name    string
		filters []string
		wantIDs []string
	}{
		{"no filters", nil, []string{"1", "2", "3"}},
		{"chat filter", []string{"Chat/alice"}, []string{"1"}},
		{"team filter", []string{"Team/engineering"}, []string{"3"}},
		{"multiple filters", []string{"Chat/alice", "Team/engineering"}, []string{"1", "3"}},
		{"no match", []string{"Chat/nobody"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterConversations(convs, tt.filters, selfUser)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d convs, want %d", len(got), len(tt.wantIDs))
			}
			for i, g := range got {
				if g.ID != tt.wantIDs[i] {
					t.Errorf("index %d: got ID %s, want %s", i, g.ID, tt.wantIDs[i])
				}
			}
		})
	}
}
