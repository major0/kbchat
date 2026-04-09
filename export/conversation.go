package export

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/major0/kbchat/keybase"
)

// Result holds export counts for a single conversation.
type Result struct {
	ConvID                string
	MessagesExported      int
	AttachmentsDownloaded int
	Errors                []error
}

// ClientAPI abstracts the keybase.Client methods used by ExportConversation.
type ClientAPI interface {
	ReadConversation(convID string, known func(int) bool) ([]keybase.MsgSummary, error)
	DownloadAttachment(channel keybase.ChatChannel, msgID int, outPath string) error
	Close() error
}

// ExportConversation exports a single conversation using per-message directories.
func ExportConversation(
	client ClientAPI,
	conv keybase.ConvSummary,
	destDir string,
	selfUsername string,
	skipAttachments bool,
	verbose bool,
) Result {
	result := Result{ConvID: conv.ID}

	convDir := ConvDirPath(destDir, conv, selfUsername)
	attachDir := filepath.Join(convDir, "attachments")
	msgsDir := filepath.Join(convDir, "messages")
	if err := os.MkdirAll(attachDir, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("create dirs: %w", err))
		return result
	}
	if err := os.MkdirAll(msgsDir, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("create dirs: %w", err))
		return result
	}

	// Fetch messages, stopping when we hit a known message ID
	known := func(id int) bool { return MsgExists(convDir, id) }
	msgs, err := client.ReadConversation(conv.ID, known)
	if err != nil {
		if verbose {
			log.Printf("read conversation failed (conv=%s): %v", conv.ID, err)
		}
		result.Errors = append(result.Errors, fmt.Errorf("read conversation: %w", err))
		return result
	}

	if len(msgs) == 0 {
		return result
	}

	// Build set of IDs in this batch for orphan detection
	batchIDs := make(map[int]bool, len(msgs))
	for _, msg := range msgs {
		batchIDs[msg.ID] = true
	}

	// Write each message to its own directory
	var newestID int
	for _, msg := range msgs {
		if err := WriteMsg(convDir, msg); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write msg %d: %w", msg.ID, err))
			continue
		}
		result.MessagesExported++

		// Track newest message for head update
		if msg.ID > newestID {
			newestID = msg.ID
		}

		// Download attachments for this message
		if skipAttachments || msg.Content.Type != "attachment" || msg.Content.Attachment == nil {
			continue
		}
		filename := msg.Content.Attachment.Object.Filename
		if filename == "" {
			continue
		}
		ref, err := DownloadAttachment(client, conv.Channel, msg.ID, filename, attachDir)
		if err != nil {
			if verbose {
				log.Printf("attachment download failed (conv=%s msg=%d): %v", conv.ID, msg.ID, err)
			}
			result.Errors = append(result.Errors, err)
			continue
		}
		if err := WriteMsgAttachments(convDir, msg.ID, []AttachmentRef{*ref}); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write msg attachments %d: %w", msg.ID, err))
		}
		result.AttachmentsDownloaded++
	}

	// Detect orphaned prev pointers after all messages are written.
	// A prev pointer is orphaned only if it references a message that is
	// neither in this batch nor already on disk from a previous export.
	for _, msg := range msgs {
		var orphans []keybase.Prev
		for _, p := range msg.Prev {
			if !batchIDs[p.ID] && !MsgExists(convDir, p.ID) {
				orphans = append(orphans, p)
			}
		}
		if len(orphans) > 0 {
			if err := WriteOrphans(convDir, msg.ID, orphans); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("write orphans %d: %w", msg.ID, err))
			}
		}
	}

	// Update head to newest message
	oldHead, _ := ReadHead(convDir)
	if newestID > oldHead {
		if err := WriteHead(convDir, newestID); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write head: %w", err))
		}
	}

	return result
}
