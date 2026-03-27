package export

import (
	"os"
	"path/filepath"
	"testing"
	"testing/quick"
)

// Feature: keybase-go-export, Property 7: Timestamp serialization round-trip
func TestPropertyTimestampRoundTrip(t *testing.T) {
	dir := t.TempDir()

	f := func(ts int64) bool {
		if ts < 0 {
			ts = -ts
		}
		path := filepath.Join(dir, ".timestamp")
		if err := WriteTimestampAtomic(path, ts); err != nil {
			t.Logf("write error: %v", err)
			return false
		}
		got, err := ReadTimestamp(path)
		if err != nil {
			t.Logf("read error: %v", err)
			return false
		}
		if got != ts {
			t.Logf("round-trip mismatch: wrote %d, read %d", ts, got)
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestReadTimestamp_MissingFile(t *testing.T) {
	ts, err := ReadTimestamp(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != 0 {
		t.Fatalf("expected 0 for missing file, got %d", ts)
	}
}

func TestReadTimestamp_GarbageContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".timestamp")
	os.WriteFile(path, []byte("not-a-number\n"), 0644)
	ts, err := ReadTimestamp(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != 0 {
		t.Fatalf("expected 0 for garbage content, got %d", ts)
	}
}

func TestReadTimestamp_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".timestamp")
	os.WriteFile(path, []byte(""), 0644)
	ts, err := ReadTimestamp(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != 0 {
		t.Fatalf("expected 0 for empty file, got %d", ts)
	}
}
