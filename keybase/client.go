package keybase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
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
	c.stdin.Close()
	return c.cmd.Wait()
}

// call sends a JSON command and decodes the response.
func (c *Client) call(req interface{}, resp interface{}) error {
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
// The known function is called for each message ID; if it returns true,
// pagination stops (the message already exists locally). Messages are
// returned newest-first as received from the API.
func (c *Client) ReadConversation(convID string, known func(int) bool) ([]MsgSummary, error) {
	var allMsgs []MsgSummary
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
	return allMsgs, nil
}

// DownloadAttachment downloads an attachment to outPath using a separate
// "keybase chat download" invocation.
func (c *Client) DownloadAttachment(convID string, msgID int, outPath string) error {
	cmd := exec.Command("keybase", "chat", "download",
		"--conversation-id", convID,
		"--message-id", fmt.Sprintf("%d", msgID),
		"--output", outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keybase chat download: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}
