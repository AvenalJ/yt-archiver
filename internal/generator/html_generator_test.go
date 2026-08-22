package generator

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateOfflineHTML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	input := &HTMLGeneratorInput{
		Title:                 "Test Video Title",
		VideoID:               "abc123xyz",
		Channel:               "Awesome Channel",
		ChannelURL:            "https://www.youtube.com/@awesome",
		ChannelAvatarFilename: "channel_avatar.jpg",
		SubscriberCount:       1420000,
		Duration:              125,
		ViewCount:             21500,
		LikeCount:             1400,
		DislikeCount:          45,
		UploadDate:            "2026-08-15",
		Description:           "This is a test description with 01:23 timestamp.",
		MediaFilename:         "test_video.mp4",
		ThumbnailFilename:     "test_thumb.jpg",
		CommentsCount:         5,
		AvatarMap: map[string]string{
			"a1b2c3d4": "data:image/jpeg;base64,dGVzdGF2YXRhcg==",
		},
		SourceURL:             "https://www.youtube.com/watch?v=abc123xyz",
		VideoQuality:          "1080p",
		Filesize:              15000000,
	}

	htmlPath, err := GenerateOfflineHTML(tmpDir, input)
	if err != nil {
		t.Fatalf("GenerateOfflineHTML failed: %v", err)
	}

	contentBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read generated HTML: %v", err)
	}
	content := string(contentBytes)

	// 1. Check Channel Avatar filename
	if !strings.Contains(content, `src="channel_avatar.jpg"`) {
		t.Errorf("Expected channel_avatar.jpg in img src, got:\n%s", content)
	}

	// 2. Check Subscriber Count formatted
	if !strings.Contains(content, "1.42M subscribers") {
		t.Errorf("Expected '1.42M subscribers', got:\n%s", content)
	}

	// 3. Ensure Subscribe button is removed
	if strings.Contains(content, "subscribe-btn") || strings.Contains(content, ">Subscribed<") {
		t.Errorf("Found Subscribed button in generated HTML!")
	}

	// 4. Ensure Share button is removed
	if strings.Contains(content, ">Share<") || strings.Contains(content, "copyShareLink") {
		t.Errorf("Found Share button in generated HTML!")
	}

	// 5. Ensure like button has the unliked outline path
	if !strings.Contains(content, "M9 21h9c.83 0 1.54-.5") {
		t.Errorf("Expected outline like SVG path in generated HTML")
	}

	// 6. Ensure Base64 AvatarMap is embedded in HTML
	if !strings.Contains(content, `"a1b2c3d4":"data:image/jpeg;base64,dGVzdGF2YXRhcg=="`) {
		t.Errorf("Expected Base64 AvatarMap to be embedded in HTML script")
	}

	// 7. Ensure Dislike count (45) is rendered
	if !strings.Contains(content, "<span>45</span>") {
		t.Errorf("Expected dislike count '45' in generated HTML")
	}

	// 8. Ensure Upload date is formatted (15 Aug 2026)
	if !strings.Contains(content, "15 Aug 2026") {
		t.Errorf("Expected formatted upload date '15 Aug 2026', got:\n%s", content)
	}
}

func TestFormatUploadDate(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"20260819", "19 Aug 2026"},
		{"2026-08-19", "19 Aug 2026"},
		{"20230101", "1 Jan 2023"},
		{"19 Aug 2026", "19 Aug 2026"},
		{"", ""},
	}

	for _, c := range cases {
		got := formatUploadDate(c.input)
		if got != c.expected {
			t.Errorf("formatUploadDate(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{950, "950"},
		{1000, "1K"},
		{1400, "1.4K"},
		{22000, "22K"},
		{22500, "22.5K"},
		{100000, "100K"},
		{999000, "999K"},
		{1000000, "1M"},
		{1420000, "1.42M"},
		{25000000, "25M"},
		{999000000, "999M"},
		{1000000000, "1B"},
		{1200000000, "1.2B"},
		{10500000000, "10.5B"},
		{25000000000, "25B"},
	}

	for _, c := range cases {
		got := formatNumber(c.input)
		if got != c.expected {
			t.Errorf("formatNumber(%d) = %q, expected %q", c.input, got, c.expected)
		}
	}
}
