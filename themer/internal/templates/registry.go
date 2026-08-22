package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed video/*.css
var videoCSS embed.FS

//go:embed portal/*.css
var portalCSS embed.FS

//go:embed channel/*.css
var channelCSS embed.FS

// TemplateInfo describes an available template style
type TemplateInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Colors      []string `json:"colors"` // Preview color swatches (hex)
	Category    string   `json:"category"`
}

var templateMeta = []struct {
	ID          string
	Name        string
	Description string
	Colors      []string
}{
	{
		ID:          "default",
		Name:        "Default (YouTube)",
		Description: "The original YouTube-inspired dark/light theme with red accent and familiar layout",
		Colors:      []string{"#0f0f0f", "#ff0033", "#f1f1f1", "#3ea6ff"},
	},
	{
		ID:          "netflix",
		Name:        "Netflix Cinema",
		Description: "Deep black cinematic theme with Netflix red accents, bold typography and dramatic shadows",
		Colors:      []string{"#141414", "#E50914", "#FFFFFF", "#B81D24"},
	},
	{
		ID:          "professional",
		Name:        "Professional",
		Description: "Clean corporate design with blue accents, Inter font, structured sidebar layout and formal aesthetics",
		Colors:      []string{"#0f1117", "#3b82f6", "#e4e6f0", "#60a5fa"},
	},
	{
		ID:          "hacker",
		Name:        "Hacker Terminal",
		Description: "Matrix-inspired green-on-black monospace aesthetic with scanline effects and terminal-style rendering",
		Colors:      []string{"#000000", "#00FF41", "#0D0D0D", "#003B00"},
	},
	{
		ID:          "glassmorphism",
		Name:        "Glassmorphism",
		Description: "Modern frosted glass UI with gradient mesh backgrounds, blur effects and vibrant floating cards",
		Colors:      []string{"#0f0f1a", "#8b5cf6", "#f0f0ff", "#ec4899"},
	},
}

// ListTemplates returns all available template styles for a given category
func ListTemplates(category string) []TemplateInfo {
	var results []TemplateInfo
	for _, m := range templateMeta {
		results = append(results, TemplateInfo{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			Colors:      m.Colors,
			Category:    category,
		})
	}
	return results
}

// ListAllTemplates returns templates for all categories
func ListAllTemplates() map[string][]TemplateInfo {
	return map[string][]TemplateInfo{
		"video":   ListTemplates("video"),
		"portal":  ListTemplates("portal"),
		"channel": ListTemplates("channel"),
	}
}

// GetThemeCSS returns the CSS content for a given category and template ID.
func GetThemeCSS(category, templateID string) (string, error) {
	var fsys embed.FS
	switch category {
	case "video":
		fsys = videoCSS
	case "portal":
		fsys = portalCSS
	case "channel":
		fsys = channelCSS
	default:
		return "", fmt.Errorf("unknown category: %s", category)
	}

	filename := category + "/" + templateID + ".css"
	data, err := fs.ReadFile(fsys, filename)
	if err != nil {
		return "", fmt.Errorf("theme CSS %s/%s not found: %w", category, templateID, err)
	}
	cssStr := strings.TrimPrefix(string(data), "\ufeff")
	return cssStr, nil
}

// styleBlockRegex matches the first <style>...</style> block (greedy, across newlines)
var styleBlockRegex = regexp.MustCompile(`(?s)(<style[^>]*>)(.*?)(</style>)`)

// ApplyThemeCSS replaces the CSS inside the first <style> block in the HTML with
// the themed CSS for the given category and template ID.
func ApplyThemeCSS(htmlContent, category, templateID string) (string, error) {
	newCSS, err := GetThemeCSS(category, templateID)
	if err != nil {
		return "", err
	}

	// Find and replace the first <style> block
	loc := styleBlockRegex.FindStringIndex(htmlContent)
	if loc == nil {
		return "", fmt.Errorf("no <style> block found in HTML")
	}

	match := styleBlockRegex.FindStringSubmatch(htmlContent)
	if len(match) < 4 {
		return "", fmt.Errorf("failed to parse <style> block")
	}

	// Reconstruct: everything before style + new style + everything after
	replacement := match[1] + "\n" + newCSS + "\n\t" + match[3]
	result := htmlContent[:loc[0]] + replacement + htmlContent[loc[1]:]

	return result, nil
}

// TemplateIDs returns valid template IDs
func TemplateIDs() []string {
	ids := make([]string, len(templateMeta))
	for i, m := range templateMeta {
		ids[i] = m.ID
	}
	return ids
}

// IsValidTemplateID checks if a template ID is valid
func IsValidTemplateID(id string) bool {
	for _, m := range templateMeta {
		if strings.EqualFold(m.ID, id) {
			return true
		}
	}
	return false
}
