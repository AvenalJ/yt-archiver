package engine

import (
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		input            string
		expectedCategory ErrorCategory
		expectedBadge    string
		isPermanent      bool
	}{
		{
			input:            "ERROR: [youtube] 123: Sign in to confirm you're not a bot",
			expectedCategory: ErrCategoryAuthRequired,
			expectedBadge:    "Sign-in Required",
			isPermanent:      true,
		},
		{
			input:            "HTTP Error 429: Too Many Requests",
			expectedCategory: ErrCategoryRateLimited,
			expectedBadge:    "Rate Limited (429)",
			isPermanent:      false,
		},
		{
			input:            "ERROR: [youtube] abc: This video is not available in your country",
			expectedCategory: ErrCategoryGeoRestricted,
			expectedBadge:    "Geo-Restricted",
			isPermanent:      true,
		},
		{
			input:            "ERROR: [youtube] xyz: Private video. Sign in if you've been granted access",
			expectedCategory: ErrCategoryVideoUnavailable,
			expectedBadge:    "Unavailable / Deleted",
			isPermanent:      true,
		},
		{
			input:            "No space left on device: writing to disk failed",
			expectedCategory: ErrCategoryDiskFull,
			expectedBadge:    "Disk Full",
			isPermanent:      false,
		},
		{
			input:            "ffmpeg merger failed with code 1",
			expectedCategory: ErrCategoryCodecError,
			expectedBadge:    "Codec / FFmpeg",
			isPermanent:      false,
		},
		{
			input:            "Connection timed out: socket timeout after 30 seconds",
			expectedCategory: ErrCategoryNetworkError,
			expectedBadge:    "Network Timeout",
			isPermanent:      false,
		},
	}

	for _, tt := range tests {
		diag := ClassifyError(tt.input)
		if diag.Category != tt.expectedCategory {
			t.Errorf("ClassifyError(%q).Category = %v; want %v", tt.input, diag.Category, tt.expectedCategory)
		}
		if diag.Badge != tt.expectedBadge {
			t.Errorf("ClassifyError(%q).Badge = %v; want %v", tt.input, diag.Badge, tt.expectedBadge)
		}
		if diag.IsPermanent != tt.isPermanent {
			t.Errorf("ClassifyError(%q).IsPermanent = %v; want %v", tt.input, diag.IsPermanent, tt.isPermanent)
		}
	}
}
