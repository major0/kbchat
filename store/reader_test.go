package store

import (
	"encoding/json"
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

// Feature: keybase-chat-cli, Property 13: Message reading returns correct count and order.
//
// For any conversation directory with N messages, ReadMessages(dir, 0) must
// return all N messages sorted by ID ascending. ReadMessages(dir, k) for
// k ≤ N must return the last k messages sorted by ID ascending.
//
// **Validates: Requirements 5.6, 5.7, 5.8**

// readerInput holds a randomly generated set of message IDs and a count parameter.
type readerInput struct {
	MsgIDs []int // unique positive IDs to write
	Count  int   // count parameter for ReadMessages
}

// Generate implements quick.Generator for readerInput.
func (readerInput) Generate(rand *rand.Rand, size int) reflect.Value {
	n := rand.Intn(size + 1)

	// Generate unique positive IDs.
	idSet := make(map[int]struct{}, n)
	for len(idSet) < n {
		idSet[rand.Intn(1000)+1] = struct{}{}
	}
	ids := make([]int, 0, n)
	for id := range idSet {
		ids = append(ids, id)
	}

	// Count: 0 means all, otherwise 0..n+2 to cover count > N.
	count := rand.Intn(n + 3)

	return reflect.ValueOf(readerInput{MsgIDs: ids, Count: count})
}

func TestPropertyReadMessagesCountAndOrder(t *testing.T) {
	f := func(input readerInput) bool {
		convDir := t.TempDir()
		msgsDir := filepath.Join(convDir, "messages")
		writeMsgsByID(t, msgsDir, input.MsgIDs)

		n := len(input.MsgIDs)
		sorted := make([]int, n)
		copy(sorted, input.MsgIDs)
		sort.Ints(sorted)

		msgs, err := ReadMessages(convDir, input.Count)
		if err != nil {
			t.Logf("ReadMessages error: %v", err)
			return false
		}

		// Determine expected IDs.
		var expected []int
		if input.Count == 0 || input.Count >= n {
			expected = sorted
		} else {
			expected = sorted[n-input.Count:]
		}

		if len(msgs) != len(expected) {
			t.Logf("count=%d, n=%d: got %d msgs, want %d", input.Count, n, len(msgs), len(expected))
			return false
		}

		// Verify correct IDs in ascending order.
		for i, msg := range msgs {
			if msg.ID != expected[i] {
				t.Logf("msgs[%d].ID = %d, want %d", i, msg.ID, expected[i])
				return false
			}
		}

		// Verify strictly ascending.
		for i := 1; i < len(msgs); i++ {
			if msgs[i].ID <= msgs[i-1].ID {
				t.Logf("not ascending: msgs[%d].ID=%d <= msgs[%d].ID=%d",
					i, msgs[i].ID, i-1, msgs[i-1].ID)
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// writeMsgsByID creates messages/<id>/message.json for each ID in the list.
func writeMsgsByID(t *testing.T, msgsDir string, ids []int) {
	t.Helper()
	for _, id := range ids {
		dir := filepath.Join(msgsDir, strconv.Itoa(id))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		msg := keybase.MsgSummary{ID: id, Content: keybase.MsgContent{Type: "text"}}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "message.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadMessages(t *testing.T) {
	tests := []struct {
		name    string
		msgIDs  []int // message IDs to create; nil means don't create messages dir
		count   int
		wantIDs []int
		wantErr bool
	}{
		{
			name:    "empty conversation (no messages dir)",
			msgIDs:  nil,
			count:   0,
			wantIDs: nil,
		},
		{
			name:    "count=0 returns all sorted ascending",
			msgIDs:  []int{5, 2, 8, 1, 3},
			count:   0,
			wantIDs: []int{1, 2, 3, 5, 8},
		},
		{
			name:    "count > N returns all N",
			msgIDs:  []int{10, 20, 30},
			count:   10,
			wantIDs: []int{10, 20, 30},
		},
		{
			name:    "count = N returns all N",
			msgIDs:  []int{4, 2, 6},
			count:   3,
			wantIDs: []int{2, 4, 6},
		},
		{
			name:    "single message count=0",
			msgIDs:  []int{42},
			count:   0,
			wantIDs: []int{42},
		},
		{
			name:    "single message count=1",
			msgIDs:  []int{42},
			count:   1,
			wantIDs: []int{42},
		},
		{
			name:    "count=2 from 5 messages returns last 2",
			msgIDs:  []int{1, 3, 5, 7, 9},
			count:   2,
			wantIDs: []int{7, 9},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			convDir := t.TempDir()

			if tt.msgIDs != nil {
				msgsDir := filepath.Join(convDir, "messages")
				writeMsgsByID(t, msgsDir, tt.msgIDs)
			}

			got, err := ReadMessages(convDir, tt.count)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadMessages() error = %v, wantErr %v", err, tt.wantErr)
			}

			var gotIDs []int
			for _, m := range got {
				gotIDs = append(gotIDs, m.ID)
			}

			if !reflect.DeepEqual(gotIDs, tt.wantIDs) {
				t.Errorf("ReadMessages() IDs = %v, want %v", gotIDs, tt.wantIDs)
			}
		})
	}
}
