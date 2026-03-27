package export

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/major0/keybase-export/keybase"
)

// Result holds export counts for a single conversation.
type Result struct {
	ConvID                string
	MessagesExported      int
	AttachmentsDownloaded int
	Errors                []error
}

// ClientAPI abstracts the keybase.Client methods used by ExportConversation,
// enabling test mocks.
type ClientAPI interface {
	ReadConversation(convID string, sinceTimestamp *int64) ([]keybase.MsgSummary, error)
	DownloadAttachment(convID string, msgID int, outPath string) error
}

// ExportConversation exports a single conversation: creates directories,
// fetches messages, writes messages.json, downloads attachments, updates timestamp.
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
	if err := os.MkdirAll(attachDir, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("create dirs: %w", err))
		return result
	}

	// Read existing timestamp for incremental export
	tsPath := filepath.Join(convDir, ".timestamp")
	ts, _ := ReadTimestamp(tsPath)
	var sinceTs *int64
	if ts > 0 {
		sinceTs = &ts
	}

	// Fetch messages
	msgs, err := client.ReadConversation(conv.ID, sinceTs)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("read conversation: %w", err))
		return result
	}

	if len(msgs) == 0 {
		return result
	}

	// Sort chronologically
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].SentAtMs < msgs[j].SentAtMs
	})

	// Write messages.json
	msgPath := filepath.Join(convDir, "messages.json")
	if err := WriteMessages(msgPath, msgs); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("write messages: %w", err))
		return result
	}
	result.MessagesExported = len(msgs)

	// Download attachments
	if !skipAttachments {
		var refs []AttachmentRef
		for _, msg := range msgs {
			if msg.Content.Type != "attachment" || msg.Content.Attachment == nil {
				continue
			}
			filename := msg.Content.Attachment.Object.Filename
			if filename == "" {
				continue
			}
			ref, err := DownloadAttachment(client, conv.ID, msg.ID, filename, attachDir)
			if err != nil {
				if verbose {
					log.Printf("attachment download failed (conv=%s msg=%d): %v", conv.ID, msg.ID, err)
				}
				result.Errors = append(result.Errors, err)
				continue
			}
			refs = append(refs, *ref)
			result.AttachmentsDownloaded++
		}
		if len(refs) > 0 {
			manifestPath := filepath.Join(convDir, "attachments.json")
			if err := WriteAttachmentManifest(manifestPath, refs); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("write manifest: %w", err))
			}
		}
	}

	// Update timestamp to the latest message
	latestTs := msgs[len(msgs)-1].SentAtMs
	if err := WriteTimestampAtomic(tsPath, latestTs); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("write timestamp: %w", err))
	}

	return result
}
