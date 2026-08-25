package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AvenalJ/yt-archiver/internal/db"
)

type DuplicateCheckResult struct {
	IsDuplicate     bool             `json:"is_duplicate"`
	ExistingItem    *db.DownloadItem `json:"existing_item,omitempty"`
	Reason          string           `json:"reason,omitempty"`
	FileExists      bool             `json:"file_exists"`
	SuggestedAction string           `json:"suggested_action"`
}

func CheckDuplicate(videoID, url string, database *db.DB) (*DuplicateCheckResult, error) {
	if videoID == "" {
		videoID = ExtractVideoID(url)
	}

	existing, err := database.FindDuplicate(videoID, url)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		return &DuplicateCheckResult{
			IsDuplicate:     false,
			SuggestedAction: "download",
		}, nil
	}

	fileExists := false
	if existing.MediaFilePath != "" {
		if _, err := os.Stat(existing.MediaFilePath); err == nil {
			fileExists = true
		}
	}
	if !fileExists && existing.HTMLFilePath != "" {
		if _, err := os.Stat(existing.HTMLFilePath); err == nil {
			fileExists = true
		}
	}
	if !fileExists && existing.OutputDir != "" {
		if _, err := os.Stat(existing.OutputDir); err == nil {
			if entries, err := os.ReadDir(existing.OutputDir); err == nil && len(entries) > 0 {
				for _, e := range entries {
					if !e.IsDir() {
						ext := strings.ToLower(filepath.Ext(e.Name()))
						if ext == ".mp4" || ext == ".webm" || ext == ".mkv" || ext == ".mp3" || ext == ".m4a" || ext == ".html" {
							fileExists = true
							break
						}
					}
				}
			}
		}
	}

	statusReason := fmt.Sprintf("Already in library with status: %s", existing.Status)
	if existing.Status == "completed" && fileExists {
		statusReason = "Already downloaded completely and file exists on disk"
	} else if existing.Status == "downloading" || existing.Status == "queued" {
		statusReason = "Currently downloading or queued in line"
	}

	return &DuplicateCheckResult{
		IsDuplicate:     true,
		ExistingItem:    existing,
		Reason:          statusReason,
		FileExists:      fileExists,
		SuggestedAction: "skip",
	}, nil
}

// CheckDuplicatesBatch checks duplicate status for a batch of video items efficiently
func CheckDuplicatesBatch(items []InspectVideoItem, database *db.DB) (map[string]*DuplicateCheckResult, error) {
	result := make(map[string]*DuplicateCheckResult)
	if len(items) == 0 {
		return result, nil
	}

	var videoIDs []string
	var urls []string
	for _, item := range items {
		if item.ID != "" {
			videoIDs = append(videoIDs, item.ID)
		}
		if item.URL != "" {
			urls = append(urls, item.URL)
		}
	}

	existingMap, err := database.FindDuplicatesBatch(videoIDs, urls)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		key := item.ID
		if key == "" {
			key = item.URL
		}

		existing := existingMap[item.ID]
		if existing == nil && item.URL != "" {
			existing = existingMap[item.URL]
		}

		if existing == nil {
			result[key] = &DuplicateCheckResult{
				IsDuplicate:     false,
				SuggestedAction: "download",
			}
			continue
		}

		fileExists := false
		if existing.MediaFilePath != "" {
			if _, err := os.Stat(existing.MediaFilePath); err == nil {
				fileExists = true
			}
		}
		if !fileExists && existing.HTMLFilePath != "" {
			if _, err := os.Stat(existing.HTMLFilePath); err == nil {
				fileExists = true
			}
		}
		if !fileExists && existing.OutputDir != "" {
			if _, err := os.Stat(existing.OutputDir); err == nil {
				if entries, err := os.ReadDir(existing.OutputDir); err == nil && len(entries) > 0 {
					for _, e := range entries {
						if !e.IsDir() {
							ext := strings.ToLower(filepath.Ext(e.Name()))
							if ext == ".mp4" || ext == ".webm" || ext == ".mkv" || ext == ".mp3" || ext == ".m4a" || ext == ".html" {
								fileExists = true
								break
							}
						}
					}
				}
			}
		}

		statusReason := fmt.Sprintf("Already in library with status: %s", existing.Status)
		if existing.Status == "completed" && fileExists {
			statusReason = "Already downloaded completely and file exists on disk"
		} else if existing.Status == "downloading" || existing.Status == "queued" {
			statusReason = "Currently downloading or queued in line"
		}

		result[key] = &DuplicateCheckResult{
			IsDuplicate:     true,
			ExistingItem:    existing,
			Reason:          statusReason,
			FileExists:      fileExists,
			SuggestedAction: "skip",
		}
	}

	return result, nil
}
