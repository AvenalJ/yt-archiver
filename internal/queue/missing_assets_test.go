package queue

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/db"
)

func TestScanAndQueueMissingAssets(t *testing.T) {
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

	// Create test download directories
	completeDir := filepath.Join(tmpDir, "complete_video")
	incompleteDir := filepath.Join(tmpDir, "incomplete_video")
	_ = os.MkdirAll(completeDir, 0755)
	_ = os.MkdirAll(incompleteDir, 0755)

	// Complete video files
	_ = os.WriteFile(filepath.Join(completeDir, "video.mp4"), []byte("mp4"), 0644)
	_ = os.WriteFile(filepath.Join(completeDir, "video.mp3"), []byte("mp3"), 0644)
	_ = os.WriteFile(filepath.Join(completeDir, "index.html"), []byte("<html></html>"), 0644)
	_ = os.WriteFile(filepath.Join(completeDir, "movie.nfo"), []byte("nfo"), 0644)
	_ = os.WriteFile(filepath.Join(completeDir, "avatars.zip"), []byte("zip"), 0644)

	// Incomplete video files (missing avatars.zip and comments)
	_ = os.WriteFile(filepath.Join(incompleteDir, "video.mp4"), []byte("mp4"), 0644)
	_ = os.WriteFile(filepath.Join(incompleteDir, "index.html"), []byte("<html></html>"), 0644)

	// Insert items into DB
	itemComplete := &db.DownloadItem{
		ID:              "dl_complete",
		VideoID:         "vid_complete",
		Title:           "Complete Video",
		Status:          "completed",
		OutputDir:       completeDir,
		CommentsCount:   50,
		CompletedStages: "media,comments,html",
		CreatedAt:       time.Now(),
	}
	_ = database.CreateDownload(itemComplete)
	_ = database.SaveComments("vid_complete", []*db.CommentItem{{ID: "c1", Author: "Test", Text: "Hello"}})

	itemIncomplete := &db.DownloadItem{
		ID:              "dl_incomplete",
		VideoID:         "vid_incomplete",
		Title:           "Incomplete Video",
		Status:          "completed",
		OutputDir:       incompleteDir,
		CommentsCount:   0,
		CompletedStages: "media,html",
		CreatedAt:       time.Now(),
	}
	_ = database.CreateDownload(itemIncomplete)

	scanned, queued, err := qm.ScanAndQueueMissingAssets()
	if err != nil {
		t.Fatalf("ScanAndQueueMissingAssets failed: %v", err)
	}

	if scanned != 2 {
		t.Errorf("Expected scanned=2, got %d", scanned)
	}
	if queued != 1 {
		t.Errorf("Expected queued=1, got %d", queued)
	}

	// Verify itemComplete remained completed
	cCheck, _ := database.GetDownload("dl_complete")
	if cCheck.Status != "completed" {
		t.Errorf("Expected complete item to remain 'completed', got '%s'", cCheck.Status)
	}

	// Verify itemIncomplete became queued with media stage preserved
	iCheck, _ := database.GetDownload("dl_incomplete")
	if iCheck.Status != "queued" {
		t.Errorf("Expected incomplete item to be 'queued', got '%s'", iCheck.Status)
	}
	if !strings.Contains(iCheck.CompletedStages, "media") {
		t.Errorf("Expected 'media' stage to be preserved in CompletedStages, got '%s'", iCheck.CompletedStages)
	}
}
