package extractor

import (
	"regexp"
	"strings"
)

// ExtractChannelData parses a rendered channel HTML and extracts all data
func ExtractChannelData(content string) (*ChannelData, error) {
	data := &ChannelData{}

	// Title from <title> tag
	title := extractBetween(content, "<title>", "</title>")
	data.Title = strings.TrimSuffix(decodeHTMLEntities(title), " - Channel Archive")

	// Handle (e.g., @channelname)
	data.Handle = extractByRegex(content, `class="channel-handle"[^>]*>([^<]+)`)
	if data.Handle == "" {
		data.Handle = extractByRegex(content, `(@[\w.-]+)`)
	}

	// Avatar filename
	re := regexp.MustCompile(`class="(?:channel-)?avatar[^"]*"[^>]*src="([^"]+)"`)
	if m := re.FindStringSubmatch(content); len(m) >= 2 && !strings.HasPrefix(m[1], "data:") {
		data.AvatarFilename = m[1]
	}
	if data.AvatarFilename == "" && strings.Contains(content, "channel_avatar.jpg") {
		data.AvatarFilename = "channel_avatar.jpg"
	}

	// Banner filename
	re2 := regexp.MustCompile(`(?:channel[_-]?banner|banner-img)[^>]*src="([^"]+)"`)
	if m := re2.FindStringSubmatch(content); len(m) >= 2 && !strings.HasPrefix(m[1], "data:") {
		data.BannerFilename = m[1]
	}
	if data.BannerFilename == "" {
		re3 := regexp.MustCompile(`background-image:\s*url\(['"]?([^'")\s]+banner[^'")\s]*)['"]?\)`)
		if m := re3.FindStringSubmatch(content); len(m) >= 2 {
			data.BannerFilename = m[1]
		}
	}
	if data.BannerFilename == "" && strings.Contains(content, "channel_banner.jpg") {
		data.BannerFilename = "channel_banner.jpg"
	}

	// Subscriber count
	data.FormattedSubscribers = extractByRegex(content, `([\d,.]+[KMB]?\s*subscribers)`)

	// Total views
	data.FormattedTotalViews = extractByRegex(content, `([\d,.]+[KMB]?\s*(?:total\s*)?views)`)

	// Total videos text
	data.TotalVideosText = extractByRegex(content, `([\d,]+\s*videos?)`)

	// Joined date
	data.JoinedDate = extractByRegex(content, `Joined\s*:?\s*([^<]+)`)

	// Country
	data.Country = extractByRegex(content, `Country\s*:?\s*([^<]+)`)

	// Description
	data.Description = extractChannelDescription(content)

	// Verified badge
	data.IsVerified = strings.Contains(content, "verified")

	// URL
	data.URL = extractByRegex(content, `href="(https?://(?:www\.)?youtube\.com/(?:channel|c|@)[^"]*)"`)
	data.CanonicalURL = data.URL

	// Extract video items
	data.RawVideoData = extractChannelVideos(content)

	return data, nil
}

func extractChannelDescription(content string) string {
	// Look for description in a dedicated section
	re := regexp.MustCompile(`class="(?:channel-)?description[^"]*"[^>]*>([\s\S]*?)</(?:div|p|section)>`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		desc := strings.TrimSpace(m[1])
		// Strip HTML tags
		tagRe := regexp.MustCompile(`<[^>]+>`)
		desc = tagRe.ReplaceAllString(desc, "")
		return decodeHTMLEntities(strings.TrimSpace(desc))
	}
	return ""
}

func extractChannelVideos(content string) []ChannelVideoItem {
	var videos []ChannelVideoItem

	// Find all video links with thumbnails
	linkRe := regexp.MustCompile(`<a[^>]*href="([^"]*index\.html[^"]*)"[^>]*>[\s\S]*?</a>`)
	matches := linkRe.FindAllStringSubmatch(content, -1)

	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		url := m[0]
		relURL := m[1]
		if seen[relURL] {
			continue
		}
		seen[relURL] = true

		// Extract video title from the card
		titleRe := regexp.MustCompile(`class="[^"]*title[^"]*"[^>]*>([^<]+)`)
		titleM := titleRe.FindStringSubmatch(url)
		title := ""
		if len(titleM) >= 2 {
			title = decodeHTMLEntities(strings.TrimSpace(titleM[1]))
		}

		// Extract thumbnail
		thumbRe := regexp.MustCompile(`<img[^>]*src="([^"]+)"`)
		thumbM := thumbRe.FindStringSubmatch(url)
		thumb := ""
		if len(thumbM) >= 2 && !strings.HasPrefix(thumbM[1], "data:") {
			thumb = thumbM[1]
		}

		// Extract duration badge
		durRe := regexp.MustCompile(`class="[^"]*duration[^"]*"[^>]*>([^<]+)`)
		durM := durRe.FindStringSubmatch(url)
		dur := ""
		if len(durM) >= 2 {
			dur = strings.TrimSpace(durM[1])
		}

		if title != "" {
			videos = append(videos, ChannelVideoItem{
				Title:             title,
				RelativeURL:       relURL,
				ThumbnailFilename: thumb,
				FormattedDuration: dur,
			})
		}
	}

	return videos
}
