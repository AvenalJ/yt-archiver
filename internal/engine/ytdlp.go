package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/logger"
	"youtube-downloader/internal/sysutil"
)

type InspectResult struct {
	IsPlaylist bool               `json:"is_playlist"`
	PlaylistID string             `json:"playlist_id,omitempty"`
	Title      string             `json:"title"`
	Channel    string             `json:"channel"`
	ChannelID  string             `json:"channel_id,omitempty"`
	ChannelURL string             `json:"channel_url"`
	Thumbnail  string             `json:"thumbnail"`
	Duration   int64              `json:"duration"`
	ItemCount  int                `json:"item_count"`
	Items      []InspectVideoItem `json:"items,omitempty"`
	Formats         []db.FormatSpec    `json:"formats,omitempty"`
	Chapters        []db.VideoChapter  `json:"chapters,omitempty"`
	IsDuplicate     bool               `json:"is_duplicate,omitempty"`
	DuplicateReason string             `json:"duplicate_reason,omitempty"`
}

type InspectVideoItem struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Channel    string `json:"channel"`
	ChannelURL string `json:"channel_url,omitempty"`
	Duration   int64  `json:"duration"`
	Thumbnail  string `json:"thumbnail"`
	Index      int    `json:"index"`
	Category   string `json:"category,omitempty"`
}

type ChannelCatalogResult struct {
	ChannelID       string             `json:"channel_id"`
	Title           string             `json:"title"`
	Handle          string             `json:"handle"`
	URL             string             `json:"url"`
	AvatarURL       string             `json:"avatar_url"`
	SubscriberCount int64              `json:"subscriber_count"`
	TotalVideos     int                `json:"total_videos"`
	Videos          []InspectVideoItem `json:"videos"`
}

// isSingleVideoURL detects if a URL is a single video rather than a channel or playlist
func isSingleVideoURL(rawURL string) bool {
	u := strings.ToLower(rawURL)
	if strings.Contains(u, "playlist?list=") || strings.Contains(u, "/@") || strings.Contains(u, "/channel/") || strings.Contains(u, "/c/") || strings.Contains(u, "/user/") || strings.Contains(u, "/videos") || strings.Contains(u, "/featured") || strings.Contains(u, "/playlists") || strings.HasSuffix(u, "/shorts") || strings.Contains(u, "/shorts?") {
		return false
	}
	if strings.Contains(u, "watch?v=") || strings.Contains(u, "youtu.be/") || strings.Contains(u, "/shorts/") || strings.Contains(u, "/live/") || strings.Contains(u, "/embed/") || strings.Contains(u, "/v/") {
		return true
	}
	return false
}

// NormalizeYouTubeURL handles raw handles (@name), channel IDs, short URLs, and missing protocols
func NormalizeYouTubeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	// 1. If it starts with @ (e.g. @CasuallyExplained)
	if strings.HasPrefix(rawURL, "@") {
		return "https://www.youtube.com/" + rawURL
	}

	// 2. If it's a raw channel ID like UC... (24 chars)
	if strings.HasPrefix(rawURL, "UC") && len(rawURL) == 24 && !strings.Contains(rawURL, "/") {
		return "https://www.youtube.com/channel/" + rawURL
	}

	// 3. If it starts with youtube.com, m.youtube.com, or youtu.be without protocol
	if strings.HasPrefix(rawURL, "youtube.com/") || strings.HasPrefix(rawURL, "www.youtube.com/") ||
		strings.HasPrefix(rawURL, "m.youtube.com/") || strings.HasPrefix(rawURL, "youtu.be/") {
		return "https://" + rawURL
	}

	// 4. If it has no scheme at all
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		// If it looks like a handle or channel slug without @
		if !strings.Contains(rawURL, "/") && !strings.Contains(rawURL, ".") {
			return "https://www.youtube.com/@" + rawURL
		}
		return "https://" + rawURL
	}

	return rawURL
}

// InspectURL inspects a single video, playlist, or channel URL and returns details
func InspectURL(rawURL string) (*InspectResult, error) {
	return InspectURLWithContext(context.Background(), rawURL)
}

// InspectURLWithContext inspects a video, playlist, or channel URL with an adaptive timeout and parent context
func InspectURLWithContext(ctx context.Context, rawURL string) (*InspectResult, error) {
	rawURL = NormalizeYouTubeURL(rawURL)
	cfg := config.GlobalConfig
	if len(cfg.YtDlpCmd) == 0 {
		err := fmt.Errorf("yt-dlp command not configured")
		logger.Errorf("[Inspect] %v", err)
		return nil, err
	}

	logger.Infof("[Inspect] Inspecting URL: %s", rawURL)

	// Adaptive timeout: 25 seconds for single videos, 15 minutes for playlists/channels
	timeout := 15 * time.Minute
	if isSingleVideoURL(rawURL) {
		timeout = 25 * time.Second
	}

	if ctx == nil {
		ctx = context.Background()
	}
	inspectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Normalize channel URL to /videos for direct video tab listing
	inspectURL := strings.TrimRight(rawURL, "/")
	if !strings.HasSuffix(inspectURL, "/videos") && (strings.Contains(inspectURL, "/@") || strings.Contains(inspectURL, "/channel/") || strings.Contains(inspectURL, "/c/")) && !strings.Contains(inspectURL, "playlist?list=") {
		inspectURL += "/videos"
	}

	// Load preferences for cookies if available
	var cookieArgs []string
	if db.GlobalDB != nil {
		if prefs, _ := db.GlobalDB.GetPreferences(); prefs != nil {
			cookieArgs = BuildCookieArgs(prefs)
		}
	}

	// Use --flat-playlist --dump-single-json to inspect quickly
	args := cfg.BuildYtDlpArgs("--socket-timeout", "10", "--retries", "3")
	if len(cookieArgs) > 0 {
		args = append(args, cookieArgs...)
	}
	args = append(args, "--dump-single-json", "--flat-playlist", "--no-warnings", inspectURL)

	cmd := exec.CommandContext(inspectCtx, args[0], args[1:]...)
	sysutil.HideWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If cookies failed (e.g. browser locked or not installed), retry without cookies
		if len(cookieArgs) > 0 {
			logger.Warnf("[Inspect] Cookie extraction failed during inspect for %s, retrying without cookies...", inspectURL)
			argsNoCookies := cfg.BuildYtDlpArgs("--socket-timeout", "10", "--retries", "3", "--dump-single-json", "--flat-playlist", "--no-warnings", inspectURL)
			cmdRetry := exec.CommandContext(inspectCtx, argsNoCookies[0], argsNoCookies[1:]...)
			sysutil.HideWindow(cmdRetry)
			var stdout2, stderr2 bytes.Buffer
			cmdRetry.Stdout = &stdout2
			cmdRetry.Stderr = &stderr2
			if err2 := cmdRetry.Run(); err2 == nil {
				stdout = stdout2
				stderr = stderr2
				goto parseInspectJSON
			}
		}
		logger.LogFailure("InspectURL", "", "", rawURL, err, stderr.String())
		return nil, fmt.Errorf("failed to inspect URL: %w (stderr: %s)", err, stderr.String())
	}

parseInspectJSON:
	var raw map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		logger.Errorf("[Inspect] Failed to parse JSON for %s: %v", rawURL, err)
		return nil, fmt.Errorf("failed to parse yt-dlp metadata: %w", err)
	}

	res := &InspectResult{}
	rawType, _ := raw["_type"].(string)

	if rawType == "playlist" || strings.Contains(rawURL, "playlist?list=") || strings.Contains(rawURL, "/@") || strings.Contains(rawURL, "/channel/") {
		res.IsPlaylist = true
		res.Title, _ = raw["title"].(string)
		res.Channel, _ = raw["uploader"].(string)
		if res.Channel == "" {
			res.Channel, _ = raw["channel"].(string)
		}
		res.ChannelID, _ = raw["channel_id"].(string)
		res.ChannelURL, _ = raw["channel_url"].(string)
		if res.ChannelURL == "" {
			res.ChannelURL, _ = raw["uploader_url"].(string)
		}
		res.PlaylistID, _ = raw["id"].(string)
		res.Thumbnail, _ = raw["thumbnail"].(string)

		entries, ok := raw["entries"].([]interface{})
		if ok {
			res.ItemCount = len(entries)
			for i, entry := range entries {
				eMap, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				vID, _ := eMap["id"].(string)
				vTitle, _ := eMap["title"].(string)
				vUploader, _ := eMap["uploader"].(string)
				if vUploader == "" {
					vUploader, _ = eMap["channel"].(string)
				}
				vURL, _ := eMap["url"].(string)
				if vURL == "" && vID != "" {
					vURL = "https://www.youtube.com/watch?v=" + vID
				}
				var vDuration int64
				if durVal, ok := eMap["duration"].(float64); ok {
					vDuration = int64(durVal)
				}

				var vThumb string
				if thumbs, ok := eMap["thumbnails"].([]interface{}); ok && len(thumbs) > 0 {
					if lastThumb, ok := thumbs[len(thumbs)-1].(map[string]interface{}); ok {
						vThumb, _ = lastThumb["url"].(string)
					}
				}
				if vThumb == "" {
					vThumb, _ = eMap["thumbnail"].(string)
				}

				res.Items = append(res.Items, InspectVideoItem{
					ID:        vID,
					URL:       vURL,
					Title:     vTitle,
					Channel:   vUploader,
					Duration:  vDuration,
					Thumbnail: vThumb,
					Index:     i + 1,
				})
			}
		}
		return res, nil
	}

	// Single video
	res.IsPlaylist = false
	res.Title, _ = raw["title"].(string)
	res.Channel, _ = raw["uploader"].(string)
	if res.Channel == "" {
		res.Channel, _ = raw["channel"].(string)
	}
	uploaderURL, _ := raw["uploader_url"].(string)
	channelURL, _ := raw["channel_url"].(string)
	if strings.Contains(uploaderURL, "/@") {
		res.ChannelURL = uploaderURL
	} else if strings.Contains(channelURL, "/@") {
		res.ChannelURL = channelURL
	} else if uploaderID, _ := raw["uploader_id"].(string); strings.HasPrefix(uploaderID, "@") {
		res.ChannelURL = "https://www.youtube.com/" + uploaderID
	} else if uploaderURL != "" {
		res.ChannelURL = uploaderURL
	} else {
		res.ChannelURL = channelURL
	}

	if durVal, ok := raw["duration"].(float64); ok {
		res.Duration = int64(durVal)
	}
	res.Thumbnail, _ = raw["thumbnail"].(string)
	res.ItemCount = 1

	vID, _ := raw["id"].(string)
	res.Items = append(res.Items, InspectVideoItem{
		ID:         vID,
		URL:        rawURL,
		Title:      res.Title,
		Channel:    res.Channel,
		ChannelURL: res.ChannelURL,
		Duration:   res.Duration,
		Thumbnail:  res.Thumbnail,
		Index:      1,
	})

	// Parse chapters
	if chList, ok := raw["chapters"].([]interface{}); ok {
		for _, ch := range chList {
			if chMap, ok := ch.(map[string]interface{}); ok {
				cTitle, _ := chMap["title"].(string)
				cStart, _ := chMap["start_time"].(float64)
				cEnd, _ := chMap["end_time"].(float64)
				res.Chapters = append(res.Chapters, db.VideoChapter{
					Title:     cTitle,
					StartTime: cStart,
					EndTime:   cEnd,
				})
			}
		}
	}

	// Parse format list for format selector
	if formats, ok := raw["formats"].([]interface{}); ok {
		for _, f := range formats {
			fMap, ok := f.(map[string]interface{})
			if !ok {
				continue
			}
			fID, _ := fMap["format_id"].(string)
			ext, _ := fMap["ext"].(string)
			resStr, _ := fMap["resolution"].(string)
			note, _ := fMap["format_note"].(string)
			vcodec, _ := fMap["vcodec"].(string)
			acodec, _ := fMap["acodec"].(string)
			var fSize int64
			if sz, ok := fMap["filesize"].(float64); ok {
				fSize = int64(sz)
			}
			var fps float64
			if fpsVal, ok := fMap["fps"].(float64); ok {
				fps = fpsVal
			}

			if resStr != "" || vcodec != "none" || acodec != "none" {
				res.Formats = append(res.Formats, db.FormatSpec{
					FormatID:   fID,
					Extension:  ext,
					Resolution: resStr,
					Note:       note,
					Filesize:   fSize,
					VCodec:     vcodec,
					ACodec:     acodec,
					FPS:        fps,
				})
			}
		}
	}

	return res, nil
}

// fetchChannelTabEntries fetches video items from a specific channel tab (/videos or /shorts)
func fetchChannelTabEntries(ctx context.Context, tabURL string, maxItems int, category string, channelTitle string, channelURL string) ([]InspectVideoItem, map[string]interface{}, error) {
	cfg := config.GlobalConfig
	if len(cfg.YtDlpCmd) == 0 {
		return nil, nil, fmt.Errorf("yt-dlp command not configured")
	}

	var cookieArgs []string
	if db.GlobalDB != nil {
		if prefs, _ := db.GlobalDB.GetPreferences(); prefs != nil {
			cookieArgs = BuildCookieArgs(prefs)
		}
	}

	args := cfg.BuildYtDlpArgs("--dump-single-json", "--flat-playlist", "--no-warnings")
	if len(cookieArgs) > 0 {
		args = append(args, cookieArgs...)
	}
	if maxItems > 0 {
		args = append(args, "--playlist-end", fmt.Sprintf("%d", maxItems))
	}
	args = append(args, tabURL)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	sysutil.HideWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if len(cookieArgs) > 0 {
			argsNoCookies := cfg.BuildYtDlpArgs("--dump-single-json", "--flat-playlist", "--no-warnings")
			if maxItems > 0 {
				argsNoCookies = append(argsNoCookies, "--playlist-end", fmt.Sprintf("%d", maxItems))
			}
			argsNoCookies = append(argsNoCookies, tabURL)
			cmdRetry := exec.CommandContext(ctx, argsNoCookies[0], argsNoCookies[1:]...)
			sysutil.HideWindow(cmdRetry)
			var stdout2, stderr2 bytes.Buffer
			cmdRetry.Stdout = &stdout2
			cmdRetry.Stderr = &stderr2
			if err2 := cmdRetry.Run(); err2 == nil {
				stdout = stdout2
				stderr = stderr2
				goto parseEntries
			}
		}
		return nil, nil, fmt.Errorf("failed to fetch tab %s: %w (stderr: %s)", tabURL, err, stderr.String())
	}

parseEntries:
	var raw map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, nil, err
	}

	var items []InspectVideoItem
	entries, _ := raw["entries"].([]interface{})
	for i, entry := range entries {
		eMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		vID, _ := eMap["id"].(string)
		vTitle, _ := eMap["title"].(string)
		vURL, _ := eMap["url"].(string)
		if vURL == "" && vID != "" {
			if category == "Shorts" {
				vURL = "https://www.youtube.com/shorts/" + vID
			} else {
				vURL = "https://www.youtube.com/watch?v=" + vID
			}
		}
		var vDuration int64
		if durVal, ok := eMap["duration"].(float64); ok {
			vDuration = int64(durVal)
		}
		var vThumb string
		if thumbs, ok := eMap["thumbnails"].([]interface{}); ok && len(thumbs) > 0 {
			if lastThumb, ok := thumbs[len(thumbs)-1].(map[string]interface{}); ok {
				vThumb, _ = lastThumb["url"].(string)
			}
		}
		if vThumb == "" {
			vThumb, _ = eMap["thumbnail"].(string)
		}

		itemCat := category
		if itemCat == "" {
			itemCat = "Videos"
		}

		items = append(items, InspectVideoItem{
			ID:         vID,
			URL:        vURL,
			Title:      vTitle,
			Channel:    channelTitle,
			ChannelURL: channelURL,
			Duration:   vDuration,
			Thumbnail:  vThumb,
			Index:      i + 1,
			Category:   itemCat,
		})
	}

	return items, raw, nil
}

// InspectChannelCatalog fetches the video catalog for a YouTube channel URL
func InspectChannelCatalog(ctx context.Context, channelURL string, maxItems int) (*ChannelCatalogResult, error) {
	channelURL = NormalizeYouTubeURL(channelURL)
	cfg := config.GlobalConfig
	if len(cfg.YtDlpCmd) == 0 {
		err := fmt.Errorf("yt-dlp command not configured")
		logger.Errorf("[ChannelCatalog] %v", err)
		return nil, err
	}

	logger.Infof("[ChannelCatalog] Inspecting channel catalog for %s (maxItems: %d)", channelURL, maxItems)

	// Clean base channel URL
	baseURL := strings.TrimRight(channelURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/videos")
	baseURL = strings.TrimSuffix(baseURL, "/shorts")
	baseURL = strings.TrimSuffix(baseURL, "/featured")
	baseURL = strings.TrimSuffix(baseURL, "/streams")

	// 1. Fetch authentic channel metadata directly for real TotalVideos and subscribers
	meta := FetchChannelMetadata(ctx, baseURL, "", "")

	result := &ChannelCatalogResult{
		URL: baseURL,
	}
	if meta != nil {
		result.ChannelID = meta.ChannelID
		result.Title = meta.Title
		result.Handle = strings.TrimPrefix(meta.Handle, "@")
		result.AvatarURL = meta.AvatarURL
		result.SubscriberCount = meta.SubscriberCount
		result.TotalVideos = meta.TotalVideos
	}

	// 2. Fetch Videos, Shorts, and Live Streams tabs concurrently
	var (
		wg                                    sync.WaitGroup
		videoItems, shortsItems, streamsItems []InspectVideoItem
		rawVideos                             map[string]interface{}
		errVideos                             error
	)

	wg.Add(3)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[ChannelCatalog] Panic recovered in videos tab inspection: %v", r)
			}
		}()
		videoItems, rawVideos, errVideos = fetchChannelTabEntries(ctx, baseURL+"/videos", maxItems, "Videos", result.Title, baseURL)
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[ChannelCatalog] Panic recovered in shorts tab inspection: %v", r)
			}
		}()
		shortsItems, _, _ = fetchChannelTabEntries(ctx, baseURL+"/shorts", maxItems, "Shorts", result.Title, baseURL)
	}()

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[ChannelCatalog] Panic recovered in streams tab inspection: %v", r)
			}
		}()
		streamsItems, _, _ = fetchChannelTabEntries(ctx, baseURL+"/streams", maxItems, "Live Streams", result.Title, baseURL)
	}()

	wg.Wait()

	// If rawVideos has extra channel info and result is empty, fill from rawVideos
	if rawVideos != nil {
		if result.ChannelID == "" {
			if cid, ok := rawVideos["channel_id"].(string); ok {
				result.ChannelID = cid
			} else if cid, ok := rawVideos["id"].(string); ok {
				result.ChannelID = cid
			}
		}
		if result.Title == "" || result.Title == "Unknown Channel" {
			if uploader, ok := rawVideos["uploader"].(string); ok && uploader != "" {
				result.Title = uploader
			} else if ch, ok := rawVideos["channel"].(string); ok && ch != "" {
				result.Title = ch
			}
		}
		if result.AvatarURL == "" {
			if thumbs, ok := rawVideos["thumbnails"].([]interface{}); ok && len(thumbs) > 0 {
				if lastThumb, ok := thumbs[len(thumbs)-1].(map[string]interface{}); ok {
					result.AvatarURL, _ = lastThumb["url"].(string)
				}
			}
		}
		if result.SubscriberCount == 0 {
			if subs, ok := rawVideos["channel_follower_count"].(float64); ok {
				result.SubscriberCount = int64(subs)
			} else if subs, ok := rawVideos["subscriber_count"].(float64); ok {
				result.SubscriberCount = int64(subs)
			}
		}
	}

	// 3. Merge Videos, Shorts & Live Streams deduplicated by ID
	seenIDs := make(map[string]bool)
	var allVideos []InspectVideoItem

	for _, v := range videoItems {
		if v.ID != "" && !seenIDs[v.ID] {
			seenIDs[v.ID] = true
			if result.Title != "" && v.Channel == "" {
				v.Channel = result.Title
			}
			allVideos = append(allVideos, v)
		}
	}

	for _, s := range shortsItems {
		if s.ID != "" && !seenIDs[s.ID] {
			seenIDs[s.ID] = true
			s.Category = "Shorts"
			if result.Title != "" && s.Channel == "" {
				s.Channel = result.Title
			}
			allVideos = append(allVideos, s)
		}
	}

	for _, l := range streamsItems {
		if l.ID != "" && !seenIDs[l.ID] {
			seenIDs[l.ID] = true
			l.Category = "Live Streams"
			if result.Title != "" && l.Channel == "" {
				l.Channel = result.Title
			}
			allVideos = append(allVideos, l)
		}
	}

	result.Videos = allVideos

	// If metadata couldn't determine total videos, fallback to fetched total
	if result.TotalVideos == 0 {
		if pc, ok := rawVideos["playlist_count"].(float64); ok && pc > 0 {
			result.TotalVideos = int(pc)
		} else {
			result.TotalVideos = len(allVideos)
		}
	}

	if result.Title == "" {
		result.Title = "YouTube Channel"
	}

	if errVideos != nil && len(allVideos) == 0 {
		logger.LogFailure("ChannelCatalog", "", "", channelURL, errVideos, "")
		return nil, errVideos
	}

	return result, nil
}

// ExtractVideoID parses YouTube video ID from various URL structures
func ExtractVideoID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if u.Host == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/")
	}

	if strings.Contains(u.Host, "youtube.com") {
		if u.Path == "/watch" {
			return u.Query().Get("v")
		}
		if strings.HasPrefix(u.Path, "/shorts/") {
			return strings.TrimPrefix(u.Path, "/shorts/")
		}
		if strings.HasPrefix(u.Path, "/live/") {
			return strings.TrimPrefix(u.Path, "/live/")
		}
		if strings.HasPrefix(u.Path, "/embed/") {
			return strings.TrimPrefix(u.Path, "/embed/")
		}
		if strings.HasPrefix(u.Path, "/v/") {
			return strings.TrimPrefix(u.Path, "/v/")
		}
	}

	return ""
}
