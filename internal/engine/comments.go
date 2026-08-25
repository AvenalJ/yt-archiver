package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/config"
	"github.com/AvenalJ/yt-archiver/internal/db"
	"github.com/AvenalJ/yt-archiver/internal/logger"
	"github.com/AvenalJ/yt-archiver/internal/sysutil"
)

type CommentExtractionResult struct {
	TotalCount int               `json:"total_count"`
	Comments   []*db.CommentItem `json:"comments"`
	AvatarMap  map[string]string `json:"avatar_map,omitempty"` // avatarHash -> data:image/jpeg;base64,...
}

// FetchComments extracts comments for a video URL up to limit (or all)
func FetchComments(ctx context.Context, videoURL, videoID string, limit int, downloadAvatars bool, outputDir string, onProgress func(step string, count int)) (*CommentExtractionResult, error) {
	cfg := config.GlobalConfig
	if len(cfg.YtDlpCmd) == 0 {
		err := fmt.Errorf("yt-dlp command not configured")
		logger.Errorf("[Comments] %v", err)
		return nil, err
	}

	logger.Infof("[Comments] Fetching comments for %s (limit: %d, avatars: %t)", videoID, limit, downloadAvatars)

	args := cfg.BuildYtDlpArgs("--dump-single-json", "--write-comments", "--skip-download", "--no-warnings")

	// Load cookies if available
	var cookieArgs []string
	if db.GlobalDB != nil {
		if prefs, _ := db.GlobalDB.GetPreferences(); prefs != nil {
			cookieArgs = BuildCookieArgs(prefs)
			if len(cookieArgs) > 0 {
				args = append(args, cookieArgs...)
			}
		}
	}

	// Limit comments if requested
	if limit > 0 {
		args = append(args, "--extractor-args", fmt.Sprintf("youtube:max_comments=%d,all,all,100", limit))
	}

	args = append(args, videoURL)

	if onProgress != nil {
		if limit > 0 {
			onProgress(fmt.Sprintf("Requesting up to %d comments from YouTube...", limit), 0)
		} else {
			onProgress("Requesting comments from YouTube...", 0)
		}
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	sysutil.HideWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		firstErr := err
		firstStderr := stderr.String()

		// If cookies were used or DPAPI error occurred, retry cleanly without cookies
		if len(cookieArgs) > 0 || strings.Contains(firstStderr, "DPAPI") || strings.Contains(strings.ToLower(firstStderr), "cookie") {
			logger.Warnf("[Comments] Cookie extraction failed for %s (%s), immediately retrying anonymously without cookies...", videoID, strings.TrimSpace(firstStderr))
			argsNoCookies := cfg.BuildYtDlpArgs("--dump-single-json", "--write-comments", "--skip-download", "--no-warnings")
			if limit > 0 {
				argsNoCookies = append(argsNoCookies, "--extractor-args", fmt.Sprintf("youtube:max_comments=%d,all,all,100", limit))
			}
			argsNoCookies = append(argsNoCookies, videoURL)

			cmdRetry := exec.CommandContext(ctx, argsNoCookies[0], argsNoCookies[1:]...)
			sysutil.HideWindow(cmdRetry)
			var stdout2, stderr2 bytes.Buffer
			cmdRetry.Stdout = &stdout2
			cmdRetry.Stderr = &stderr2

			if err2 := cmdRetry.Run(); err2 == nil {
				logger.Infof("[Comments] Anonymous fallback succeeded for %s", videoID)
				stdout = stdout2
				stderr = stderr2
				goto parseCommentsJSON
			} else {
				firstErr = err2
				firstStderr = stderr2.String()
			}
		}

		logger.LogFailure("Comments", videoID, "", videoURL, firstErr, firstStderr)
		return nil, fmt.Errorf("failed to fetch comments: %w (stderr: %s)", firstErr, firstStderr)
	}

parseCommentsJSON:
	var raw map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		logger.Errorf("[Comments] Failed to parse JSON for %s: %v", videoID, err)
		return nil, fmt.Errorf("failed to parse yt-dlp comments payload: %w", err)
	}

	rawComments, ok := raw["comments"].([]interface{})
	if !ok || len(rawComments) == 0 {
		logger.Infof("[Comments] No comments found for %s", videoID)
		return &CommentExtractionResult{TotalCount: 0, Comments: []*db.CommentItem{}, AvatarMap: make(map[string]string)}, nil
	}

	totalComments := len(rawComments)
	logger.Infof("[Comments] Extracted %d comments for %s", totalComments, videoID)
	var allItems []*db.CommentItem
	allMap := make(map[string]*db.CommentItem)

	for i, c := range rawComments {
		if onProgress != nil && (i%20 == 0 || i == totalComments-1) {
			onProgress(fmt.Sprintf("Extracting comments (%d/%d)...", i+1, totalComments), i+1)
		}

		cMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		cID, _ := cMap["id"].(string)
		if cID == "" {
			continue
		}
		parentID, _ := cMap["parent"].(string)
		if parentID == "root" {
			parentID = ""
		}
		author, _ := cMap["author"].(string)
		authorID, _ := cMap["author_id"].(string)
		authorURL, _ := cMap["author_url"].(string)
		authorThumb, _ := cMap["author_thumbnail"].(string)
		text, _ := cMap["text"].(string)

		var likeCount int
		if lk, ok := cMap["like_count"].(float64); ok {
			likeCount = int(lk)
		}
		var timestamp int64
		if ts, ok := cMap["timestamp"].(float64); ok {
			timestamp = int64(ts)
		}
		timeText, _ := cMap["_time_text"].(string)
		if timeText == "" && timestamp > 0 {
			timeText = time.Unix(timestamp, 0).Format("Jan 02, 2006")
		}

		var isFav, isCreator, isVer bool
		if fav, ok := cMap["is_favorited"].(bool); ok {
			isFav = fav
		}
		if cr, ok := cMap["is_uploader"].(bool); ok {
			isCreator = cr
		}
		if ver, ok := cMap["author_is_uploader"].(bool); ok {
			isVer = ver
		}

		item := &db.CommentItem{
			ID:              cID,
			VideoID:         videoID,
			ParentID:        parentID,
			Author:          author,
			AuthorID:        authorID,
			AuthorURL:       authorURL,
			AuthorThumbnail: authorThumb,
			Text:            text,
			LikeCount:       likeCount,
			Timestamp:       timestamp,
			TimeText:        timeText,
			IsFavorited:     isFav,
			IsCreator:       isCreator,
			IsVerified:      isVer,
			Replies:         make([]*db.CommentItem, 0),
		}

		allItems = append(allItems, item)
		allMap[cID] = item
	}

	// Download avatars and bundle into avatars.zip + Base64 map if enabled
	avatarMap := make(map[string]string)
	if downloadAvatars {
		avatarMap = downloadAvatarsToZipAndBase64(ctx, allItems, outputDir, onProgress)
	}

	// Group into parent -> replies hierarchy
	var rootComments []*db.CommentItem
	for _, item := range allItems {
		if item.ParentID == "" {
			rootComments = append(rootComments, item)
		} else if parent, exists := allMap[item.ParentID]; exists {
			parent.Replies = append(parent.Replies, item)
			parent.RepliesCount = len(parent.Replies)
		} else {
			rootComments = append(rootComments, item)
		}
	}

	// Save raw comments.json to output directory
	if outputDir != "" && len(rootComments) > 0 {
		commentsPath := filepath.Join(outputDir, "comments.json")
		if cBytes, err := json.MarshalIndent(rootComments, "", "  "); err == nil {
			_ = os.WriteFile(commentsPath, cBytes, 0644)
		}
	}

	return &CommentExtractionResult{
		TotalCount: len(allItems),
		Comments:   rootComments,
		AvatarMap:  avatarMap,
	}, nil
}

// downloadAvatarsToZipAndBase64 downloads commenter avatars concurrently, packs them into a single avatars.zip,
// and returns a Base64 data URI map (avatarHash -> data:image/jpeg;base64,...).
func downloadAvatarsToZipAndBase64(ctx context.Context, comments []*db.CommentItem, outputDir string, onProgress func(step string, count int)) map[string]string {
	avatarMap := make(map[string]string)

	// Deduplicate avatar URLs -> assign hash key to comments
	uniqueAvatars := make(map[string]string) // url -> hash
	for _, c := range comments {
		if c.AuthorThumbnail != "" {
			hashBytes := md5.Sum([]byte(c.AuthorThumbnail))
			hash := hex.EncodeToString(hashBytes[:8])
			uniqueAvatars[c.AuthorThumbnail] = hash
			c.AuthorAvatarLocal = hash
		}
	}

	totalAvatars := len(uniqueAvatars)
	if totalAvatars == 0 {
		return avatarMap
	}

	if onProgress != nil {
		onProgress(fmt.Sprintf("Downloading avatars (0/%d)...", totalAvatars), 0)
	}

	// Concurrency limit for image downloads
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var completedCount int32
	downloadedImages := make(map[string][]byte) // hash -> image bytes
	client := &http.Client{Timeout: 12 * time.Second}

	for avatarURL, hash := range uniqueAvatars {
		wg.Add(1)
		go func(imgURL, h string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// silently recover from avatar worker panic
				}
			}()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

			resp, err := client.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				return
			}
			defer resp.Body.Close()

			imgData, err := io.ReadAll(resp.Body)
			if err != nil || len(imgData) == 0 {
				return
			}

			mu.Lock()
			downloadedImages[h] = imgData
			mu.Unlock()

			done := atomic.AddInt32(&completedCount, 1)
			if onProgress != nil && (done%10 == 0 || int(done) == totalAvatars) {
				onProgress(fmt.Sprintf("Downloading avatars (%d/%d)...", done, totalAvatars), int(done))
			}
		}(avatarURL, hash)
	}

	wg.Wait()

	if onProgress != nil {
		onProgress(fmt.Sprintf("Packaging %d avatars into zip archive...", len(downloadedImages)), len(downloadedImages))
	}

	// 1. Convert to Base64 data URIs for embedding inside index.html
	for h, imgData := range downloadedImages {
		mime := http.DetectContentType(imgData)
		if !strings.HasPrefix(mime, "image/") {
			mime = "image/jpeg"
		}
		b64 := base64.StdEncoding.EncodeToString(imgData)
		avatarMap[h] = fmt.Sprintf("data:%s;base64,%s", mime, b64)
	}

	// 2. Compress all avatar files into a single avatars.zip archive on disk
	if outputDir != "" {
		zipPath := filepath.Join(outputDir, "avatars.zip")
		if err := saveImagesToZip(zipPath, downloadedImages); err == nil {
			// clean up loose avatars folder if one existed from older versions
			_ = os.RemoveAll(filepath.Join(outputDir, "avatars"))
		}
	}

	return avatarMap
}

// saveImagesToZip writes image bytes into a zip archive with Deflate compression
func saveImagesToZip(zipPath string, images map[string][]byte) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for hash, data := range images {
		header := &zip.FileHeader{
			Name:   hash + ".jpg",
			Method: zip.Deflate,
		}
		w, err := zipWriter.CreateHeader(header)
		if err != nil {
			continue
		}
		_, _ = w.Write(data)
	}

	return nil
}

// LoadAvatarMapFromZip reads an avatars.zip archive from disk and reconstructs the Base64 Data URI map
func LoadAvatarMapFromZip(zipPath string) (map[string]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	avatarMap := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		imgBytes, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || len(imgBytes) == 0 {
			continue
		}

		hash := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
		mime := http.DetectContentType(imgBytes)
		if !strings.HasPrefix(mime, "image/") {
			mime = "image/jpeg"
		}
		b64 := base64.StdEncoding.EncodeToString(imgBytes)
		avatarMap[hash] = fmt.Sprintf("data:%s;base64,%s", mime, b64)
	}

	return avatarMap, nil
}
