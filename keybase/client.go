package keybase

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Client manages a long-running keybase chat api subprocess.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *json.Decoder
	stderr io.ReadCloser
	mu     sync.Mutex // serializes API calls
}

// NewClient starts a keybase chat api subprocess with stdin/stdout/stderr pipes.
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
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start keybase chat api: %w", err)
	}
	return &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: json.NewDecoder(stdout),
		stderr: stderr,
	}, nil
}

// Close terminates the subprocess.
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

// call sends a JSON command and decodes the response.
func (c *Client) call(req any, resp any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write to keybase: %w", err)
	}
	if err := c.stdout.Decode(resp); err != nil {
		return fmt.Errorf("read from keybase: %w", err)
	}
	return nil
}
