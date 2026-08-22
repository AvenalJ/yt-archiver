package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"youtube-downloader/internal/db"
	"youtube-downloader/internal/logger"
)

type apiSponsorSegment struct {
	Category   string    `json:"category"`
	ActionType string    `json:"actionType"`
	Segment    []float64 `json:"segment"` // [startSec, endSec]
	UUID       string    `json:"UUID"`
}

// FetchSponsorBlockSegments queries the public SponsorBlock API for community skip segments
func FetchSponsorBlockSegments(ctx context.Context, videoID string, outputDir string) []db.SponsorSegment {
	if videoID == "" {
		return nil
	}

	// First check if sponsorblock.json already exists in outputDir
	if outputDir != "" {
		cachedPath := filepath.Join(outputDir, "sponsorblock.json")
		if data, err := os.ReadFile(cachedPath); err == nil {
			var cached []db.SponsorSegment
			if json.Unmarshal(data, &cached) == nil && len(cached) > 0 {
				return cached
			}
		}
	}

	apiURL := fmt.Sprintf(
		"https://sponsor.ajay.app/api/skipSegments?videoID=%s&categories=%s",
		url.QueryEscape(videoID),
		url.QueryEscape(`["sponsor","intro","outro","selfpromo","preview","interaction","music_offtopic"]`),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) YT-Archiver/2.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 404 means no sponsor segments found for this video
		return nil
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}

	var apiSegs []apiSponsorSegment
	if err := json.Unmarshal(bodyBytes, &apiSegs); err != nil {
		return nil
	}

	var segments []db.SponsorSegment
	for _, item := range apiSegs {
		start := 0.0
		end := 0.0
		if len(item.Segment) >= 2 {
			start = item.Segment[0]
			end = item.Segment[1]
		}
		action := item.ActionType
		if action == "" {
			action = "skip"
		}
		segments = append(segments, db.SponsorSegment{
			Category:  item.Category,
			Action:    action,
			StartTime: start,
			EndTime:   end,
			UUID:      item.UUID,
		})
	}

	if len(segments) > 0 && outputDir != "" {
		sponsorPath := filepath.Join(outputDir, "sponsorblock.json")
		if formattedJSON, err := json.MarshalIndent(segments, "", "  "); err == nil {
			_ = os.WriteFile(sponsorPath, formattedJSON, 0644)
		}
		logger.Infof("[SponsorBlock] Fetched and saved %d skip segments for video %s", len(segments), videoID)
	}

	return segments
}
