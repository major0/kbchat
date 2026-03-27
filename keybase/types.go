package keybase

import "encoding/json"

// ConvSummary represents a conversation from the list API.
type ConvSummary struct {
	ID            string      `json:"id"`
	Channel       ChatChannel `json:"channel"`
	IsDefaultConv bool        `json:"is_default_conv"`
	Unread        bool        `json:"unread"`
	ActiveAt      int64       `json:"active_at"`
	ActiveAtMs    int64       `json:"active_at_ms"`
	MemberStatus  string      `json:"member_status"`
}

// ChatChannel identifies a conversation's participants or team/channel.
type ChatChannel struct {
	Name        string `json:"name"`
	Public      bool   `json:"public,omitempty"`
	MembersType string `json:"members_type,omitempty"`
	TopicType   string `json:"topic_type,omitempty"`
	TopicName   string `json:"topic_name,omitempty"`
}

// MsgSummary represents a single message from the read API.
type MsgSummary struct {
	ID                 int         `json:"id"`
	ConversationID     string      `json:"conversation_id"`
	Channel            ChatChannel `json:"channel"`
	Sender             MsgSender   `json:"sender"`
	SentAt             int64       `json:"sent_at"`
	SentAtMs           int64       `json:"sent_at_ms"`
	Content            MsgContent  `json:"content"`
	Prev               []Prev      `json:"prev"`
	Unread             bool        `json:"unread"`
	RevokedDevice      bool        `json:"revoked_device,omitempty"`
	KBFSEncrypted      bool        `json:"kbfs_encrypted,omitempty"`
	IsEphemeral        bool        `json:"is_ephemeral,omitempty"`
	HasPairwiseMacs    bool        `json:"has_pairwise_macs,omitempty"`
	AtMentionUsernames []string    `json:"at_mention_usernames,omitempty"`
	ChannelMention     string      `json:"channel_mention,omitempty"`
	Reactions          *Reactions  `json:"reactions,omitempty"`
	BotInfo            *BotInfo    `json:"bot_info,omitempty"`
}

// MsgSender identifies who sent a message.
type MsgSender struct {
	UID        string `json:"uid"`
	Username   string `json:"username,omitempty"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
}

// MsgContent holds the message type and type-specific payload.
type MsgContent struct {
	Type               string              `json:"type"`
	Text               *TextContent        `json:"text,omitempty"`
	Edit               *EditContent        `json:"edit,omitempty"`
	Delete             *DeleteContent      `json:"delete,omitempty"`
	Reaction           *ReactionContent    `json:"reaction,omitempty"`
	Attachment         *AttachmentContent  `json:"attachment,omitempty"`
	AttachmentUploaded *AttachmentUploaded `json:"attachment_uploaded,omitempty"`
	Metadata           *MetadataContent    `json:"metadata,omitempty"`
	Headline           *HeadlineContent    `json:"headline,omitempty"`
	System             json.RawMessage     `json:"system,omitempty"`
	SendPayment        json.RawMessage     `json:"send_payment,omitempty"`
	RequestPayment     json.RawMessage     `json:"request_payment,omitempty"`
	Unfurl             json.RawMessage     `json:"unfurl,omitempty"`
	Flip               json.RawMessage     `json:"flip,omitempty"`
}

// Prev references a previous message in the chain.
type Prev struct {
	ID   int    `json:"id"`
	Hash string `json:"hash"`
}

// Reactions maps emoji to a map of username→reaction.
type Reactions struct {
	Reactions map[string]map[string]Reaction `json:"reactions"`
}

// Reaction records when a reaction was created.
type Reaction struct {
	Ctime int64 `json:"ctime"`
}

// BotInfo identifies a bot associated with a message.
type BotInfo struct {
	BotUID      string `json:"bot_uid"`
	BotUsername string `json:"bot_username,omitempty"`
}

// TextContent holds a text message body and associated metadata.
type TextContent struct {
	Body         string          `json:"body"`
	Payments     json.RawMessage `json:"payments,omitempty"`
	ReplyTo      *int            `json:"replyTo,omitempty"`
	UserMentions json.RawMessage `json:"userMentions,omitempty"`
	TeamMentions json.RawMessage `json:"teamMentions,omitempty"`
	Emojis       json.RawMessage `json:"emojis,omitempty"`
}

// EditContent holds an edit message.
type EditContent struct {
	Body         string          `json:"body"`
	MessageID    int             `json:"messageID"`
	UserMentions json.RawMessage `json:"userMentions,omitempty"`
	TeamMentions json.RawMessage `json:"teamMentions,omitempty"`
	Emojis       json.RawMessage `json:"emojis,omitempty"`
}

// DeleteContent holds a delete message.
type DeleteContent struct {
	MessageIDs []int `json:"messageIDs"`
}

// ReactionContent holds a reaction message.
type ReactionContent struct {
	Body      string `json:"b"`
	MessageID int    `json:"m"`
}

// AttachmentContent holds an attachment message.
type AttachmentContent struct {
	Object   AttachmentObject `json:"object"`
	Preview  *AttachmentObject `json:"preview,omitempty"`
	Uploaded bool             `json:"uploaded"`
}

// AttachmentObject describes an attachment file.
type AttachmentObject struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	MimeType string `json:"mimeType"`
}

// AttachmentUploaded holds an attachment_uploaded message.
type AttachmentUploaded struct {
	MessageID int              `json:"messageID"`
	Object    AttachmentObject `json:"object"`
	Previews  []AttachmentObject `json:"previews,omitempty"`
}

// MetadataContent holds a conversation metadata message.
type MetadataContent struct {
	ConversationTitle string `json:"conversationTitle"`
}

// HeadlineContent holds a headline message.
type HeadlineContent struct {
	Headline string `json:"headline"`
}

// Pagination holds pagination state for API responses.
type Pagination struct {
	Next     string `json:"next,omitempty"`
	Previous string `json:"previous,omitempty"`
	Num      int    `json:"num"`
	Last     bool   `json:"last,omitempty"`
}

// API response wrappers

// ChatListResult wraps the list API response.
type ChatListResult struct {
	Result ChatList `json:"result"`
	Error  *APIError `json:"error,omitempty"`
}

// ChatList holds the conversation list.
type ChatList struct {
	Conversations []ConvSummary `json:"conversations"`
	Offline       bool          `json:"offline"`
}

// ReadResult wraps the read API response.
type ReadResult struct {
	Result Thread   `json:"result"`
	Error  *APIError `json:"error,omitempty"`
}

// Thread holds messages and pagination for a read response.
type Thread struct {
	Messages   []Message   `json:"messages"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Message wraps a message or an error.
type Message struct {
	Msg   *MsgSummary `json:"msg,omitempty"`
	Error *string     `json:"error,omitempty"`
}

// APIError represents an error from the Keybase API.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ConvType represents the classification of a conversation.
type ConvType int

const (
	ConvDM    ConvType = iota
	ConvGroup
	ConvTeam
)

// ClassifyConversation classifies a conversation by its MembersType and participant count.
// The participant count is derived from the comma-separated Name field.
func ClassifyConversation(ch ChatChannel) ConvType {
	switch ch.MembersType {
	case "team":
		return ConvTeam
	case "impteamnative", "impteamupgrade":
		// Count participants by splitting the name on commas.
		count := 1
		for _, c := range ch.Name {
			if c == ',' {
				count++
			}
		}
		if count <= 2 {
			return ConvDM
		}
		return ConvGroup
	default:
		return ConvDM
	}
}
