package queue

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"youtube-downloader/internal/db"
)

func TestPauseAllAndResumeAll(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.InitDB(dbPath, tmpDir)
	if err != nil {
		t.Fatalf("Failed to init test db: %v", err)
	}
	defer database.Close()

	qm := &QueueManager{
		db:          database,
		wakeUpCh:    make(chan struct{}, 10),
		activeIDs:   make(map[string]bool),
		cancelFuncs: make(map[string]context.CancelFunc),
		retryAt:     make(map[string]time.Time),
	}

	// Insert active and queued downloads
	item1 := &db.DownloadItem{
		ID:        "dl_1",
		VideoID:   "vid_1",
		Title:     "Downloading Video",
		Status:    "downloading",
		CreatedAt: time.Now(),
	}
	item2 := &db.DownloadItem{
		ID:        "dl_2",
		VideoID:   "vid_2",
		Title:     "Queued Video",
		Status:    "queued",
		CreatedAt: time.Now(),
	}
	_ = database.CreateDownload(item1)
	_ = database.CreateDownload(item2)

	// Pause all
	qm.PauseAll()

	// Verify all items are now paused in the database
	d1, err := database.GetDownload("dl_1")
	if err != nil || d1.Status != "paused" {
		t.Fatalf("expected dl_1 to be paused, got %v (err: %v)", d1.Status, err)
	}
	d2, err := database.GetDownload("dl_2")
	if err != nil || d2.Status != "paused" {
		t.Fatalf("expected dl_2 to be paused, got %v (err: %v)", d2.Status, err)
	}

	// Resume all
	qm.ResumeAll()

	// Verify all items are now queued in the database
	d1After, err := database.GetDownload("dl_1")
	if err != nil || d1After.Status != "queued" {
		t.Fatalf("expected dl_1 to be queued after resume, got %v (err: %v)", d1After.Status, err)
	}
	d2After, err := database.GetDownload("dl_2")
	if err != nil || d2After.Status != "queued" {
		t.Fatalf("expected dl_2 to be queued after resume, got %v (err: %v)", d2After.Status, err)
	}
}
