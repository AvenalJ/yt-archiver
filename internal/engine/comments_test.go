package engine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAvatarsZipAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "avatar_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "avatars.zip")

	// Create a mock avatars.zip with 2 test images
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip: %v", err)
	}

	w := zip.NewWriter(f)
	// Entry 1
	w1, err := w.Create("abc12345.jpg")
	if err != nil {
		t.Fatalf("Failed to create zip entry 1: %v", err)
	}
	_, _ = w1.Write([]byte("fake-jpeg-binary-data-1"))

	// Entry 2
	w2, err := w.Create("def67890.jpg")
	if err != nil {
		t.Fatalf("Failed to create zip entry 2: %v", err)
	}
	_, _ = w2.Write([]byte("fake-jpeg-binary-data-2"))

	_ = w.Close()
	_ = f.Close()

	// Test LoadAvatarMapFromZip
	avatarMap, err := LoadAvatarMapFromZip(zipPath)
	if err != nil {
		t.Fatalf("LoadAvatarMapFromZip failed: %v", err)
	}

	if len(avatarMap) != 2 {
		t.Fatalf("Expected 2 avatars loaded, got %d", len(avatarMap))
	}

	if val, ok := avatarMap["abc12345"]; !ok || !strings.HasPrefix(val, "data:") {
		t.Errorf("Expected valid data URI for abc12345, got %v", val)
	}

	if val, ok := avatarMap["def67890"]; !ok || !strings.HasPrefix(val, "data:") {
		t.Errorf("Expected valid data URI for def67890, got %v", val)
	}
}
