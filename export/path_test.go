package export

import (
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/quick"

	"github.com/major0/keybase-export/keybase"
)

// Feature: keybase-go-export, Property 2: Directory path derivation
func TestPropertyDirectoryPathDerivation(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		destDir := "/export"
		selfUser := "self"

		// Randomly generate team or DM/group
		isTeam := r.Intn(2) == 0

		var conv keybase.ConvSummary
		if isTeam {
			topics := []string{"general", "random", "dev"}
			conv = keybase.ConvSummary{
				Channel: keybase.ChatChannel{
					Name:        "myteam",
					MembersType: "team",
					TopicName:   topics[r.Intn(len(topics))],
				},
			}
			got := ConvDirPath(destDir, conv, selfUser)
			want := filepath.Join(destDir, "Teams", "myteam", conv.Channel.TopicName)
			if got != want {
				t.Logf("team path: got %q, want %q", got, want)
				return false
			}
		} else {
			// Generate 1-5 participants plus self
			users := []string{selfUser}
			numOthers := r.Intn(5) + 1
			for i := 0; i < numOthers; i++ {
				users = append(users, string(rune('a'+i)))
			}
			mt := "impteamnative"
			if r.Intn(2) == 0 {
				mt = "impteamupgrade"
			}
			conv = keybase.ConvSummary{
				Channel: keybase.ChatChannel{
					Name:        strings.Join(users, ","),
					MembersType: mt,
				},
			}
			got := ConvDirPath(destDir, conv, selfUser)

			// Verify path starts with Chats/
			if !strings.HasPrefix(got, filepath.Join(destDir, "Chats")+string(filepath.Separator)) {
				t.Logf("DM/group path should start with Chats/: %q", got)
				return false
			}

			// Extract participant list from path
			participantPart := strings.TrimPrefix(got, filepath.Join(destDir, "Chats")+string(filepath.Separator))
			parts := strings.Split(participantPart, ",")

			// Self should not be in the path
			for _, p := range parts {
				if p == selfUser {
					t.Logf("self username found in path: %q", got)
					return false
				}
			}

			// Participants should be sorted
			sorted := make([]string, len(parts))
			copy(sorted, parts)
			sort.Strings(sorted)
			for i := range parts {
				if parts[i] != sorted[i] {
					t.Logf("participants not sorted in path: %q", got)
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

func TestConvDirPath_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		conv     keybase.ConvSummary
		selfUser string
		want     string
	}{
		{
			name: "single participant DM",
			conv: keybase.ConvSummary{
				Channel: keybase.ChatChannel{
					Name:        "alice,bob",
					MembersType: "impteamnative",
				},
			},
			selfUser: "alice",
			want:     filepath.Join("/export", "Chats", "bob"),
		},
		{
			name: "many-participant group",
			conv: keybase.ConvSummary{
				Channel: keybase.ChatChannel{
					Name:        "charlie,alice,bob,dave",
					MembersType: "impteamupgrade",
				},
			},
			selfUser: "alice",
			want:     filepath.Join("/export", "Chats", "bob,charlie,dave"),
		},
		{
			name: "team with default channel",
			conv: keybase.ConvSummary{
				Channel: keybase.ChatChannel{
					Name:        "engineering",
					MembersType: "team",
					TopicName:   "general",
				},
			},
			selfUser: "alice",
			want:     filepath.Join("/export", "Teams", "engineering", "general"),
		},
		{
			name: "team with empty topic defaults to general",
			conv: keybase.ConvSummary{
				Channel: keybase.ChatChannel{
					Name:        "engineering",
					MembersType: "team",
					TopicName:   "",
				},
			},
			selfUser: "alice",
			want:     filepath.Join("/export", "Teams", "engineering", "general"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvDirPath("/export", tt.conv, tt.selfUser)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
