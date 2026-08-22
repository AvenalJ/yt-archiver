package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionLoggerAndRetention(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "yt_archiver_log_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logsDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}

	// Create 15 dummy older session logs
	now := time.Now()
	for i := 1; i <= 15; i++ {
		dummyName := fmt.Sprintf("session_2026-01-%02d_12-00-00.log", i)
		dummyPath := filepath.Join(logsDir, dummyName)
		if err := os.WriteFile(dummyPath, []byte(fmt.Sprintf("log content %d", i)), 0644); err != nil {
			t.Fatalf("failed to write dummy log: %v", err)
		}
		// Stagger mod times
		modTime := now.Add(time.Duration(i-20) * time.Hour)
		_ = os.Chtimes(dummyPath, modTime, modTime)
	}

	// Initialize logger which should prune down to 10
	l, err := InitLogger(tempDir)
	if err != nil {
		t.Fatalf("InitLogger failed: %v", err)
	}
	defer l.Close()

	Infof("Testing info message")
	Errorf("Testing error message")
	Warnf("Testing warn message")
	LogFailure("downloader", "dl_123", "Test Video", "https://youtube.com/watch?v=123", fmt.Errorf("sample error"), "yt-dlp stderr output", "additional detail")
	LogSuccess("downloader", "dl_123", "Test Video", "1080p", "15MB")

	// Read logs directory and count files
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("failed to read logs dir: %v", err)
	}

	sessionLogCount := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			sessionLogCount++
		}
	}

	if sessionLogCount > MaxSessionLogs {
		t.Errorf("expected at most %d log files, found %d", MaxSessionLogs, sessionLogCount)
	}

	// Verify log file has content
	content, err := os.ReadFile(l.filePath)
	if err != nil {
		t.Fatalf("failed to read session log file: %v", err)
	}

	if len(content) == 0 {
		t.Errorf("expected session log to contain log content, but it was empty")
	}
}
