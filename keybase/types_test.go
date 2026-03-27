package keybase

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
)

// Feature: keybase-go-export, Property 1: Conversation classification is deterministic and correct
func TestPropertyConversationClassification(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		// Generate a random ChatChannel
		membersTypes := []string{"team", "impteamnative", "impteamupgrade"}
		mt := membersTypes[r.Intn(len(membersTypes))]

		// Generate participant name
		numParticipants := r.Intn(5) + 1
		name := "user0"
		for i := 1; i < numParticipants; i++ {
			name += ",user" + string(rune('0'+i))
		}
		if mt == "team" {
			name = "myteam"
		}

		ch := ChatChannel{
			Name:        name,
			MembersType: mt,
			TopicName:   "general",
		}

		result := ClassifyConversation(ch)

		// Determinism: same input produces same output
		result2 := ClassifyConversation(ch)
		if result != result2 {
			t.Logf("non-deterministic: %v vs %v for %+v", result, result2, ch)
			return false
		}

		// Correctness
		switch mt {
		case "team":
			if result != ConvTeam {
				t.Logf("expected ConvTeam for members_type=team, got %v", result)
				return false
			}
		case "impteamnative", "impteamupgrade":
			if numParticipants <= 2 {
				if result != ConvDM {
					t.Logf("expected ConvDM for %d participants, got %v", numParticipants, result)
					return false
				}
			} else {
				if result != ConvGroup {
					t.Logf("expected ConvGroup for %d participants, got %v", numParticipants, result)
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

// Feature: keybase-go-export, Property 3: Message serialization round-trip
func TestPropertyMessageSerializationRoundTrip(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		contentTypes := []string{"text", "edit", "delete", "reaction", "attachment", "metadata", "headline"}
		ct := contentTypes[r.Intn(len(contentTypes))]

		msg := MsgSummary{
			ID:             r.Intn(100000),
			ConversationID: "conv123",
			Channel: ChatChannel{
				Name:        "alice,bob",
				MembersType: "impteamnative",
				TopicType:   "chat",
			},
			Sender: MsgSender{
				UID:        "uid123",
				Username:   "alice",
				DeviceID:   "dev123",
				DeviceName: "phone",
			},
			SentAt:   r.Int63(),
			SentAtMs: r.Int63(),
			Content:  MsgContent{Type: ct},
			Prev: []Prev{
				{ID: r.Intn(1000), Hash: "abc123"},
			},
			AtMentionUsernames: []string{"bob"},
			ChannelMention:     "none",
		}

		// Set content based on type
		switch ct {
		case "text":
			msg.Content.Text = &TextContent{Body: "hello world"}
		case "edit":
			msg.Content.Edit = &EditContent{Body: "edited", MessageID: 42}
		case "delete":
			msg.Content.Delete = &DeleteContent{MessageIDs: []int{1, 2, 3}}
		case "reaction":
			msg.Content.Reaction = &ReactionContent{Body: ":+1:", MessageID: 10}
		case "attachment":
			msg.Content.Attachment = &AttachmentContent{
				Object:   AttachmentObject{Filename: "photo.jpg", Title: "Photo", MimeType: "image/jpeg"},
				Uploaded: true,
			}
		case "metadata":
			msg.Content.Metadata = &MetadataContent{ConversationTitle: "My Chat"}
		case "headline":
			msg.Content.Headline = &HeadlineContent{Headline: "Welcome!"}
		}

		// Serialize
		data, err := json.Marshal(msg)
		if err != nil {
			t.Logf("marshal error: %v", err)
			return false
		}

		// Deserialize
		var got MsgSummary
		if err := json.Unmarshal(data, &got); err != nil {
			t.Logf("unmarshal error: %v", err)
			return false
		}

		// Compare
		if !reflect.DeepEqual(msg, got) {
			t.Logf("round-trip mismatch for type %s", ct)
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}
