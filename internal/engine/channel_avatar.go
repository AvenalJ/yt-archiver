package engine

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var channelAvatarRegex = regexp.MustCompile(`"avatar":\s*\{[^}]*"thumbnails":\s*\[([^\]]+)\]`)
var avatarURLRegex = regexp.MustCompile(`"url":\s*"(https://yt3\.googleusercontent\.com/[^"]+)"`)
var avatarSizeRegex = regexp.MustCompile(`=s\d+`)

// DownloadChannelAvatar attempts to download the channel's avatar image.
// It fetches the YouTube channel page and extracts the avatar URL from the embedded JSON.
// Returns the local filename (e.g. "channel_avatar.jpg") or empty string on failure.
func DownloadChannelAvatar(ctx context.Context, channelURL string, outputDir string) string {
	if channelURL == "" {
		return ""
	}

	destPath := filepath.Join(outputDir, "channel_avatar.jpg")

	// If already downloaded, skip
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		return "channel_avatar.jpg"
	}

	avatarURL := extractChannelAvatarURL(ctx, channelURL)
	if avatarURL == "" {
		return ""
	}

	// Download the avatar image
	if downloadFile(ctx, avatarURL, destPath) {
		return "channel_avatar.jpg"
	}
	return ""
}

func extractChannelAvatarURL(ctx context.Context, channelURL string) string {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", channelURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return ""
	}
	body := string(bodyBytes)

	// Strategy 1: YouTube embeds channel avatar JSON
	// Pattern: "avatar":{"thumbnails":[{"url":"https://yt3.googleusercontent.com/...","width":...}]}
	matches := channelAvatarRegex.FindStringSubmatch(body)
	if len(matches) >= 2 {
		urlMatches := avatarURLRegex.FindAllStringSubmatch(matches[1], -1)
		if len(urlMatches) > 0 {
			bestURL := urlMatches[len(urlMatches)-1][1]
			if avatarSizeRegex.MatchString(bestURL) {
				bestURL = avatarSizeRegex.ReplaceAllString(bestURL, "=s176")
			}
			return bestURL
		}
	}

	// Strategy 2: Check og:image meta tag
	ogImageRegex := regexp.MustCompile(`<meta\s+property="og:image"\s+content="([^"]+)"`)
	ogMatches := ogImageRegex.FindStringSubmatch(body)
	if len(ogMatches) >= 2 && (regexp.MustCompile(`googleusercontent|ytimg`).MatchString(ogMatches[1])) {
		return ogMatches[1]
	}

	// Strategy 3: Find any yt3.googleusercontent.com URL with avatar sizing
	generalAvatarRegex := regexp.MustCompile(`(https://yt3\.googleusercontent\.com/[a-zA-Z0-9_\-=]+)`)
	genMatches := generalAvatarRegex.FindAllStringSubmatch(body, -1)
	for _, gm := range genMatches {
		u := gm[1]
		if avatarSizeRegex.MatchString(u) && !regexp.MustCompile(`=w\d+`).MatchString(u) {
			return avatarSizeRegex.ReplaceAllString(u, "=s176")
		}
	}

	return ""
}

func downloadFile(ctx context.Context, url, destPath string) bool {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	defer resp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return false
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(destPath)
		return false
	}
	return true
}
