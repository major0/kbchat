package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/major0/kbchat/keybase"
	"github.com/major0/kbchat/store"
)

// makeTestStore creates a temp store with Chat conversations.
// Each key is a chat name (e.g. "alice,bob"), value is the messages.
func makeTestStore(t *testing.T, convs map[string][]keybase.MsgSummary) string {
	t.Helper()
	storeDir := t.TempDir()
	for name, msgs := range convs {
		writeConvMessages(t, filepath.Join(storeDir, "Chats", name), msgs)
	}
	return storeDir
}

// makeConvInfoStore creates a temp store from ConvInfo descriptors.
// Messages are minimal stubs with sequential IDs and timestamps.
func makeConvInfoStore(t *testing.T, convs []store.ConvInfo) string {
	t.Helper()
	storeDir := t.TempDir()
	for _, conv := range convs {
		var convDir string
		switch conv.Type {
		case "Team":
			convDir = filepath.Join(storeDir, "Teams", conv.Name, conv.Channel)
		default:
			convDir = filepath.Join(storeDir, "Chats", conv.Name)
		}
		msgs := make([]keybase.MsgSummary, conv.MsgCount)
		for i := range msgs {
			msgs[i] = keybase.MsgSummary{
				ID:     i + 1,
				SentAt: int64(1000000 + (i+1)*1000),
			}
		}
		writeConvMessages(t, convDir, msgs)
	}
	return storeDir
}

// writeConvMessages writes message.json files into convDir/messages/<id>/.
func writeConvMessages(t *testing.T, convDir string, msgs []keybase.MsgSummary) {
	t.Helper()
	msgsDir := filepath.Join(convDir, "messages")
	if err := os.MkdirAll(msgsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, msg := range msgs {
		msgDir := filepath.Join(msgsDir, strconv.Itoa(msg.ID))
		if err := os.MkdirAll(msgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(msgDir, "message.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// textMsg creates a text MsgSummary with the given parameters.
func textMsg(id int, sentAt int64, user, body string) keybase.MsgSummary {
	return keybase.MsgSummary{
		ID:     id,
		SentAt: sentAt,
		Sender: keybase.MsgSender{Username: user, DeviceName: "phone"},
		Content: keybase.MsgContent{
			Type: "text",
			Text: &keybase.TextContent{Body: body},
		},
	}
}

// textMsgs generates n text messages with sequential IDs and timestamps
// starting at baseTime, spaced 60 seconds apart.
func textMsgs(n int, baseTime int64) []keybase.MsgSummary {
	msgs := make([]keybase.MsgSummary, n)
	for i := range msgs {
		msgs[i] = textMsg(i+1, baseTime+int64(i*60), "alice", "message "+strconv.Itoa(i+1))
		msgs[i].Sender.DeviceName = "laptop"
	}
	return msgs
}

// makeTestStoreOneConv creates a temp store with a single "alice,bob" Chat conversation.
func makeTestStoreOneConv(t *testing.T, msgs []keybase.MsgSummary) string {
	t.Helper()
	return makeTestStore(t, map[string][]keybase.MsgSummary{
		"alice,bob": msgs,
	})
}
