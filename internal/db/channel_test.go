package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChannelCRUDAndPreferences(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "yt_archiver_db_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := InitDB(dbPath, tempDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	// 1. Test Preferences with Advanced Features
	prefs, err := database.GetPreferences()
	if err != nil {
		t.Fatalf("GetPreferences failed: %v", err)
	}
	prefs.CookieSource = "browser"
	prefs.CookieBrowser = "chrome"
	prefs.SponsorBlockAction = "mark"
	prefs.EmbedMetadata = true
	prefs.EmbedCoverArt = true
	prefs.EmbedChapters = true

	if err := database.SavePreferences(prefs); err != nil {
		t.Fatalf("SavePreferences failed: %v", err)
	}

	savedPrefs, err := database.GetPreferences()
	if err != nil || savedPrefs.CookieBrowser != "chrome" || savedPrefs.SponsorBlockAction != "mark" {
		t.Errorf("Preferences round-trip mismatch: %+v", savedPrefs)
	}

	// 2. Test Channel Subscriptions
	channel := &ChannelSubscription{
		ID:              "ch_123",
		ChannelID:       "UC123456789",
		Title:           "Veritasium",
		Handle:          "Veritasium",
		URL:             "https://www.youtube.com/@Veritasium",
		AvatarURL:       "https://example.com/avatar.jpg",
		SubscriberCount: 15000000,
		AutoDownload:    true,
		TotalVideos:     450,
		CreatedAt:       time.Now(),
	}

	if err := database.SaveChannel(channel); err != nil {
		t.Fatalf("SaveChannel failed: %v", err)
	}

	got, err := database.GetChannel("ch_123")
	if err != nil || got == nil {
		t.Fatalf("GetChannel failed: %v", err)
	}
	if got.Title != "Veritasium" || got.SubscriberCount != 15000000 {
		t.Errorf("Channel field mismatch: %+v", got)
	}

	allChannels, err := database.GetAllChannels()
	if err != nil || len(allChannels) != 1 {
		t.Errorf("GetAllChannels expected 1, got %d", len(allChannels))
	}

	if err := database.UpdateChannelSync("ch_123", 455); err != nil {
		t.Errorf("UpdateChannelSync failed: %v", err)
	}

	updated, _ := database.GetChannel("ch_123")
	if updated.TotalVideos != 455 || updated.LastSynced == nil {
		t.Errorf("UpdateChannelSync did not apply: %+v", updated)
	}

	if err := database.DeleteChannel("ch_123"); err != nil {
		t.Errorf("DeleteChannel failed: %v", err)
	}

	deleted, _ := database.GetChannel("ch_123")
	if deleted != nil {
		t.Errorf("Expected channel to be deleted, got %+v", deleted)
	}
}
