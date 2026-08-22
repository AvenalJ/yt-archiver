package extractor

import (
	"html/template"
	"regexp"
	"strings"
)

// ExtractVideoData parses a rendered video player HTML and extracts all data needed for re-templating
func ExtractVideoData(content string) (*VideoData, error) {
	data := &VideoData{}

	// 1. Extract all JSON data blocks (these are the most reliable data sources)
	data.CommentsJSON = template.JS(extractJSONBlock(content, "data-comments"))
	data.AvatarsJSON = template.JS(extractJSONBlock(content, "data-avatars"))
	data.ChaptersJSON = template.JS(extractJSONBlock(content, "data-chapters"))
	data.SponsorSegmentsJSON = template.JS(extractJSONBlock(content, "data-sponsor"))
	data.StoryboardJSON = template.JS(extractJSONBlock(content, "data-storyboard"))
	data.LiveChatJSON = template.JS(extractJSONBlock(content, "data-livechat"))
	data.SubtitlesCuesJSON = template.JS(extractJSONBlock(content, "data-subtitles"))
	data.TagsJSON = template.JS(extractJSONBlock(content, "data-tags"))
	data.DescriptionJSON = template.JS(extractJSONBlock(content, "data-description"))

	// Default empty values for JSON blocks
	if data.CommentsJSON == "" {
		data.CommentsJSON = "[]"
	}
	if data.AvatarsJSON == "" {
		data.AvatarsJSON = "{}"
	}
	if data.ChaptersJSON == "" {
		data.ChaptersJSON = "[]"
	}
	if data.SponsorSegmentsJSON == "" {
		data.SponsorSegmentsJSON = "[]"
	}
	if data.StoryboardJSON == "" {
		data.StoryboardJSON = "null"
	}
	if data.LiveChatJSON == "" {
		data.LiveChatJSON = "[]"
	}
	if data.SubtitlesCuesJSON == "" {
		data.SubtitlesCuesJSON = "[]"
	}
	if data.TagsJSON == "" {
		data.TagsJSON = "[]"
	}
	if data.DescriptionJSON == "" {
		data.DescriptionJSON = `""`
	}

	// 2. Extract title
	data.Title = extractTitle(content)

	// 3. Detect if audio-only
	data.IsAudioOnly = strings.Contains(content, "audio-card") && !strings.Contains(content, `id="player-wrapper"`)

	// 4. Extract video ID from localStorage key or source URL
	data.VideoID = extractByRegex(content, `yt_pos_([a-zA-Z0-9_-]{11})`)
	if data.VideoID == "" {
		data.VideoID = extractByRegex(content, `youtube\.com/watch\?v=([a-zA-Z0-9_-]{11})`)
	}

	// 5. Extract channel name from the channel-name span
	data.Channel = extractChannelName(content)

	// 6. Extract media filename from <source> tag
	data.MediaFilename = extractSourceFilename(content)
	data.MediaMimeType = extractSourceMimeType(content)

	// 7. Extract thumbnail from poster attribute or img tag
	data.ThumbnailFilename = extractThumbnail(content)

	// 8. Extract channel avatar filename
	data.ChannelAvatarFilename = extractChannelAvatar(content)

	// 9. Extract formatted values from display elements
	data.FormattedViews = extractFormattedViews(content)
	data.FormattedLikes = extractFormattedLikes(content)
	data.FormattedDislikes = extractFormattedDislikes(content)
	data.FormattedDuration = extractFormattedDuration(content)
	data.FormattedSubscribers = extractFormattedSubscribers(content)
	data.UploadDate = extractUploadDate(content)
	data.CommentsCount = extractCommentsCount(content)
	data.GeneratedAt = extractGeneratedAt(content)
	data.VideoQuality = extractVideoQuality(content)
	data.FormattedFilesize = extractFilesize(content)

	// 10. Extract source URL
	data.SourceURL = extractSourceURL(content)

	// 11. Extract subtitles filename from <track> tag
	data.SubtitlesFilename = extractSubtitlesFilename(content)

	// 12. Extract storyboard filename
	data.StoryboardFilename = extractStoryboardFilename(content)

	// 13. Determine feature flags
	chaptersJSON := string(data.ChaptersJSON)
	data.HasChapters = chaptersJSON != "[]" && chaptersJSON != ""
	sponsorJSON := string(data.SponsorSegmentsJSON)
	data.HasSponsorSegments = sponsorJSON != "[]" && sponsorJSON != ""
	liveChatJSON := string(data.LiveChatJSON)
	data.HasLiveChat = liveChatJSON != "[]" && liveChatJSON != ""

	// Count chapters
	if data.HasChapters {
		data.ChapterCount = strings.Count(chaptersJSON, `"start_time"`)
	}

	// Count live chat
	if data.HasLiveChat {
		data.LiveChatCount = strings.Count(liveChatJSON, `"timestamp"`)
		if data.LiveChatCount == 0 {
			data.LiveChatCount = strings.Count(liveChatJSON, `"author"`)
		}
	}

	return data, nil
}

func extractTitle(content string) string {
	title := extractBetween(content, "<title>", "</title>")
	title = strings.TrimSuffix(title, " - Offline YouTube Player")
	return decodeHTMLEntities(title)
}

func extractChannelName(content string) string {
	// Look for channel-name span content
	re := regexp.MustCompile(`class="channel-name"[^>]*>\s*\n?\s*([^<\n]+)`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		name := strings.TrimSpace(m[1])
		name = decodeHTMLEntities(name)
		if name != "" {
			return name
		}
	}
	// Fallback: Audio card channel text
	re2 := regexp.MustCompile(`Audio Track</span>`)
	if re2.MatchString(content) {
		re3 := regexp.MustCompile(`<span>([^<]+)\s*&bull;\s*Audio Track</span>`)
		m3 := re3.FindStringSubmatch(content)
		if len(m3) >= 2 {
			return decodeHTMLEntities(strings.TrimSpace(m3[1]))
		}
	}
	return ""
}

func extractSourceFilename(content string) string {
	re := regexp.MustCompile(`<source\s+src="([^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractSourceMimeType(content string) string {
	re := regexp.MustCompile(`<source\s+src="[^"]+"\s+type="([^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return "video/mp4"
}

func extractThumbnail(content string) string {
	// From video poster attribute
	re := regexp.MustCompile(`poster="([^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 && !strings.HasPrefix(m[1], "data:") {
		return m[1]
	}
	// From audio card cover art
	re2 := regexp.MustCompile(`class="audio-art-box"[^>]*>.*?<img\s+src="([^"]+)"`)
	m2 := re2.FindStringSubmatch(content)
	if len(m2) >= 2 && !strings.HasPrefix(m2[1], "data:") {
		return m2[1]
	}
	return ""
}

func extractChannelAvatar(content string) string {
	re := regexp.MustCompile(`class="channel-avatar"[^>]*src="([^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 && !strings.HasPrefix(m[1], "data:") {
		return m[1]
	}
	// Also check: src="channel_avatar.jpg"
	if strings.Contains(content, `channel_avatar.jpg`) {
		return "channel_avatar.jpg"
	}
	return ""
}

func extractFormattedViews(content string) string {
	re := regexp.MustCompile(`>\s*([\d,.]+[KMB]?)\s*views\s*<`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return "0"
}

func extractFormattedLikes(content string) string {
	// Likes are in the likes-dislikes-pill, right after the thumbs-up SVG
	re := regexp.MustCompile(`class="likes-dislikes-pill".*?</svg>\s*\n?\s*<span>([^<]+)</span>`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return "0"
}

func extractFormattedDislikes(content string) string {
	// Dislikes SVG is rotated 180deg
	re := regexp.MustCompile(`transform:\s*rotate\(180deg\).*?</svg>\s*\n?\s*<span>([^<]+)</span>`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractFormattedDuration(content string) string {
	// From total-duration span
	re := regexp.MustCompile(`id="total-duration"[^>]*>([^<]+)<`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		dur := strings.TrimSpace(m[1])
		if dur != "0:00" {
			return dur
		}
	}
	// Fallback: Duration spec item
	re2 := regexp.MustCompile(`<strong>Duration:</strong>\s*([^<]+)<`)
	m2 := re2.FindStringSubmatch(content)
	if len(m2) >= 2 {
		return strings.TrimSpace(m2[1])
	}
	return "0:00"
}

func extractFormattedSubscribers(content string) string {
	re := regexp.MustCompile(`class="channel-subs"[^>]*>([^<]*(?:subscribers?|Official Channel)[^<]*)`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		s := strings.TrimSpace(m[1])
		if s != "" {
			return s
		}
	}
	// Alternate: look for subscriber text
	re2 := regexp.MustCompile(`([\d,.]+[KMB]?\s*subscribers)`)
	m2 := re2.FindStringSubmatch(content)
	if len(m2) >= 2 {
		return strings.TrimSpace(m2[1])
	}
	return ""
}

func extractUploadDate(content string) string {
	re := regexp.MustCompile(`class="desc-stats"[^>]*>.*?<span>[^<]*</span>\s*<span>([^<]+)</span>`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractCommentsCount(content string) string {
	re := regexp.MustCompile(`id="comments-count-label"[^>]*>([^<]*)\s*Comments`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return "0"
}

func extractGeneratedAt(content string) string {
	re := regexp.MustCompile(`<strong>Archived:</strong>\s*([^<]+)<`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractVideoQuality(content string) string {
	re := regexp.MustCompile(`<strong>Quality:</strong>\s*([^<]+)<`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractFilesize(content string) string {
	re := regexp.MustCompile(`<strong>Size:</strong>\s*([^<]+)<`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractSourceURL(content string) string {
	re := regexp.MustCompile(`href="(https?://(?:www\.)?youtube\.com/watch\?v=[^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractSubtitlesFilename(content string) string {
	re := regexp.MustCompile(`<track[^>]*src="([^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractStoryboardFilename(content string) string {
	re := regexp.MustCompile(`background-image:\s*url\('([^']*storyboard[^']*)'\)`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
