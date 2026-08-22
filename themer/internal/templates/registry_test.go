package templates

import (
	"strings"
	"testing"
)

func TestAllThemesExist(t *testing.T) {
	categories := []string{"video", "portal", "channel"}
	themes := []string{"default", "netflix", "professional", "hacker", "glassmorphism"}

	for _, cat := range categories {
		for _, theme := range themes {
			css, err := GetThemeCSS(cat, theme)
			if err != nil {
				t.Fatalf("Failed to load CSS for %s/%s: %v", cat, theme, err)
			}
			if len(css) < 500 {
				t.Fatalf("CSS for %s/%s is suspiciously short (%d bytes)", cat, theme, len(css))
			}
			if !strings.Contains(css, ":root") {
				t.Fatalf("CSS for %s/%s missing :root variables", cat, theme)
			}
		}
	}
}

func TestVideoThemeClassCoverage(t *testing.T) {
	themes := []string{"default", "netflix", "professional", "hacker", "glassmorphism"}

	// Essential classes required by video player JS and DOM
	requiredClasses := []string{
		".comment-thread",
		".comment-avatar",
		".comment-body",
		".comment-author-bar",
		".comment-author-name",
		".comment-time",
		".comment-text",
		".comment-actions",
		".comment-like-box",
		".replies-toggle",
		".replies-list",
		".load-more-btn",
		".settings-menu",
		".settings-menu.open",
		".speed-option",
		".speed-option.active",
		".container.theater-mode",
		".chapter-row",
		".chapter-row.active",
		".desc-text.collapsed",
		".timeline-track",
		".timeline-progress",
		".timeline-thumb",
		".skip-sponsor-pill",
		".custom-captions-overlay",
		".player-yt-badge",
		".sb-toast",
		".live-chat-card",
	}

	for _, theme := range themes {
		css, err := GetThemeCSS("video", theme)
		if err != nil {
			t.Fatalf("GetThemeCSS(video, %s) error: %v", theme, err)
		}

		for _, cls := range requiredClasses {
			if !strings.Contains(css, cls) {
				t.Errorf("Theme %s missing required class: %s", theme, cls)
			}
		}
	}
}

func TestApplyThemeCSSRoundTrip(t *testing.T) {
	sampleHTML := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Test Video</title>
	<style>
		:root { --bg-primary: #0f0f0f; --accent-red: #ff0033; }
		body { background: var(--bg-primary); }
		.comment-thread { display: flex; }
	</style>
</head>
<body>
	<div id="player-wrapper"></div>
	<div id="comments-list"></div>
	<script>console.log("intact");</script>
</body>
</html>`

	// Step 1: Apply Netflix
	netflixHTML, err := ApplyThemeCSS(sampleHTML, "video", "netflix")
	if err != nil {
		t.Fatalf("ApplyThemeCSS(netflix) failed: %v", err)
	}
	if !strings.Contains(netflixHTML, "#E50914") {
		t.Errorf("Expected Netflix red (#E50914) in netflixHTML")
	}
	if !strings.Contains(netflixHTML, `<div id="comments-list"></div>`) {
		t.Errorf("HTML body was mutated or lost")
	}
	if !strings.Contains(netflixHTML, `<script>console.log("intact");</script>`) {
		t.Errorf("JavaScript was mutated or lost")
	}

	// Step 2: Apply Default back to the Netflix-themed HTML
	defaultHTML, err := ApplyThemeCSS(netflixHTML, "video", "default")
	if err != nil {
		t.Fatalf("ApplyThemeCSS(default) failed: %v", err)
	}
	if !strings.Contains(defaultHTML, "#ff0033") {
		t.Errorf("Expected YouTube red (#ff0033) in defaultHTML after re-applying default")
	}
	if strings.Contains(defaultHTML, "#E50914") {
		t.Errorf("Netflix red should be replaced when default theme is applied")
	}

	// Step 3: Apply Hacker
	hackerHTML, err := ApplyThemeCSS(defaultHTML, "video", "hacker")
	if err != nil {
		t.Fatalf("ApplyThemeCSS(hacker) failed: %v", err)
	}
	if !strings.Contains(hackerHTML, "#00FF41") {
		t.Errorf("Expected Hacker green (#00FF41) in hackerHTML")
	}

	// Step 4: Apply Glassmorphism
	glassHTML, err := ApplyThemeCSS(hackerHTML, "video", "glassmorphism")
	if err != nil {
		t.Fatalf("ApplyThemeCSS(glassmorphism) failed: %v", err)
	}
	if !strings.Contains(glassHTML, "#8b5cf6") {
		t.Errorf("Expected Glassmorphism purple (#8b5cf6) in glassHTML")
	}

	// Step 5: Apply Default once again
	defaultAgainHTML, err := ApplyThemeCSS(glassHTML, "video", "default")
	if err != nil {
		t.Fatalf("ApplyThemeCSS(default) second time failed: %v", err)
	}
	if !strings.Contains(defaultAgainHTML, "#ff0033") {
		t.Errorf("Expected YouTube red (#ff0033) after applying default from glassmorphism")
	}
}
