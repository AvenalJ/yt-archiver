package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateNFOFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nfo_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = GenerateNFOFile(
		tmpDir,
		"Spider-Man: Into the Spider-Verse",
		"oJkM-eZu6U4",
		"Sony Pictures",
		120,
		"20260819",
		"A thrilling animated film.",
		[]string{"Animation", "Action"},
		[]string{"Spider-Man", "Marvel"},
		"poster.jpg",
		"banner.jpg",
		"avatar.jpg",
	)
	if err != nil {
		t.Fatalf("GenerateNFOFile failed: %v", err)
	}

	movieNFO := filepath.Join(tmpDir, "movie.nfo")
	content, err := os.ReadFile(movieNFO)
	if err != nil {
		t.Fatalf("Failed to read movie.nfo: %v", err)
	}

	xmlStr := string(content)
	if !strings.Contains(xmlStr, "<title>Spider-Man: Into the Spider-Verse</title>") {
		t.Errorf("Expected title in NFO")
	}
	if !strings.Contains(xmlStr, "<studio>Sony Pictures</studio>") {
		t.Errorf("Expected studio in NFO")
	}
	if !strings.Contains(xmlStr, "<premiered>2026-08-19</premiered>") {
		t.Errorf("Expected premiered date in NFO")
	}
	if !strings.Contains(xmlStr, "<thumb aspect=\"poster\">poster.jpg</thumb>") {
		t.Errorf("Expected poster thumb in NFO")
	}
}
