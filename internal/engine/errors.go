package engine

import (
	"strings"
)

type ErrorCategory string

const (
	ErrCategoryAuthRequired     ErrorCategory = "AUTH_REQUIRED"
	ErrCategoryRateLimited      ErrorCategory = "RATE_LIMITED"
	ErrCategoryGeoRestricted    ErrorCategory = "GEO_RESTRICTED"
	ErrCategoryVideoUnavailable ErrorCategory = "VIDEO_UNAVAILABLE"
	ErrCategoryNetworkError     ErrorCategory = "NETWORK_ERROR"
	ErrCategoryDiskFull         ErrorCategory = "DISK_FULL"
	ErrCategoryCodecError       ErrorCategory = "CODEC_ERROR"
	ErrCategoryUnknown          ErrorCategory = "UNKNOWN"
)

type DiagnosticError struct {
	Category          ErrorCategory `json:"category"`
	Badge             string        `json:"badge"`
	ActionableMessage string        `json:"actionable_message"`
	IsPermanent       bool          `json:"is_permanent"`
	CanRetryAltClient bool          `json:"can_retry_alt_client"`
	RawError          string        `json:"raw_error"`
}

// ClassifyError inspects yt-dlp stderr output or error strings and categorizes them with actionable instructions
func ClassifyError(raw string) *DiagnosticError {
	lower := strings.ToLower(raw)
	res := &DiagnosticError{
		Category:          ErrCategoryUnknown,
		Badge:             "Error",
		ActionableMessage: "Download failed. Check details or retry with alternative player client.",
		IsPermanent:       false,
		CanRetryAltClient: true,
		RawError:          raw,
	}

	switch {
	case strings.Contains(lower, "sign in to confirm you're not a bot"),
		strings.Contains(lower, "bot confirmation"),
		strings.Contains(lower, "sign in to confirm your age"),
		strings.Contains(lower, "this video requires payment"),
		strings.Contains(lower, "members-only"):
		res.Category = ErrCategoryAuthRequired
		res.Badge = "Sign-in Required"
		res.ActionableMessage = "YouTube requires account sign-in or bot challenge. Upload a cookies.txt file in Preferences to proceed."
		res.IsPermanent = true
		res.CanRetryAltClient = false

	case strings.Contains(lower, "429") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "rate-limit") || strings.Contains(lower, "rate limit"):
		res.Category = ErrCategoryRateLimited
		res.Badge = "Rate Limited (429)"
		res.ActionableMessage = "YouTube is rate-limiting requests. Pausing queue to cool down IP."
		res.IsPermanent = false
		res.CanRetryAltClient = true

	case strings.Contains(lower, "not available in your country") || strings.Contains(lower, "geo") || strings.Contains(lower, "blocked in your region") || strings.Contains(lower, "country"):
		res.Category = ErrCategoryGeoRestricted
		res.Badge = "Geo-Restricted"
		res.ActionableMessage = "Video is not available in your region. A VPN or proxy is required."
		res.IsPermanent = true
		res.CanRetryAltClient = false

	case strings.Contains(lower, "private video") || strings.Contains(lower, "video unavailable") || strings.Contains(lower, "this video has been removed") || strings.Contains(lower, "copyright claim") || strings.Contains(lower, "account has been terminated"):
		res.Category = ErrCategoryVideoUnavailable
		res.Badge = "Unavailable / Deleted"
		res.ActionableMessage = "Video has been deleted, set to private, or removed by copyright owner."
		res.IsPermanent = true
		res.CanRetryAltClient = false

	case strings.Contains(lower, "no space left on device") || strings.Contains(lower, "disk full") || strings.Contains(lower, "not enough space") || strings.Contains(lower, "space"):
		res.Category = ErrCategoryDiskFull
		res.Badge = "Disk Full"
		res.ActionableMessage = "Target drive is out of storage space. Free up disk space then resume."
		res.IsPermanent = false
		res.CanRetryAltClient = false

	case strings.Contains(lower, "ffmpeg") || strings.Contains(lower, "merg") || strings.Contains(lower, "conversion failed") || strings.Contains(lower, "mux"):
		res.Category = ErrCategoryCodecError
		res.Badge = "Codec / FFmpeg"
		res.ActionableMessage = "FFmpeg stream muxing error. Run Engine Health Check or change output format to MKV."
		res.IsPermanent = false
		res.CanRetryAltClient = true

	case strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "reset by peer") || strings.Contains(lower, "network") || strings.Contains(lower, "temporary failure in name resolution") || strings.Contains(lower, "broken pipe"):
		res.Category = ErrCategoryNetworkError
		res.Badge = "Network Timeout"
		res.ActionableMessage = "Network connection interrupted. Scheduled for automatic retry with backoff."
		res.IsPermanent = false
		res.CanRetryAltClient = true

	default:
		if strings.Contains(lower, "format") || strings.Contains(lower, "requested format not available") || strings.Contains(lower, "sabr") {
			res.Badge = "Stream Fallback"
			res.ActionableMessage = "Requested format stream unavailable. Retrying with fallback stream extractor."
			res.CanRetryAltClient = true
		}
	}

	return res
}
