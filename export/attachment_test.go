package export

import (
	"math/rand"
	"testing"
	"testing/quick"
)

// Feature: keybase-go-export, Property 6: Attachment filename deduplication produces unique names
func TestPropertyFilenameDedup(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		n := r.Intn(20) + 1
		names := []string{"photo.jpg", "doc.pdf", "image.png", "file", "report.txt"}

		input := make([]string, n)
		for i := range input {
			input[i] = names[r.Intn(len(names))]
		}

		used := make(map[string]bool)
		output := make([]string, n)
		for i, name := range input {
			output[i] = DeduplicateFilename(name, used)
		}

		// Output length must equal input length
		if len(output) != len(input) {
			t.Logf("length mismatch: %d vs %d", len(output), len(input))
			return false
		}

		// All output names must be unique
		seen := make(map[string]bool)
		for _, name := range output {
			if seen[name] {
				t.Logf("duplicate output name: %s", name)
				return false
			}
			seen[name] = true
		}

		// First occurrence of each name retains original
		firstSeen := make(map[string]bool)
		for i, name := range input {
			if !firstSeen[name] {
				firstSeen[name] = true
				if output[i] != name {
					t.Logf("first occurrence changed: input %q, output %q", name, output[i])
					return false
				}
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestDeduplicateFilename_NoExtension(t *testing.T) {
	used := make(map[string]bool)
	got1 := DeduplicateFilename("README", used)
	got2 := DeduplicateFilename("README", used)
	if got1 != "README" {
		t.Errorf("first: got %q, want %q", got1, "README")
	}
	if got2 != "README_1" {
		t.Errorf("second: got %q, want %q", got2, "README_1")
	}
}

func TestDeduplicateFilename_MultipleCollisions(t *testing.T) {
	used := make(map[string]bool)
	results := make([]string, 5)
	for i := range results {
		results[i] = DeduplicateFilename("photo.jpg", used)
	}
	want := []string{"photo.jpg", "photo_1.jpg", "photo_2.jpg", "photo_3.jpg", "photo_4.jpg"}
	for i := range results {
		if results[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, results[i], want[i])
		}
	}
}
