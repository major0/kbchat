// Package logfile provides a log file writer with SIGHUP-based reopen
// support for logrotate compatibility.
package logfile

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// LogFile manages a log file opened in append mode. It implements io.Writer
// so it can be passed to log.SetOutput. The Reopen method supports logrotate
// by closing and reopening the same path on SIGHUP.
type LogFile struct {
	path string
	file *os.File
	mu   sync.Mutex
}

// Open opens the file at path in append mode (create if missing) and returns
// a LogFile. The caller should defer Close.
func Open(path string) (*LogFile, error) {
	f, err := openAppend(path)
	if err != nil {
		return nil, fmt.Errorf("opening log file %q: %w", path, err)
	}
	return &LogFile{path: path, file: f}, nil
}

// Write prepends an ISO 8601 timestamp to each write, then writes to the
// underlying file. It satisfies io.Writer.
func (l *LogFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339) + " "
	if _, err := l.file.WriteString(ts); err != nil {
		return 0, err
	}
	return l.file.Write(p)
}

// Reopen closes the current file and reopens the same path. This is the
// standard logrotate contract: the log manager renames the file, sends
// SIGHUP, and the process opens a new file at the original path.
func (l *LogFile) Reopen() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("closing log file for reopen: %w", err)
	}
	f, err := openAppend(l.path)
	if err != nil {
		return fmt.Errorf("reopening log file %q: %w", l.path, err)
	}
	l.file = f
	return nil
}

// Close closes the underlying file.
func (l *LogFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// openAppend opens a file in append mode, creating it if it doesn't exist.
func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
}
