package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AvenalJ/yt-archiver/internal/db"
)

func TestChaptersAndSponsorBlockInHTML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "html_chapters_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	input := &HTMLGeneratorInput{
		Title:             "Test Chapter Video",
		VideoID:           "test12345",
		Channel:           "Tech Science",
		ChannelURL:        "https://youtube.com/@techscience",
		Duration:          300,
		ViewCount:         100000,
		LikeCount:         5000,
		UploadDate:        "20260816",
		Description:       "Test Description with chapters",
		MediaFilename:     "video.mp4",
		ThumbnailFilename: "thumbnail.jpg",
		Chapters: []db.VideoChapter{
			{Title: "Introduction", StartTime: 0, EndTime: 60},
			{Title: "Deep Dive", StartTime: 60, EndTime: 200},
			{Title: "Conclusion", StartTime: 200, EndTime: 300},
		},
		SponsorSegments: []db.SponsorSegment{
			{Category: "sponsor", Action: "skip", StartTime: 75, EndTime: 110, UUID: "sb-123"},
		},
	}

	htmlPath, err := GenerateOfflineHTML(tempDir, input)
	if err != nil {
		t.Fatalf("GenerateOfflineHTML failed: %v", err)
	}

	content, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read generated HTML: %v", err)
	}

	htmlStr := string(content)

	if !strings.Contains(htmlStr, "Video Chapters (3)") {
		t.Errorf("Expected Chapters header in HTML")
	}
	if !strings.Contains(htmlStr, "Introduction") || !strings.Contains(htmlStr, "Deep Dive") {
		t.Errorf("Expected chapter titles in HTML")
	}
	if !strings.Contains(htmlStr, "sb-toggle-btn") {
		t.Errorf("Expected SponsorBlock toggle button in HTML")
	}
	if !strings.Contains(htmlStr, "rawChapters") || !strings.Contains(htmlStr, "sponsorSegments") {
		t.Errorf("Expected chapter and sponsor JS arrays in HTML")
	}
	if !filepath.IsAbs(htmlPath) {
		t.Errorf("Expected absolute path for htmlPath, got %s", htmlPath)
	}
}
