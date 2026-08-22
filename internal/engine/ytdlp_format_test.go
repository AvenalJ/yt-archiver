package engine

import (
	"testing"
	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
)

func TestExtractMaxHeight(t *testing.T) {
	tests := []struct {
		quality  string
		expected int
	}{
		{"best", 0},
		{"Best Available", 0},
		{"", 0},
		{"4320p", 4320},
		{"8K", 4320},
		{"2160p", 2160},
		{"4K", 2160},
		{"1440p", 1440},
		{"2K", 1440},
		{"1080p", 1080},
		{"720p", 720},
		{"480p", 480},
		{"360p", 360},
	}

	for _, tt := range tests {
		got := extractMaxHeight(tt.quality)
		if got != tt.expected {
			t.Errorf("extractMaxHeight(%q) = %d; want %d", tt.quality, got, tt.expected)
		}
	}
}

func TestBuildYtDlpArgs(t *testing.T) {
	cfg := &config.AppConfig{
		YtDlpCmd:   []string{"yt-dlp"},
		FFmpegPath: `C:\ffmpeg\bin\ffmpeg.exe`,
		JSRuntime:  "node",
	}

	args := cfg.BuildYtDlpArgs("--continue", "--no-part")
	expected := []string{
		"yt-dlp",
		"--js-runtimes", "node",
		"--ffmpeg-location", `C:\ffmpeg\bin\ffmpeg.exe`,
		"--retries", "10",
		"--fragment-retries", "10",
		"--file-access-retries", "3",
		"--retry-sleep", "5",
		"--socket-timeout", "30",
		"--extractor-retries", "3",
		"--http-chunk-size", "10M",
		"--continue", "--no-part",
	}

	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d:\nGot:  %v\nWant: %v", len(expected), len(args), args, expected)
	}

	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestPreferencesDefaults(t *testing.T) {
	prefs := db.DefaultPreferences("./downloads")
	if prefs.VideoQuality == "" {
		t.Errorf("default VideoQuality should not be empty")
	}
}
