package export

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/major0/kbchat/keybase"
)

// Result holds export counts for a single conversation.
type Result struct {
	ConvID                string
	MessagesExported      int
	AttachmentsDownloaded int
	Errors                []error
}

// ClientAPI abstracts the keybase.Client methods used by Conversation.
type ClientAPI interface {
	ReadConversation(convID string, known func(int) bool) ([]keybase.MsgSummary, error)
	GetMessages(convID string, msgIDs []int) ([]keybase.MsgSummary, error)
	DownloadAttachment(channel keybase.ChatChannel, msgID int, outPath string) error
	Close() error
}

// Conversation exports a single conversation using per-message directories.
func Conversation(
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
	if err := os.MkdirAll(attachDir, 0o750); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("create dirs: %w", err))
		return result
	}
	if err := os.MkdirAll(msgsDir, 0o750); err != nil {
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
		// No new messages, but still run backfill for gaps from previous exports.
		backfilled := backfillOrphans(client, conv, convDir, attachDir, skipAttachments, verbose)
		result.MessagesExported += backfilled.MessagesExported
		result.AttachmentsDownloaded += backfilled.AttachmentsDownloaded
		result.Errors = append(result.Errors, backfilled.Errors...)
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

	// Backfill gaps: scan existing messages for orphaned prev pointers
	// and fetch any that are still missing. This resolves gaps left by
	// previous exports that hit the ~1000 message pagination limit.
	backfilled := backfillOrphans(client, conv, convDir, attachDir, skipAttachments, verbose)
	result.MessagesExported += backfilled.MessagesExported
	result.AttachmentsDownloaded += backfilled.AttachmentsDownloaded
	result.Errors = append(result.Errors, backfilled.Errors...)

	return result
}

// backfillOrphans scans all existing messages for prev pointers to
// missing messages, fetches them via GetMessages in batches, and
// repeats until the full chain is resolved. Tracks state in memory
// to avoid re-scanning disk on each iteration.
func backfillOrphans(
	client ClientAPI,
	conv keybase.ConvSummary,
	convDir string,
	attachDir string,
	skipAttachments bool,
	verbose bool,
) Result {
	var result Result

	// Build the set of existing message IDs from disk (one scan).
	msgsDir := filepath.Join(convDir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return result
	}
	existing := make(map[int]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		existing[id] = true
	}

	// Collect all missing prev IDs from existing messages (one scan).
	pending := make(map[int]bool)
	for id := range existing {
		msg, err := ReadMsg(convDir, id)
		if err != nil || msg == nil {
			continue
		}
		for _, p := range msg.Prev {
			if !existing[p.ID] {
				pending[p.ID] = true
			}
		}
	}

	if len(pending) == 0 {
		return result
	}

	if verbose {
		log.Printf("backfilling %d missing messages (conv=%s)", len(pending), conv.ID)
	}

	// Fetch missing messages in batches, enqueuing new prev pointers
	// as they're discovered. No disk re-scan needed.
	for len(pending) > 0 {
		batch := make([]int, 0, min(len(pending), 50))
		for id := range pending {
			batch = append(batch, id)
			if len(batch) >= 50 {
				break
			}
		}
		for _, id := range batch {
			delete(pending, id)
		}

		msgs, err := client.GetMessages(conv.ID, batch)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("backfill get messages: %w", err))
			return result
		}

		for _, msg := range msgs {
			if existing[msg.ID] {
				continue
			}
			existing[msg.ID] = true

			if err := WriteMsg(convDir, msg); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("backfill write msg %d: %w", msg.ID, err))
				continue
			}
			result.MessagesExported++

			// Enqueue this message's prev pointers.
			for _, p := range msg.Prev {
				if !existing[p.ID] {
					pending[p.ID] = true
				}
			}

			// Download attachments for backfilled messages.
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
					log.Printf("backfill attachment failed (conv=%s msg=%d): %v", conv.ID, msg.ID, err)
				}
				result.Errors = append(result.Errors, err)
				continue
			}
			if err := WriteMsgAttachments(convDir, msg.ID, []AttachmentRef{*ref}); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("backfill write attachments %d: %w", msg.ID, err))
			}
			result.AttachmentsDownloaded++
		}

		// If the batch returned nothing useful (all IDs already existed
		// or the API returned empty), check if we're making progress.
		// Remaining pending IDs that the API can't resolve are
		// permanently missing (deleted, ephemeral, etc.).
		allPendingUnresolvable := true
		for id := range pending {
			if !existing[id] {
				allPendingUnresolvable = false
				break
			}
		}
		if allPendingUnresolvable && len(pending) > 0 {
			break
		}
	}

	// Clean up stale orphans.json files now that gaps are filled.
	refreshOrphans(convDir)

	return result
}

// refreshOrphans re-evaluates orphans.json for all messages: removes
// resolved orphans (now on disk) and writes new orphans for backfilled
// messages. Deletes orphans.json when all orphans are resolved.
func refreshOrphans(convDir string) {
	msgsDir := filepath.Join(convDir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		msgID, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		// Check existing orphans — remove resolved ones.
		orphans, _ := ReadOrphans(convDir, msgID)
		if len(orphans) > 0 {
			var remaining []keybase.Prev
			for _, o := range orphans {
				if !MsgExists(convDir, o.ID) {
					remaining = append(remaining, o)
				}
			}
			orphansPath := filepath.Join(msgDir(convDir, msgID), "orphans.json")
			if len(remaining) == 0 {
				_ = os.Remove(orphansPath)
			} else if len(remaining) < len(orphans) {
				_ = WriteOrphans(convDir, msgID, remaining)
			}
			continue
		}

		// For messages without orphans.json, check if they have prev
		// pointers to missing messages (newly backfilled messages may
		// introduce new orphans).
		msg, err := ReadMsg(convDir, msgID)
		if err != nil || msg == nil {
			continue
		}
		var newOrphans []keybase.Prev
		for _, p := range msg.Prev {
			if !MsgExists(convDir, p.ID) {
				newOrphans = append(newOrphans, p)
			}
		}
		if len(newOrphans) > 0 {
			_ = WriteOrphans(convDir, msgID, newOrphans)
		}
	}
}
