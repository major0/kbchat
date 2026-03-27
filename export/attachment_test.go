package export

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"
)

// Feature: keybase-go-export, Property 6: Content-addressable attachment storage
func TestPropertyContentAddressableStorage(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		dir := t.TempDir()

		// Generate 1-10 attachments with possible content overlap
		n := r.Intn(10) + 1
		contents := [][]byte{
			[]byte("content-a"),
			[]byte("content-b"),
			[]byte("content-c"),
		}
		filenames := []string{"photo.jpg", "doc.pdf", "image.png", "file", "report.txt"}

		type input struct {
			filename string
			content  []byte
		}
		inputs := make([]input, n)
		for i := range inputs {
			inputs[i] = input{
				filename: filenames[r.Intn(len(filenames))],
				content:  contents[r.Intn(len(contents))],
			}
		}

		// Store each attachment
		var refs []AttachmentRef
		for _, in := range inputs {
			// Write content to a temp file, hash it, store it
			tmpPath := filepath.Join(dir, "tmp")
			if err := os.WriteFile(tmpPath, in.content, 0644); err != nil {
				t.Logf("write temp: %v", err)
				return false
			}

			hash, err := HashFile(tmpPath)
			if err != nil {
				t.Logf("hash error: %v", err)
				return false
			}
			ref := StorageRef(hash, in.filename)
			destPath := filepath.Join(dir, ref)

			if _, err := os.Stat(destPath); err != nil {
				if err := os.Rename(tmpPath, destPath); err != nil {
					t.Logf("rename: %v", err)
					return false
				}
			} else {
				os.Remove(tmpPath)
			}

			refs = append(refs, AttachmentRef{
				Filename:   in.filename,
				StorageRef: ref,
			})
		}

		// Verify: one manifest entry per input
		if len(refs) != n {
			t.Logf("manifest length %d != input length %d", len(refs), n)
			return false
		}

		// Verify: all storage refs point to files that exist
		for _, ref := range refs {
			path := filepath.Join(dir, ref.StorageRef)
			if _, err := os.Stat(path); err != nil {
				t.Logf("storage ref %q does not exist", ref.StorageRef)
				return false
			}
		}

		// Verify: identical content produces same storage ref
		hashMap := make(map[string]string) // content hash → storage ref
		for i, in := range inputs {
			h := sha256.Sum256(in.content)
			hash := hex.EncodeToString(h[:])
			expected := StorageRef(hash, in.filename)
			if refs[i].StorageRef != expected {
				t.Logf("ref mismatch: got %q, want %q", refs[i].StorageRef, expected)
				return false
			}
			if prev, ok := hashMap[hash]; ok {
				// Same content hash should produce same storage ref prefix
				// (ext may differ if filenames differ)
				_ = prev
			}
			hashMap[hash] = refs[i].StorageRef
		}

		// Verify: number of files on disk <= number of unique content hashes
		files, _ := filepath.Glob(filepath.Join(dir, "*.*"))
		uniqueContents := make(map[string]bool)
		for _, in := range inputs {
			h := sha256.Sum256(in.content)
			// Account for different extensions producing different storage refs
			ref := StorageRef(hex.EncodeToString(h[:]), in.filename)
			uniqueContents[ref] = true
		}
		if len(files) > len(uniqueContents) {
			t.Logf("files on disk (%d) > unique storage refs (%d)", len(files), len(uniqueContents))
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestStorageRef_NoExtension(t *testing.T) {
	ref := StorageRef("abc123", "README")
	if ref != "abc123.bin" {
		t.Errorf("got %q, want %q", ref, "abc123.bin")
	}
}

func TestStorageRef_WithExtension(t *testing.T) {
	ref := StorageRef("abc123", "photo.jpg")
	if ref != "abc123.jpg" {
		t.Errorf("got %q, want %q", ref, "abc123.jpg")
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSameContentDifferentFilename(t *testing.T) {
	dir := t.TempDir()
	content := []byte("identical content")
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	// Store as photo.jpg
	ref1 := StorageRef(hash, "photo.jpg")
	if err := os.WriteFile(filepath.Join(dir, ref1), content, 0644); err != nil {
		t.Fatal(err)
	}

	// Store as image.jpg — same hash, same ext → same file
	ref2 := StorageRef(hash, "image.jpg")
	if ref1 != ref2 {
		t.Errorf("same content same ext should produce same ref: %q vs %q", ref1, ref2)
	}

	// Store as photo.png — same hash, different ext → different file
	ref3 := StorageRef(hash, "photo.png")
	if ref1 == ref3 {
		t.Errorf("same content different ext should produce different ref: %q vs %q", ref1, ref3)
	}
}

func TestSameFilenameDifferentContent(t *testing.T) {
	content1 := []byte("content version 1")
	content2 := []byte("content version 2")

	h1 := sha256.Sum256(content1)
	h2 := sha256.Sum256(content2)

	ref1 := StorageRef(hex.EncodeToString(h1[:]), "photo.jpg")
	ref2 := StorageRef(hex.EncodeToString(h2[:]), "photo.jpg")

	if ref1 == ref2 {
		t.Error("different content should produce different storage refs")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "attachments.json")

	refs := []AttachmentRef{
		{Filename: "photo.jpg", StorageRef: "abc123.jpg"},
		{Filename: "doc.pdf", StorageRef: "def456.pdf"},
	}

	if err := WriteAttachmentManifest(path, refs); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAttachmentManifest(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(refs) {
		t.Fatalf("length mismatch: %d vs %d", len(got), len(refs))
	}
	for i := range refs {
		if got[i] != refs[i] {
			t.Errorf("index %d: got %+v, want %+v", i, got[i], refs[i])
		}
	}
}
