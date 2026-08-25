package engine

import (
	"os"
	"strings"

	"github.com/AvenalJ/yt-archiver/internal/db"
)

// BuildCookieArgs returns the CLI arguments for yt-dlp based on user preferences.
func BuildCookieArgs(prefs *db.UserPreferences) []string {
	if prefs == nil {
		return nil
	}

	// 1. Uploaded cookie file option (highest reliability, no DPAPI encryption lock issues)
	if prefs.CookieSource == "file" && prefs.CookieFilePath != "" {
		if _, err := os.Stat(prefs.CookieFilePath); err == nil {
			return []string{"--cookies", prefs.CookieFilePath}
		}
	}

	// 2. Browser profile cookies (ONLY if CookieSource is explicitly "browser" and browser is not "none")
	if prefs.CookieSource == "browser" {
		browser := strings.ToLower(strings.TrimSpace(prefs.CookieBrowser))
		if browser != "" && browser != "none" {
			return []string{"--cookies-from-browser", browser}
		}
	}

	// 3. Fallback: If a valid cookie file path is present and source was not explicitly disabled
	if prefs.CookieFilePath != "" && prefs.CookieSource != "none" {
		if _, err := os.Stat(prefs.CookieFilePath); err == nil {
			return []string{"--cookies", prefs.CookieFilePath}
		}
	}

	return nil
}
