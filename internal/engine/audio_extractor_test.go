package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCompanionAudioFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "audio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// If media file is already audio, should return filename immediately
	audioSrc := filepath.Join(tmpDir, "sample_audio.mp3")
	_ = os.WriteFile(audioSrc, []byte("fake mp3 content"), 0644)

	res := ExtractCompanionAudioFile(context.Background(), audioSrc, tmpDir, "mp3")
	if res != "sample_audio.mp3" {
		t.Errorf("Expected sample_audio.mp3, got %s", res)
	}
}
