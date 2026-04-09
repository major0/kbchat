package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOpenCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	lf, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = lf.Close() }()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestOpenExistingAppendMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	// Write initial content directly.
	if err := os.WriteFile(path, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	lf, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = lf.Close() }()

	// Write through LogFile — should append, not truncate.
	if _, err := lf.Write([]byte("new line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = lf.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "existing\n") {
		t.Fatalf("existing content truncated: %q", content)
	}
	if !strings.Contains(content, "new line\n") {
		t.Fatalf("new content missing: %q", content)
	}
}

func TestWritePrependsTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	lf, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	before := time.Now().UTC()
	if _, err := lf.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after := time.Now().UTC()
	_ = lf.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	line := string(data)

	// Expect format: "<RFC3339> hello\n"
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		t.Fatalf("expected timestamp prefix, got: %q", line)
	}

	ts, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		t.Fatalf("timestamp not RFC3339: %v (line: %q)", err, line)
	}

	if ts.Before(before.Truncate(time.Second)) || ts.After(after.Add(time.Second)) {
		t.Fatalf("timestamp %v outside [%v, %v]", ts, before, after)
	}

	if parts[1] != "hello\n" {
		t.Fatalf("unexpected payload: %q", parts[1])
	}
}

func TestWriteThreadSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	lf, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = lf.Close() }()

	const goroutines = 10
	const writes = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for range writes {
				msg := strings.Repeat("x", 50) + "\n"
				if _, err := lf.Write([]byte(msg)); err != nil {
					t.Errorf("goroutine %d write failed: %v", id, err)
				}
			}
		}(i)
	}
	wg.Wait()
	_ = lf.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	expected := goroutines * writes
	if len(lines) != expected {
		t.Fatalf("expected %d lines, got %d", expected, len(lines))
	}

	// Each line should start with a valid RFC3339 timestamp.
	for i, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			t.Fatalf("line %d: missing timestamp: %q", i, line)
		}
		if _, err := time.Parse(time.RFC3339, parts[0]); err != nil {
			t.Fatalf("line %d: bad timestamp: %v", i, err)
		}
	}
}

func TestReopenWritesToNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	lf, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = lf.Close() }()

	if _, err := lf.Write([]byte("before reopen\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Simulate logrotate: rename the file, then reopen.
	rotated := filepath.Join(dir, "test.log.1")
	if err := os.Rename(path, rotated); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if err := lf.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if _, err := lf.Write([]byte("after reopen\n")); err != nil {
		t.Fatalf("Write after reopen: %v", err)
	}
	_ = lf.Close()

	// The rotated file should have the first write.
	rotatedData, err := os.ReadFile(rotated)
	if err != nil {
		t.Fatalf("ReadFile rotated: %v", err)
	}
	if !strings.Contains(string(rotatedData), "before reopen") {
		t.Fatalf("rotated file missing first write: %q", rotatedData)
	}

	// The new file at the original path should have only the second write.
	newData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile new: %v", err)
	}
	if !strings.Contains(string(newData), "after reopen") {
		t.Fatalf("new file missing second write: %q", newData)
	}
	if strings.Contains(string(newData), "before reopen") {
		t.Fatalf("new file should not contain first write: %q", newData)
	}
}

func TestClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	lf, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := lf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Writing after close should fail.
	if _, err := lf.Write([]byte("should fail\n")); err == nil {
		t.Fatal("expected error writing to closed LogFile")
	}
}

func TestAppendModeVerification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	// First session: open, write, close.
	lf1, err := Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	if _, err := lf1.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	_ = lf1.Close()

	// Second session: reopen, write, close.
	lf2, err := Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	if _, err := lf2.Write([]byte("second\n")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	_ = lf2.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "first\n") {
		t.Fatalf("first write missing: %q", content)
	}
	if !strings.Contains(content, "second\n") {
		t.Fatalf("second write missing: %q", content)
	}

	// Verify ordering: first write appears before second.
	firstIdx := strings.Index(content, "first\n")
	secondIdx := strings.Index(content, "second\n")
	if firstIdx >= secondIdx {
		t.Fatalf("first write should precede second: first@%d second@%d in %q", firstIdx, secondIdx, content)
	}
}
