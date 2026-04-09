package cmd

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/major0/kbchat/keybase"
)

// --- Property-Based Tests ---

// Feature: keybase-chat-view, Property 5: FormatMsg produces correct format
// for all message types.
//
// For any valid MsgSummary, output matches IRC-log format: text messages use
// [ts] <user> body, non-text use [ts] * user type: summary. Summary is never
// empty.
//
// **Validates: Requirements 2.1, 2.2**

// formatMsgInput holds a randomly generated MsgSummary for property testing.
type formatMsgInput struct {
	Msg     keybase.MsgSummary
	TimeFmt string
}

// Generate implements quick.Generator for formatMsgInput.
func (formatMsgInput) Generate(r *rand.Rand, size int) reflect.Value {
	usernames := []string{"alice", "bob", "charlie", "dave"}
	devices := []string{"phone", "laptop", "desktop"}
	timeFmts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"15:04",
	}

	msg := keybase.MsgSummary{
		ID:     r.Intn(10000) + 1,
		SentAt: int64(r.Intn(2000000000)),
		Sender: keybase.MsgSender{
			Username:   usernames[r.Intn(len(usernames))],
			DeviceName: devices[r.Intn(len(devices))],
		},
	}

	// Pick a random message type and populate the corresponding content.
	types := []string{"text", "edit", "delete", "reaction", "attachment", "headline", "metadata", "system", "send_payment", "request_payment", "unfurl", "flip"}
	msgType := types[r.Intn(len(types))]
	msg.Content.Type = msgType

	switch msgType {
	case "text":
		msg.Content.Text = &keybase.TextContent{Body: randomString(r, 1+r.Intn(20))}
	case "edit":
		msg.Content.Edit = &keybase.EditContent{Body: randomString(r, 1+r.Intn(20))}
	case "delete":
		n := 1 + r.Intn(5)
		ids := make([]int, n)
		for i := range ids {
			ids[i] = r.Intn(1000) + 1
		}
		msg.Content.Delete = &keybase.DeleteContent{MessageIDs: ids}
	case "reaction":
		emojis := []string{"👍", "❤️", "😂", "🎉", "🔥"}
		msg.Content.Reaction = &keybase.ReactionContent{Body: emojis[r.Intn(len(emojis))]}
	case "attachment":
		msg.Content.Attachment = &keybase.AttachmentContent{
			Object: keybase.AttachmentObject{Filename: randomString(r, 3+r.Intn(10)) + ".png"},
		}
	case "headline":
		msg.Content.Headline = &keybase.HeadlineContent{Headline: randomString(r, 5+r.Intn(20))}
	case "metadata":
		msg.Content.Metadata = &keybase.MetadataContent{ConversationTitle: randomString(r, 3+r.Intn(15))}
	}

	return reflect.ValueOf(formatMsgInput{
		Msg:     msg,
		TimeFmt: timeFmts[r.Intn(len(timeFmts))],
	})
}

func TestPropertyFormatMsgIRCFormat(t *testing.T) {
	f := func(input formatMsgInput) bool {
		result := FormatMsg(input.Msg, input.TimeFmt, false)
		ts := time.Unix(input.Msg.SentAt, 0).Format(input.TimeFmt)
		user := input.Msg.Sender.Username

		if input.Msg.Content.Type == "text" {
			// Text format: [ts] <user> body
			prefix := fmt.Sprintf("[%s] <%s> ", ts, user)
			if !strings.HasPrefix(result, prefix) {
				t.Logf("text msg: result=%q missing prefix=%q", result, prefix)
				return false
			}
		} else {
			// Non-text format: [ts] * user type: summary
			prefix := fmt.Sprintf("[%s] * %s %s: ", ts, user, input.Msg.Content.Type)
			if !strings.HasPrefix(result, prefix) {
				t.Logf("non-text msg: result=%q missing prefix=%q", result, prefix)
				return false
			}
			// Extract summary (everything after the prefix).
			summary := result[len(prefix):]
			if summary == "" {
				t.Logf("non-text msg: empty summary for type=%q", input.Msg.Content.Type)
				return false
			}
		}

		// No trailing newline.
		if strings.HasSuffix(result, "\n") {
			t.Logf("result has trailing newline: %q", result)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// Feature: keybase-chat-view, Property 6: Verbose mode adds ID prefix and
// device suffix.
//
// For any message with verbose=true, output contains [id=N] prefix and
// (device_name) suffix.
//
// **Validates: Requirements 2.4**

func TestPropertyFormatMsgVerbose(t *testing.T) {
	f := func(input formatMsgInput) bool {
		result := FormatMsg(input.Msg, input.TimeFmt, true)

		// Must start with [id=<msgID>]
		idPrefix := fmt.Sprintf("[id=%d] ", input.Msg.ID)
		if !strings.HasPrefix(result, idPrefix) {
			t.Logf("verbose: result=%q missing id prefix=%q", result, idPrefix)
			return false
		}

		// Must end with (<device_name>)
		devSuffix := fmt.Sprintf("(%s)", input.Msg.Sender.DeviceName)
		if !strings.HasSuffix(result, devSuffix) {
			t.Logf("verbose: result=%q missing device suffix=%q", result, devSuffix)
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// randomString generates a random alphanumeric string of the given length.
func randomString(r *rand.Rand, n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[r.Intn(len(chars))]
	}
	return string(b)
}

// --- Table-Driven Tests ---

func TestFormatMsg(t *testing.T) {
	const timeFmt = "2006-01-02 15:04:05"
	const sentAt int64 = 1700000000
	ts := time.Unix(sentAt, 0).Format(timeFmt)

	base := keybase.MsgSummary{
		ID:     42,
		SentAt: sentAt,
		Sender: keybase.MsgSender{
			Username:   "alice",
			DeviceName: "laptop",
		},
	}

	tests := []struct {
		name    string
		msg     keybase.MsgSummary
		verbose bool
		want    string
	}{
		{
			name: "text message",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type: "text",
					Text: &keybase.TextContent{Body: "hello world"},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] <alice> hello world", ts),
		},
		{
			name: "edit message",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type: "edit",
					Edit: &keybase.EditContent{Body: "corrected text"},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice edit: corrected text", ts),
		},
		{
			name: "delete single ID",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:   "delete",
					Delete: &keybase.DeleteContent{MessageIDs: []int{7}},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice delete: deleted message 7", ts),
		},
		{
			name: "delete multiple IDs",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:   "delete",
					Delete: &keybase.DeleteContent{MessageIDs: []int{3, 5, 9}},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice delete: deleted messages 3, 5, 9", ts),
		},
		{
			name: "reaction message",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:     "reaction",
					Reaction: &keybase.ReactionContent{Body: "👍"},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice reaction: 👍", ts),
		},
		{
			name: "attachment message",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type: "attachment",
					Attachment: &keybase.AttachmentContent{
						Object: keybase.AttachmentObject{Filename: "photo.jpg"},
					},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice attachment: photo.jpg", ts),
		},
		{
			name: "headline message",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:     "headline",
					Headline: &keybase.HeadlineContent{Headline: "Welcome!"},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice headline: Welcome!", ts),
		},
		{
			name: "metadata message",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:     "metadata",
					Metadata: &keybase.MetadataContent{ConversationTitle: "Project Chat"},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice metadata: Project Chat", ts),
		},
		{
			name: "system message (no summary)",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "system"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice system: (no summary)", ts),
		},
		{
			name: "send_payment (no summary)",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "send_payment"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice send_payment: (no summary)", ts),
		},
		{
			name: "unfurl (no summary)",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "unfurl"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice unfurl: (no summary)", ts),
		},
		{
			name: "flip (no summary)",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "flip"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice flip: (no summary)", ts),
		},
		{
			name:    "verbose text message",
			verbose: true,
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type: "text",
					Text: &keybase.TextContent{Body: "hi"},
				}
				return m
			}(),
			want: fmt.Sprintf("[id=42] [%s] <alice> hi (laptop)", ts),
		},
		{
			name:    "verbose non-text message",
			verbose: true,
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:     "reaction",
					Reaction: &keybase.ReactionContent{Body: "🔥"},
				}
				return m
			}(),
			want: fmt.Sprintf("[id=42] [%s] * alice reaction: 🔥 (laptop)", ts),
		},
		{
			name: "non-verbose text message has no id or device",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type: "text",
					Text: &keybase.TextContent{Body: "test"},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] <alice> test", ts),
		},
		{
			name: "nil text content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "text"}
				return m
			}(),
			want: fmt.Sprintf("[%s] <alice> ", ts),
		},
		{
			name: "nil edit content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "edit"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice edit: (no summary)", ts),
		},
		{
			name: "nil delete content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "delete"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice delete: (no summary)", ts),
		},
		{
			name: "nil reaction content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "reaction"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice reaction: (no summary)", ts),
		},
		{
			name: "nil attachment content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "attachment"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice attachment: (no summary)", ts),
		},
		{
			name: "nil headline content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "headline"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice headline: (no summary)", ts),
		},
		{
			name: "nil metadata content pointer",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{Type: "metadata"}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice metadata: (no summary)", ts),
		},
		{
			name: "delete with empty IDs slice",
			msg: func() keybase.MsgSummary {
				m := base
				m.Content = keybase.MsgContent{
					Type:   "delete",
					Delete: &keybase.DeleteContent{MessageIDs: []int{}},
				}
				return m
			}(),
			want: fmt.Sprintf("[%s] * alice delete: (no summary)", ts),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMsg(tt.msg, timeFmt, tt.verbose)
			if got != tt.want {
				t.Errorf("FormatMsg() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}
