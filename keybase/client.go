package keybase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
)

// Client manages a long-running "keybase chat api" subprocess.
// Each worker goroutine should create its own Client instance.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *json.Decoder
	stderr bytes.Buffer
}

// NewClient starts a "keybase chat api" subprocess with stdin/stdout/stderr pipes.
func NewClient() (*Client, error) {
	cmd := exec.Command("keybase", "chat", "api")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: json.NewDecoder(stdout),
	}
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start keybase chat api: %w", err)
	}
	return c, nil
}

// Close terminates the subprocess.
func (c *Client) Close() error {
	_ = c.stdin.Close()
	return c.cmd.Wait()
}

// call sends a JSON command and decodes the response.
func (c *Client) call(req any, resp any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write to keybase: %w", err)
	}
	if err := c.stdout.Decode(resp); err != nil {
		return fmt.Errorf("read from keybase: %w (stderr: %s)", err, c.stderr.String())
	}
	return nil
}

// ListConversations retrieves all conversations accessible to the authenticated user.
func (c *Client) ListConversations() ([]ConvSummary, error) {
	req := struct {
		Method string `json:"method"`
	}{Method: "list"}

	var resp ChatListResult
	if err := c.call(req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("keybase API error: %s", resp.Error.Message)
	}
	return resp.Result.Conversations, nil
}

// readParams holds parameters for the read API call.
type readParams struct {
	Options readOptions `json:"options"`
}

type readOptions struct {
	ConversationID string      `json:"conversation_id"`
	Pagination     *Pagination `json:"pagination,omitempty"`
}

// ReadConversation reads messages from a conversation, handling pagination.
// It first uses the paginated read API, then crawls prev chains via the
// get API to retrieve messages beyond the ~1000 pagination limit.
// The known function is called for each message ID; if it returns true,
// that message is skipped (already exists locally). Messages are returned
// newest-first as received from the API.
func (c *Client) ReadConversation(convID string, known func(int) bool) ([]MsgSummary, error) {
	var allMsgs []MsgSummary
	seen := make(map[int]bool)

	// Phase 1: paginated read (gets up to ~1000 newest messages).
	var next string
	for {
		opts := readOptions{ConversationID: convID}
		if next != "" {
			opts.Pagination = &Pagination{Next: next}
		}
		req := struct {
			Method string     `json:"method"`
			Params readParams `json:"params"`
		}{
			Method: "read",
			Params: readParams{Options: opts},
		}

		var resp ReadResult
		if err := c.call(req, &resp); err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("keybase API error: %s", resp.Error.Message)
		}

		hitKnown := false
		for _, m := range resp.Result.Messages {
			if m.Msg == nil {
				continue
			}
			seen[m.Msg.ID] = true
			if known != nil && known(m.Msg.ID) {
				hitKnown = true
				break
			}
			allMsgs = append(allMsgs, *m.Msg)
		}
		if hitKnown {
			break
		}

		if resp.Result.Pagination == nil || resp.Result.Pagination.Last {
			break
		}
		next = resp.Result.Pagination.Next
		if next == "" {
			break
		}
	}

	// Phase 2: crawl prev chains to fetch messages beyond the pagination limit.
	// Collect all prev IDs that we haven't seen yet and don't already exist locally.
	pending := make(map[int]bool)
	for _, msg := range allMsgs {
		for _, p := range msg.Prev {
			if !seen[p.ID] && (known == nil || !known(p.ID)) {
				pending[p.ID] = true
			}
		}
	}

	for len(pending) > 0 {
		// Collect up to 50 IDs per batch to avoid oversized requests.
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

		fetched, err := c.GetMessages(convID, batch)
		if err != nil {
			return nil, fmt.Errorf("get messages (chain crawl): %w", err)
		}

		for _, msg := range fetched {
			if seen[msg.ID] {
				continue
			}
			seen[msg.ID] = true
			if known != nil && known(msg.ID) {
				continue
			}
			allMsgs = append(allMsgs, msg)

			// Enqueue this message's prev pointers for crawling.
			for _, p := range msg.Prev {
				if !seen[p.ID] && (known == nil || !known(p.ID)) {
					pending[p.ID] = true
				}
			}
		}
	}

	return allMsgs, nil
}

// getParams holds parameters for the get API call.
type getParams struct {
	Options getOptions `json:"options"`
}

type getOptions struct {
	ConversationID string `json:"conversation_id"`
	MessageIDs     []int  `json:"message_ids"`
}

// GetMessages fetches specific messages by ID using the get API.
// This bypasses the ~1000 message pagination limit of the read API.
func (c *Client) GetMessages(convID string, msgIDs []int) ([]MsgSummary, error) {
	req := struct {
		Method string    `json:"method"`
		Params getParams `json:"params"`
	}{
		Method: "get",
		Params: getParams{Options: getOptions{
			ConversationID: convID,
			MessageIDs:     msgIDs,
		}},
	}

	var resp ReadResult
	if err := c.call(req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("keybase API error: %s", resp.Error.Message)
	}

	msgs := make([]MsgSummary, 0, len(resp.Result.Messages))
	for _, m := range resp.Result.Messages {
		if m.Msg != nil {
			msgs = append(msgs, *m.Msg)
		}
	}
	return msgs, nil
}

// DownloadAttachment downloads an attachment to outPath using a separate
// "keybase chat download" invocation. For team channels, --channel specifies
// the topic name.
func (c *Client) DownloadAttachment(channel ChatChannel, msgID int, outPath string) error {
	args := []string{"chat", "download", channel.Name, strconv.Itoa(msgID), "-o", outPath}
	if channel.MembersType == "team" && channel.TopicName != "" {
		args = append(args, "--channel", channel.TopicName)
	}
	cmd := exec.Command("keybase", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keybase chat download: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}
