package engine

import (
	"reflect"
	"testing"

	"youtube-downloader/internal/db"
)

func TestBuildCookieArgs(t *testing.T) {
	tests := []struct {
		name     string
		prefs    *db.UserPreferences
		expected []string
	}{
		{
			name: "Vivaldi browser profile",
			prefs: &db.UserPreferences{
				CookieSource:  "browser",
				CookieBrowser: "vivaldi",
			},
			expected: []string{"--cookies-from-browser", "vivaldi"},
		},
		{
			name: "Chrome browser profile",
			prefs: &db.UserPreferences{
				CookieSource:  "browser",
				CookieBrowser: "chrome",
			},
			expected: []string{"--cookies-from-browser", "chrome"},
		},
		{
			name: "Brave browser profile",
			prefs: &db.UserPreferences{
				CookieSource:  "browser",
				CookieBrowser: "brave",
			},
			expected: []string{"--cookies-from-browser", "brave"},
		},
		{
			name: "Disabled cookies",
			prefs: &db.UserPreferences{
				CookieSource:  "none",
				CookieBrowser: "none",
			},
			expected: nil,
		},
		{
			name: "Legacy unconfigured CookieSource",
			prefs: &db.UserPreferences{
				CookieSource:  "",
				CookieBrowser: "chrome",
			},
			expected: nil,
		},
		{
			name: "Uploaded cookie file with file source",
			prefs: &db.UserPreferences{
				CookieSource:   "file",
				CookieFilePath: "test_cookies.txt",
			},
			expected: nil, // file does not exist on disk
		},
		{
			name: "Nil preferences",
			prefs: nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCookieArgs(tt.prefs)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("BuildCookieArgs() = %v, want %v", got, tt.expected)
			}
		})
	}
}
