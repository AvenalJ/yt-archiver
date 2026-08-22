package extractor

import (
	"html/template"
	"regexp"
	"strings"
)

// VideoData holds all data extracted from a video player HTML file
type VideoData struct {
	Title                  string      `json:"title"`
	VideoID                string      `json:"video_id"`
	Channel                string      `json:"channel"`
	ChannelAvatarFilename  string      `json:"channel_avatar_filename"`
	FormattedSubscribers   string      `json:"formatted_subscribers"`
	FormattedDuration      string      `json:"formatted_duration"`
	FormattedViews         string      `json:"formatted_views"`
	FormattedLikes         string      `json:"formatted_likes"`
	FormattedDislikes      string      `json:"formatted_dislikes"`
	UploadDate             string      `json:"upload_date"`
	MediaFilename          string      `json:"media_filename"`
	MediaMimeType          string      `json:"media_mime_type"`
	IsAudioOnly            bool        `json:"is_audio_only"`
	ThumbnailFilename      string      `json:"thumbnail_filename"`
	SubtitlesFilename      string      `json:"subtitles_filename"`
	CompanionAudioFilename string      `json:"companion_audio_filename"`
	StoryboardFilename     string      `json:"storyboard_filename"`
	GeneratedAt            string      `json:"generated_at"`
	SourceURL              string      `json:"source_url"`
	VideoQuality           string      `json:"video_quality"`
	FormattedFilesize      string      `json:"formatted_filesize"`
	CommentsCount          string      `json:"comments_count"`
	HasChapters            bool        `json:"has_chapters"`
	ChapterCount           int         `json:"chapter_count"`
	HasSponsorSegments     bool        `json:"has_sponsor_segments"`
	HasLiveChat            bool        `json:"has_live_chat"`
	LiveChatCount          int         `json:"live_chat_count"`
	// Raw JSON data (preserved exactly as-is from the original HTML)
	CommentsJSON        template.JS `json:"-"`
	AvatarsJSON         template.JS `json:"-"`
	ChaptersJSON        template.JS `json:"-"`
	SponsorSegmentsJSON template.JS `json:"-"`
	StoryboardJSON      template.JS `json:"-"`
	LiveChatJSON        template.JS `json:"-"`
	SubtitlesCuesJSON   template.JS `json:"-"`
	TagsJSON            template.JS `json:"-"`
	DescriptionJSON     template.JS `json:"-"`
}

// PortalData holds all data extracted from a portal/catalog HTML file
type PortalData struct {
	GeneratedAt string      `json:"generated_at"`
	CatalogJSON template.JS `json:"-"`
}

// ChannelData holds all data extracted from a channel HTML file
type ChannelData struct {
	Title                string `json:"title"`
	Handle               string `json:"handle"`
	AvatarFilename       string `json:"avatar_filename"`
	BannerFilename       string `json:"banner_filename"`
	FormattedSubscribers string `json:"formatted_subscribers"`
	FormattedTotalViews  string `json:"formatted_total_views"`
	TotalVideosText      string `json:"total_videos_text"`
	JoinedDate           string `json:"joined_date"`
	Country              string `json:"country"`
	Description          string `json:"description"`
	IsVerified           bool   `json:"is_verified"`
	URL                  string `json:"url"`
	CanonicalURL         string `json:"canonical_url"`

	// Reconstructed from the HTML
	VideosHTML    template.HTML `json:"-"` // The raw video cards HTML
	RawVideoData []ChannelVideoItem `json:"-"`
}

// ChannelVideoItem holds parsed video entries from a channel page
type ChannelVideoItem struct {
	Title             string `json:"title"`
	VideoID           string `json:"video_id"`
	RelativeURL       string `json:"relative_url"`
	ThumbnailFilename string `json:"thumbnail_filename"`
	FormattedDuration string `json:"formatted_duration"`
	FormattedViews    string `json:"formatted_views"`
	UploadDate        string `json:"upload_date"`
}

// extractJSONBlock pulls content from a <script type="application/json" id="ID"> block
func extractJSONBlock(content, id string) string {
	marker := `id="` + id + `"`
	idx := strings.Index(content, marker)
	if idx == -1 {
		return ""
	}

	// Find the closing > of the script tag
	rest := content[idx:]
	tagEnd := strings.Index(rest, ">")
	if tagEnd == -1 {
		return ""
	}
	rest = rest[tagEnd+1:]

	// Find </script>
	scriptEnd := strings.Index(rest, "</script>")
	if scriptEnd == -1 {
		return ""
	}
	return strings.TrimSpace(rest[:scriptEnd])
}

// extractByRegex returns the first capture group match
func extractByRegex(content, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(content)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractAttribute extracts an attribute value from an HTML tag context
func extractAttribute(tag, attr string) string {
	pattern := attr + `="([^"]*?)"`
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(tag)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// extractBetween extracts text between two markers
func extractBetween(content, start, end string) string {
	si := strings.Index(content, start)
	if si == -1 {
		return ""
	}
	si += len(start)
	ei := strings.Index(content[si:], end)
	if ei == -1 {
		return ""
	}
	return strings.TrimSpace(content[si : si+ei])
}

// decodeHTMLEntities handles common HTML entities
func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&bull;", "•")
	return s
}
