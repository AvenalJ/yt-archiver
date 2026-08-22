package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBatchDownloadsAndPagination(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "yt_archiver_batch_db_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "batch_test.db")
	database, err := InitDB(dbPath, tempDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	// 1. Create a large batch of items (500 items)
	var items []*DownloadItem
	var videoIDs []string
	var urls []string

	for i := 1; i <= 500; i++ {
		vID := fmt.Sprintf("vid_%04d", i)
		vURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", vID)
		videoIDs = append(videoIDs, vID)
		urls = append(urls, vURL)

		status := "queued"
		if i%5 == 0 {
			status = "completed"
		} else if i%3 == 0 {
			status = "failed"
		}

		items = append(items, &DownloadItem{
			ID:            fmt.Sprintf("dl_%04d", i),
			URL:           vURL,
			VideoID:       vID,
			PlaylistID:    "PL_test_10k",
			PlaylistTitle: "Large Channel Playlist",
			PlaylistIndex: i,
			PlaylistTotal: 500,
			Title:         fmt.Sprintf("Video Title %04d", i),
			Channel:       "Tech Channel",
			ChannelURL:    "https://www.youtube.com/@TechChannel",
			Duration:      120 + int64(i),
			ThumbnailURL:  fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", vID),
			Format:        "mp4",
			Quality:       "1080p",
			Status:        status,
			CurrentStep:   "Waiting in queue",
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	// Test CreateDownloadsBatch
	start := time.Now()
	if err := database.CreateDownloadsBatch(items); err != nil {
		t.Fatalf("CreateDownloadsBatch failed: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("CreateDownloadsBatch (500 items) took: %v", elapsed)

	// 2. Test FindDuplicatesBatch
	dupMap, err := database.FindDuplicatesBatch(videoIDs[:100], urls[100:200])
	if err != nil {
		t.Fatalf("FindDuplicatesBatch failed: %v", err)
	}
	if len(dupMap) < 200 {
		t.Errorf("Expected at least 200 duplicates in map, got %d", len(dupMap))
	}
	if dupMap["vid_0001"] == nil || dupMap["vid_0001"].Title != "Video Title 0001" {
		t.Errorf("Expected vid_0001 to be found in dupMap")
	}

	// 3. Test GetDownloadsPaged
	// Page 1: 50 items
	page1, total, err := database.GetDownloadsPaged("all", "", 50, 0)
	if err != nil {
		t.Fatalf("GetDownloadsPaged page 1 failed: %v", err)
	}
	if total != 500 {
		t.Errorf("Expected total=500, got %d", total)
	}
	if len(page1) != 50 {
		t.Errorf("Expected 50 items on page 1, got %d", len(page1))
	}

	// Test status filter
	completedItems, completedTotal, err := database.GetDownloadsPaged("completed", "", 20, 0)
	if err != nil {
		t.Fatalf("GetDownloadsPaged completed failed: %v", err)
	}
	if completedTotal != 100 { // 500 / 5 = 100
		t.Errorf("Expected 100 completed items, got %d", completedTotal)
	}
	if len(completedItems) != 20 {
		t.Errorf("Expected 20 items on page 1 of completed, got %d", len(completedItems))
	}
}
