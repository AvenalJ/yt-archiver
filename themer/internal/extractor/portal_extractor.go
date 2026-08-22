package extractor

import (
	"html/template"
)

// ExtractPortalData parses a rendered portal/catalog HTML and extracts all data
func ExtractPortalData(content string) (*PortalData, error) {
	data := &PortalData{}

	// Extract catalog JSON from the data-catalog script block
	catalogJSON := extractJSONBlock(content, "data-catalog")
	if catalogJSON == "" {
		catalogJSON = `{"videos":[],"channels":[]}`
	}
	data.CatalogJSON = template.JS(catalogJSON)

	// Extract generated-at timestamp
	data.GeneratedAt = extractByRegex(content, `Generated:\s*([^<"]+)`)
	if data.GeneratedAt == "" {
		data.GeneratedAt = extractByRegex(content, `updated:\s*([^<"]+)`)
	}

	return data, nil
}
