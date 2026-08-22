package generator

import (
	"os"
	"strings"
	"testing"

	"youtube-downloader/internal/db"
	"youtube-downloader/internal/engine"
)

func TestStoryboardAndLiveChatHTML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "html_advanced_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	input := &HTMLGeneratorInput{
		Title:             "Advanced Features Test Video",
		VideoID:           "adv123",
		Channel:           "Veritasium",
		ChannelURL:        "https://youtube.com/@Veritasium",
		Duration:          600,
		ViewCount:         1500000,
		LikeCount:         75000,
		MediaFilename:     "video.mp4",
		ThumbnailFilename: "thumb.jpg",
		Storyboard: &engine.StoryboardMetadata{
			StoryboardFilename: "storyboard.jpg",
			Cols:               10,
			Rows:               10,
			TotalFrames:        100,
			IntervalSeconds:    6.0,
			FrameWidth:         160,
			FrameHeight:        90,
		},
		LiveChat: []engine.LiveChatMessage{
			{
				Author:        "John Doe",
				Message:       "This video is incredible!",
				TimeOffsetMs:  12000,
				TimeFormatted: "0:12",
			},
			{
				Author:        "Super Fan",
				Message:       "Take my superchat!",
				TimeOffsetMs:  45000,
				TimeFormatted: "0:45",
				Superchat:     "$10.00",
			},
		},
		SponsorSegments: []db.SponsorSegment{
			{
				Category:  "sponsor",
				Action:    "skip",
				StartTime: 120.0,
				EndTime:   180.0,
				UUID:      "test-uuid",
			},
		},
	}

	htmlPath, err := GenerateOfflineHTML(tempDir, input)
	if err != nil {
		t.Fatalf("GenerateOfflineHTML failed: %v", err)
	}

	content, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read HTML: %v", err)
	}
	htmlStr := string(content)

	// Check storyboard
	if !strings.Contains(htmlStr, "storyboard-preview") {
		t.Errorf("Expected storyboard-preview element in HTML")
	}
	if !strings.Contains(htmlStr, "storyboard.jpg") {
		t.Errorf("Expected storyboard.jpg reference in HTML")
	}

	// Check live chat
	if !strings.Contains(htmlStr, "Live Chat Replay (2 messages)") {
		t.Errorf("Expected Live Chat Replay header in HTML")
	}

	// Check skip sponsor pill
	if !strings.Contains(htmlStr, "skip-sponsor-btn") {
		t.Errorf("Expected skip-sponsor-btn in HTML")
	}
}

func TestChannelCommunityAndPlaylistsHTML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "channel_advanced_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	meta := &engine.ChannelMetadata{
		Title:                "Veritasium",
		Handle:               "@Veritasium",
		URL:                  "https://youtube.com/@Veritasium",
		SubscriberCount:      16500000,
		FormattedSubscribers: "16.5M subscribers",
		TotalVideosText:      "420 videos",
		TotalViews:           2500000000,
		Country:              "United States",
		Description:          "An element of truth - videos about science, education, and anything interesting.",
		Playlists: []engine.ChannelPlaylistItem{
			{
				PlaylistID:   "PL123",
				Title:        "Quantum Physics",
				ThumbnailURL: "https://example.com/p1.jpg",
				VideoCount:   "15 videos",
			},
		},
		Community: []engine.ChannelCommunityPost{
			{
				PostID:    "post_001",
				Published: "2 days ago",
				Text:      "Excited to announce our new documentary!",
				LikeCount: "12K",
			},
		},
	}

	videos := []ChannelVideoItem{
		{
			Title:             "The Science of Thinking",
			VideoID:           "vid_001",
			RelativeURL:       "index.html",
			ThumbnailFilename: "thumb.jpg",
			Duration:          720,
		},
	}

	htmlPath, err := GenerateChannelHTML(tempDir, meta, videos)
	if err != nil {
		t.Fatalf("GenerateChannelHTML failed: %v", err)
	}

	content, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read channel HTML: %v", err)
	}
	htmlStr := string(content)

	if !strings.Contains(htmlStr, "Playlists (1)") {
		t.Errorf("Expected Playlists (1) tab in channel HTML")
	}
	if !strings.Contains(htmlStr, "Posts (1)") {
		t.Errorf("Expected Posts (1) tab in channel HTML")
	}
	if !strings.Contains(htmlStr, "Quantum Physics") {
		t.Errorf("Expected playlist title in channel HTML")
	}
	if !strings.Contains(htmlStr, "Excited to announce our new documentary!") {
		t.Errorf("Expected community post text in channel HTML")
	}
}
