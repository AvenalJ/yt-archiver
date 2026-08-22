package detector

import (
	"strings"
)

// PageType represents the type of YT Archive HTML page
type PageType string

const (
	TypeVideoPlayer PageType = "video"
	TypePortal      PageType = "portal"
	TypeChannel     PageType = "channel"
	TypeUnknown     PageType = "unknown"
)

// DetectedFile enriches a scanned file with its detected type
type DetectedFile struct {
	Path         string   `json:"path"`
	RelativePath string   `json:"relative_path"`
	Filename     string   `json:"filename"`
	SizeBytes    int64    `json:"size_bytes"`
	ParentDir    string   `json:"parent_dir"`
	PageType     PageType `json:"page_type"`
	PageLabel    string   `json:"page_label"` // Human-readable label e.g. "Video: My Video Title"
}

// DetectPageType analyzes HTML content and returns the page type
func DetectPageType(htmlContent string) PageType {
	// Check for portal/catalog signatures first (most specific)
	if isPortalPage(htmlContent) {
		return TypePortal
	}

	// Check for channel page
	if isChannelPage(htmlContent) {
		return TypeChannel
	}

	// Check for video player page
	if isVideoPlayerPage(htmlContent) {
		return TypeVideoPlayer
	}

	return TypeUnknown
}

// ExtractPageLabel pulls a human-readable label from the HTML content
func ExtractPageLabel(htmlContent string, pageType PageType) string {
	title := extractTitle(htmlContent)

	switch pageType {
	case TypeVideoPlayer:
		// Remove " - Offline YouTube Player" suffix
		title = strings.TrimSuffix(title, " - Offline YouTube Player")
		if title == "" {
			title = "Untitled Video"
		}
		return "Video: " + title
	case TypePortal:
		return "Portal: Downloads Catalog"
	case TypeChannel:
		title = strings.TrimSuffix(title, " - Channel Archive")
		if title == "" {
			title = "Unknown Channel"
		}
		return "Channel: " + title
	default:
		return "Unknown HTML"
	}
}

func isPortalPage(content string) bool {
	signatures := []string{
		`id="data-catalog"`,
		`Offline YouTube Archive`,
		`YT Archive - Offline YouTube Archive`,
	}
	for _, sig := range signatures {
		if strings.Contains(content, sig) {
			return true
		}
	}
	return false
}

func isChannelPage(content string) bool {
	signatures := []string{
		`Channel Archive</title>`,
		`- Channel Archive`,
		`channel-banner-section`,
	}
	for _, sig := range signatures {
		if strings.Contains(content, sig) {
			return true
		}
	}
	return false
}

func isVideoPlayerPage(content string) bool {
	signatures := []string{
		`id="main-player"`,
		`id="data-comments"`,
		`Offline YouTube Player`,
		`id="player-wrapper"`,
	}
	matchCount := 0
	for _, sig := range signatures {
		if strings.Contains(content, sig) {
			matchCount++
		}
	}
	return matchCount >= 1
}

func extractTitle(content string) string {
	start := strings.Index(content, "<title>")
	if start == -1 {
		return ""
	}
	start += len("<title>")
	end := strings.Index(content[start:], "</title>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(content[start : start+end])
}
