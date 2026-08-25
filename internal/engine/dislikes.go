package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/logger"
)

type ReturnYouTubeDislikeResult struct {
	ID        string  `json:"id"`
	Likes     int64   `json:"likes"`
	Dislikes  int64   `json:"dislikes"`
	Rating    float64 `json:"rating"`
	ViewCount int64   `json:"viewCount"`
	Deleted   bool    `json:"deleted"`
}

// FetchReturnYouTubeDislike queries the Return YouTube Dislike API to obtain accurate dislike counts
func FetchReturnYouTubeDislike(ctx context.Context, videoID string) (*ReturnYouTubeDislikeResult, error) {
	if videoID == "" {
		return nil, fmt.Errorf("video ID is empty")
	}

	reqURL := fmt.Sprintf("https://returnyoutubedislikeapi.com/votes?videoId=%s", videoID)

	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 4 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Warnf("[Dislikes] Failed to query Return YouTube Dislike API for %s: %v", videoID, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("RYD API returned status %d", resp.StatusCode)
		logger.Warnf("[Dislikes] %v for video %s", err, videoID)
		return nil, err
	}

	var res ReturnYouTubeDislikeResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logger.Warnf("[Dislikes] Failed to decode RYD response for %s: %v", videoID, err)
		return nil, err
	}

	return &res, nil
}
