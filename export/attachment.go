package export

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AttachmentRef maps an attachment to its content-addressable file on disk.
type AttachmentRef struct {
	Filename   string `json:"filename"`    // original filename from API
	StorageRef string `json:"storage_ref"` // <sha256>.<ext> on disk
}

// StorageRef computes the content-addressable filename: <sha256>.<ext>.
func StorageRef(hash string, originalFilename string) string {
	ext := filepath.Ext(originalFilename)
	if ext == "" {
		ext = ".bin"
	}
	return hash + ext
}

// HashFile computes the SHA-256 hex digest of a file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DownloadAttachment downloads an attachment using content-addressable storage.
// Flow: download to temp → hash → store as <sha256>.<ext> → skip if exists.
func DownloadAttachment(client ClientAPI, convID string, msgID int, filename string, attachDir string) (*AttachmentRef, error) {
	tmp, err := os.CreateTemp(attachDir, ".download-*")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	tmp.Close()

	if err := client.DownloadAttachment(convID, msgID, tmpName); err != nil {
		os.Remove(tmpName)
		return nil, err
	}

	hash, err := HashFile(tmpName)
	if err != nil {
		os.Remove(tmpName)
		return nil, fmt.Errorf("hash file: %w", err)
	}

	ref := StorageRef(hash, filename)
	destPath := filepath.Join(attachDir, ref)

	if _, err := os.Stat(destPath); err == nil {
		// Content already stored, discard temp
		os.Remove(tmpName)
	} else {
		if err := os.Rename(tmpName, destPath); err != nil {
			os.Remove(tmpName)
			return nil, fmt.Errorf("rename to storage: %w", err)
		}
	}

	return &AttachmentRef{
		Filename:   filename,
		StorageRef: ref,
	}, nil
}

// WriteAttachmentManifest writes the attachments.json manifest.
func WriteAttachmentManifest(path string, refs []AttachmentRef) error {
	data, err := json.MarshalIndent(refs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// ReadAttachmentManifest reads the attachments.json manifest.
func ReadAttachmentManifest(path string) ([]AttachmentRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var refs []AttachmentRef
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return refs, nil
}
