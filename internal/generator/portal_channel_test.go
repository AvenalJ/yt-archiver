package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AvenalJ/yt-archiver/internal/engine"
)

func TestGenerateChannelHTML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_channel_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	meta := &engine.ChannelMetadata{
		Title:                "Test Channel",
		Handle:               "@TestChannel",
		URL:                  "https://www.youtube.com/@TestChannel",
		CanonicalURL:         "http://www.youtube.com/@TestChannel",
		SubscriberCount:      474000,
		FormattedSubscribers: "474K subscribers",
		TotalVideos:          923,
		TotalVideosText:      "923 videos",
		TotalViews:           193852566,
		FormattedTotalViews:  "193,852,566 views",
		JoinedDate:           "Joined Mar 27, 2017",
		Country:              "New Zealand",
		Description:          "Welcome to the official Test Channel!",
		IsVerified:           true,
		AvatarFilename:       "channel_avatar.jpg",
		BannerFilename:       "channel_banner.jpg",
		Links: []engine.ChannelLink{
			{Title: "Twitter", URL: "x.com/test"},
			{Title: "Discord", URL: "discord.gg/test"},
		},
	}

	videos := []ChannelVideoItem{
		{
			Title:             "Sample Video 1",
			VideoID:           "vid1",
			RelativeURL:       "index.html",
			ThumbnailFilename: "thumb.jpg",
			Duration:          300,
			FormattedDuration: "5:00",
			FormattedViews:    "100K",
			UploadDate:        "2026-01-01",
		},
	}

	dest, err := GenerateChannelHTML(tempDir, meta, videos)
	if err != nil {
		t.Fatalf("GenerateChannelHTML failed: %v", err)
	}

	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read generated channel.html: %v", err)
	}

	htmlStr := string(content)
	if !strings.Contains(htmlStr, "Test Channel") {
		t.Errorf("channel.html missing channel title")
	}
	if !strings.Contains(htmlStr, "474K subscribers") {
		t.Errorf("channel.html missing subscriber count")
	}
	if !strings.Contains(htmlStr, "New Zealand") {
		t.Errorf("channel.html missing country")
	}
	if !strings.Contains(htmlStr, "Joined Mar 27, 2017") {
		t.Errorf("channel.html missing joined date")
	}
	if !strings.Contains(htmlStr, "193,852,566 views") {
		t.Errorf("channel.html missing total views")
	}
	if !strings.Contains(htmlStr, "Twitter") || !strings.Contains(htmlStr, "x.com/test") {
		t.Errorf("channel.html missing external links")
	}
	if !strings.Contains(htmlStr, "Sample Video 1") {
		t.Errorf("channel.html missing sample video item")
	}
}

func TestRegisterVideoInMasterPortal(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_portal_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	item := &PortalCatalogItem{
		VideoID:              "abc12345",
		Title:                "Master 4K Video Test",
		Channel:              "Creator Studio",
		ChannelURL:           "https://www.youtube.com/@CreatorStudio",
		Duration:             600,
		ViewCount:            500000,
		UploadDate:           "2026-05-10",
		Description:          "Testing master portal catalog registration.",
		Tags:                 []string{"4K", "OLED", "Testing"},
		RelativePlayerURL:    "videos/abc12345/index.html",
		RelativeThumbnailURL: "videos/abc12345/thumb.jpg",
		RelativeAvatarURL:    "videos/abc12345/channel_avatar.jpg",
		RelativeChannelURL:   "videos/abc12345/channel.html",
	}

	chanMeta := &engine.ChannelMetadata{
		Title:                "Creator Studio",
		Handle:               "@CreatorStudio",
		FormattedSubscribers: "2.4M subscribers",
	}

	err = RegisterVideoInMasterPortal(tempDir, item, chanMeta)
	if err != nil {
		t.Fatalf("RegisterVideoInMasterPortal failed: %v", err)
	}

	// Verify catalog.json
	catalogPath := filepath.Join(tempDir, "catalog.json")
	catData, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("failed to read catalog.json: %v", err)
	}
	if !strings.Contains(string(catData), "abc12345") {
		t.Errorf("catalog.json missing registered videoID")
	}

	// Verify downloads/index.html
	indexPath := filepath.Join(tempDir, "index.html")
	idxData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read portal index.html: %v", err)
	}
	if !strings.Contains(string(idxData), "Master 4K Video Test") {
		t.Errorf("portal index.html missing video title")
	}
	if !strings.Contains(string(idxData), "Creator Studio") {
		t.Errorf("portal index.html missing channel name")
	}
}
