package store

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"testing/quick"

	"github.com/major0/kbchat/keybase"
)

// Feature: keybase-chat-cli, Property 12: Store scan discovers all conversations.
//
// For any store directory containing Chats/<name>/messages/ and
// Teams/<team>/<channel>/messages/ subdirectories, ScanConversations must
// return one ConvInfo per conversation with correct type, name, channel,
// and message count.
//
// **Validates: Requirements 4.1, 4.5**

// storeLayout describes a randomly generated store for property testing.
type storeLayout struct {
	Chats []chatEntry
	Teams []teamEntry
}

type chatEntry struct {
	Name     string
	MsgCount int
}

type teamEntry struct {
	Team     string
	Channel  string
	MsgCount int
}

// Generate implements quick.Generator for storeLayout.
func (storeLayout) Generate(rand *rand.Rand, size int) reflect.Value {
	numChats := rand.Intn(size + 1)
	numTeams := rand.Intn(size + 1)

	chats := make([]chatEntry, numChats)
	for i := range chats {
		chats[i] = chatEntry{
			Name:     fmt.Sprintf("chat%d", i),
			MsgCount: rand.Intn(10),
		}
	}

	teams := make([]teamEntry, numTeams)
	for i := range teams {
		teams[i] = teamEntry{
			Team:     fmt.Sprintf("team%d", rand.Intn(size+1)),
			Channel:  fmt.Sprintf("chan%d", i),
			MsgCount: rand.Intn(10),
		}
	}

	return reflect.ValueOf(storeLayout{Chats: chats, Teams: teams})
}

// createStore builds the on-disk directory structure for a storeLayout
// and returns the store root path.
func createStore(t *testing.T, layout storeLayout) string {
	t.Helper()
	root := t.TempDir()

	for _, c := range layout.Chats {
		msgsDir := filepath.Join(root, "Chats", c.Name, "messages")
		if err := os.MkdirAll(msgsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		createMsgDirs(t, msgsDir, c.MsgCount)
	}

	for _, tm := range layout.Teams {
		msgsDir := filepath.Join(root, "Teams", tm.Team, tm.Channel, "messages")
		if err := os.MkdirAll(msgsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		createMsgDirs(t, msgsDir, tm.MsgCount)
	}

	return root
}

// createMsgDirs creates n message subdirectories with minimal message.json files.
func createMsgDirs(t *testing.T, msgsDir string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		msgDir := filepath.Join(msgsDir, strconv.Itoa(i))
		if err := os.MkdirAll(msgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		msg := keybase.MsgSummary{ID: i, Content: keybase.MsgContent{Type: "text"}}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(msgDir, "message.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPropertyScanDiscoversAllConversations(t *testing.T) {
	f := func(layout storeLayout) bool {
		root := createStore(t, layout)

		convs, err := ScanConversations(root)
		if err != nil {
			t.Logf("ScanConversations error: %v", err)
			return false
		}

		// Deduplicate team entries (same team+channel → last wins in layout).
		type teamKey struct{ team, channel string }
		expectedTeams := make(map[teamKey]int)
		for _, tm := range layout.Teams {
			expectedTeams[teamKey{tm.Team, tm.Channel}] = tm.MsgCount
		}

		expectedTotal := len(layout.Chats) + len(expectedTeams)
		if len(convs) != expectedTotal {
			t.Logf("expected %d conversations, got %d", expectedTotal, len(convs))
			return false
		}

		// Build lookup maps from results.
		chatResults := make(map[string]ConvInfo)
		teamResults := make(map[teamKey]ConvInfo)
		for _, c := range convs {
			switch c.Type {
			case "Chat":
				chatResults[c.Name] = c
			case "Team":
				teamResults[teamKey{c.Name, c.Channel}] = c
			default:
				t.Logf("unexpected type: %s", c.Type)
				return false
			}
		}

		// Verify chats.
		for _, c := range layout.Chats {
			ci, ok := chatResults[c.Name]
			if !ok {
				t.Logf("missing chat: %s", c.Name)
				return false
			}
			if ci.MsgCount != c.MsgCount {
				t.Logf("chat %s: expected %d msgs, got %d", c.Name, c.MsgCount, ci.MsgCount)
				return false
			}
			if ci.Channel != "" {
				t.Logf("chat %s: expected empty channel, got %q", c.Name, ci.Channel)
				return false
			}
		}

		// Verify teams.
		for key, expectedCount := range expectedTeams {
			ci, ok := teamResults[key]
			if !ok {
				t.Logf("missing team: %s/%s", key.team, key.channel)
				return false
			}
			if ci.MsgCount != expectedCount {
				t.Logf("team %s/%s: expected %d msgs, got %d", key.team, key.channel, expectedCount, ci.MsgCount)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

func TestScanConversations(t *testing.T) {
	tests := []struct {
		name   string
		layout storeLayout
		want   []ConvInfo // Dir field is ignored in comparison
	}{
		{
			name:   "empty store",
			layout: storeLayout{},
			want:   nil,
		},
		{
			name: "chats only",
			layout: storeLayout{
				Chats: []chatEntry{
					{Name: "alice,bob", MsgCount: 3},
					{Name: "carol", MsgCount: 0},
				},
			},
			want: []ConvInfo{
				{Type: "Chat", Name: "alice,bob", MsgCount: 3},
				{Type: "Chat", Name: "carol", MsgCount: 0},
			},
		},
		{
			name: "teams only",
			layout: storeLayout{
				Teams: []teamEntry{
					{Team: "engineering", Channel: "general", MsgCount: 5},
				},
			},
			want: []ConvInfo{
				{Type: "Team", Name: "engineering", Channel: "general", MsgCount: 5},
			},
		},
		{
			name: "mixed chats and teams",
			layout: storeLayout{
				Chats: []chatEntry{
					{Name: "alice,bob", MsgCount: 2},
				},
				Teams: []teamEntry{
					{Team: "devops", Channel: "alerts", MsgCount: 7},
				},
			},
			want: []ConvInfo{
				{Type: "Chat", Name: "alice,bob", MsgCount: 2},
				{Type: "Team", Name: "devops", Channel: "alerts", MsgCount: 7},
			},
		},
		{
			name: "nested team channels",
			layout: storeLayout{
				Teams: []teamEntry{
					{Team: "platform", Channel: "general", MsgCount: 4},
					{Team: "platform", Channel: "backend", MsgCount: 1},
					{Team: "platform", Channel: "frontend", MsgCount: 6},
				},
			},
			want: []ConvInfo{
				{Type: "Team", Name: "platform", Channel: "backend", MsgCount: 1},
				{Type: "Team", Name: "platform", Channel: "frontend", MsgCount: 6},
				{Type: "Team", Name: "platform", Channel: "general", MsgCount: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := createStore(t, tt.layout)
			got, err := ScanConversations(root)
			if err != nil {
				t.Fatalf("ScanConversations: %v", err)
			}

			// Sort both slices for deterministic comparison.
			sortConvInfos(got)
			sortConvInfos(tt.want)

			if len(got) == 0 && len(tt.want) == 0 {
				return // both empty
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d conversations, want %d", len(got), len(tt.want))
			}

			for i := range got {
				g, w := got[i], tt.want[i]
				if g.Type != w.Type || g.Name != w.Name || g.Channel != w.Channel || g.MsgCount != w.MsgCount {
					t.Errorf("conv[%d] = {Type:%q Name:%q Channel:%q MsgCount:%d}, want {Type:%q Name:%q Channel:%q MsgCount:%d}",
						i, g.Type, g.Name, g.Channel, g.MsgCount, w.Type, w.Name, w.Channel, w.MsgCount)
				}
			}
		})
	}
}

// sortConvInfos sorts by Type, then Name, then Channel for deterministic comparison.
func sortConvInfos(convs []ConvInfo) {
	sort.Slice(convs, func(i, j int) bool {
		if convs[i].Type != convs[j].Type {
			return convs[i].Type < convs[j].Type
		}
		if convs[i].Name != convs[j].Name {
			return convs[i].Name < convs[j].Name
		}
		return convs[i].Channel < convs[j].Channel
	})
}
