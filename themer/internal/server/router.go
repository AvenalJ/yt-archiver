package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"themer/internal/detector"
	"themer/internal/scanner"
	"themer/internal/templates"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	mux *http.ServeMux
}

func NewServer() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("POST /api/scan", s.handleScan)
	s.mux.HandleFunc("GET /api/templates", s.handleListTemplates)
	s.mux.HandleFunc("POST /api/apply", s.handleApply)
	s.mux.HandleFunc("POST /api/restore", s.handleRestore)

	// Static files
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	s.mux.Handle("GET /", http.FileServer(http.FS(staticSub)))
}

// --- API Types ---

type ScanRequest struct {
	Path string `json:"path"`
}

type ScanResult struct {
	RootPath string            `json:"root_path"`
	Files    []DetectedFileDTO `json:"files"`
	Summary  ScanSummary       `json:"summary"`
}

type DetectedFileDTO struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Filename     string `json:"filename"`
	SizeBytes    int64  `json:"size_bytes"`
	ParentDir    string `json:"parent_dir"`
	PageType     string `json:"page_type"`
	PageLabel    string `json:"page_label"`
	CurrentTheme string `json:"current_theme"`
}

type ScanSummary struct {
	TotalFiles   int `json:"total_files"`
	VideoCount   int `json:"video_count"`
	PortalCount  int `json:"portal_count"`
	ChannelCount int `json:"channel_count"`
	UnknownCount int `json:"unknown_count"`
}

type ApplyRequest struct {
	TemplateID string   `json:"template_id"`
	Files      []string `json:"files"` // file paths to apply template to
}

type ApplyResult struct {
	Success bool          `json:"success"`
	Applied int           `json:"applied"`
	Failed  int           `json:"failed"`
	Details []ApplyDetail `json:"details"`
}

type ApplyDetail struct {
	Path    string `json:"path"`
	Status  string `json:"status"` // "success" or "error"
	Message string `json:"message,omitempty"`
}

type RestoreRequest struct {
	Files []string `json:"files"` // .html file paths whose .bak should be restored
}

// --- Handlers ---

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		jsonError(w, "Path is required", http.StatusBadRequest)
		return
	}

	// Normalize path
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		jsonError(w, "Invalid path: "+err.Error(), http.StatusBadRequest)
		return
	}

	scanned, err := scanner.ScanDirectory(absPath)
	if err != nil {
		jsonError(w, "Failed to scan directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var files []DetectedFileDTO
	summary := ScanSummary{TotalFiles: 0}

	for _, sf := range scanned {
		nativeFilePath := filepath.FromSlash(sf.Path)
		// Read the HTML content
		content, err := os.ReadFile(nativeFilePath)
		if err != nil {
			continue
		}

		htmlStr := string(content)
		pageType := detector.DetectPageType(htmlStr)
		pageLabel := detector.ExtractPageLabel(htmlStr, pageType)

		if pageType == detector.TypeUnknown {
			summary.UnknownCount++
			continue // Skip unknown files
		}

		// Detect current theme from sidecar if available
		currentTheme := "default"
		sidecarPath := strings.TrimSuffix(nativeFilePath, filepath.Ext(nativeFilePath)) + ".themer.json"
		if sidecarData, err := os.ReadFile(sidecarPath); err == nil {
			var meta map[string]string
			if err := json.Unmarshal(sidecarData, &meta); err == nil && meta["template"] != "" {
				currentTheme = meta["template"]
			}
		}

		dto := DetectedFileDTO{
			Path:         sf.Path,
			RelativePath: sf.RelativePath,
			Filename:     sf.Filename,
			SizeBytes:    sf.SizeBytes,
			ParentDir:    sf.ParentDir,
			PageType:     string(pageType),
			PageLabel:    pageLabel,
			CurrentTheme: currentTheme,
		}
		files = append(files, dto)
		summary.TotalFiles++

		switch pageType {
		case detector.TypeVideoPlayer:
			summary.VideoCount++
		case detector.TypePortal:
			summary.PortalCount++
		case detector.TypeChannel:
			summary.ChannelCount++
		}
	}

	result := ScanResult{
		RootPath: filepath.ToSlash(absPath),
		Files:    files,
		Summary:  summary,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	all := templates.ListAllTemplates()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(all)
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	var req ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !templates.IsValidTemplateID(req.TemplateID) {
		jsonError(w, "Invalid template ID: "+req.TemplateID, http.StatusBadRequest)
		return
	}

	if len(req.Files) == 0 {
		jsonError(w, "No files specified", http.StatusBadRequest)
		return
	}

	result := ApplyResult{Success: true, Details: []ApplyDetail{}}

	for _, filePath := range req.Files {
		detail := applyTemplateToFile(filePath, req.TemplateID)
		result.Details = append(result.Details, detail)
		if detail.Status == "success" {
			result.Applied++
		} else {
			result.Failed++
		}
	}

	if result.Failed > 0 {
		result.Success = false
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	restored := 0
	failed := 0
	var details []ApplyDetail

	for _, filePath := range req.Files {
		nativePath := filepath.FromSlash(filePath)
		bakPath := nativePath + ".bak"

		if _, err := os.Stat(bakPath); err != nil {
			details = append(details, ApplyDetail{
				Path:    filePath,
				Status:  "error",
				Message: "No backup found",
			})
			failed++
			continue
		}

		bakContent, err := os.ReadFile(bakPath)
		if err != nil {
			details = append(details, ApplyDetail{
				Path:    filePath,
				Status:  "error",
				Message: "Failed to read backup: " + err.Error(),
			})
			failed++
			continue
		}

		if err := os.WriteFile(nativePath, bakContent, 0644); err != nil {
			details = append(details, ApplyDetail{
				Path:    filePath,
				Status:  "error",
				Message: "Failed to restore: " + err.Error(),
			})
			failed++
			continue
		}

		// Remove sidecar metadata on restore
		_ = os.Remove(strings.TrimSuffix(nativePath, filepath.Ext(nativePath)) + ".themer.json")

		details = append(details, ApplyDetail{
			Path:   filePath,
			Status: "success",
		})
		restored++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  failed == 0,
		"restored": restored,
		"failed":   failed,
		"details":  details,
	})
}

// --- Core Apply Logic (CSS Replacement) ---

func applyTemplateToFile(filePath, templateID string) ApplyDetail {
	nativePath := filepath.FromSlash(filePath)

	// 1. Read existing HTML
	content, err := os.ReadFile(nativePath)
	if err != nil {
		return ApplyDetail{Path: filePath, Status: "error", Message: "Failed to read file: " + err.Error()}
	}
	htmlStr := string(content)

	// 2. Detect page type
	pageType := detector.DetectPageType(htmlStr)
	if pageType == detector.TypeUnknown {
		return ApplyDetail{Path: filePath, Status: "error", Message: "Cannot detect page type"}
	}

	// 3. Create backup (always from the original, never overwrite existing backup)
	bakPath := nativePath + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		if err := os.WriteFile(bakPath, content, 0644); err != nil {
			return ApplyDetail{Path: filePath, Status: "error", Message: "Failed to create backup: " + err.Error()}
		}
	}

	// 4. Apply CSS replacement — swap the <style> block with the themed CSS
	category := string(pageType) // "video", "portal", or "channel"
	newHTML, err := templates.ApplyThemeCSS(htmlStr, category, templateID)
	if err != nil {
		return ApplyDetail{Path: filePath, Status: "error", Message: "CSS swap failed: " + err.Error()}
	}

	// 5. Write the new HTML
	if err := os.WriteFile(nativePath, []byte(newHTML), 0644); err != nil {
		return ApplyDetail{Path: filePath, Status: "error", Message: "Failed to write file: " + err.Error()}
	}

	// 6. Write a sidecar metadata file
	writeSidecar(nativePath, templateID)

	return ApplyDetail{Path: filePath, Status: "success", Message: fmt.Sprintf("Applied '%s' theme", templateID)}
}

func writeSidecar(htmlPath, templateID string) {
	sidecarPath := strings.TrimSuffix(htmlPath, filepath.Ext(htmlPath)) + ".themer.json"
	meta := map[string]string{
		"template":   templateID,
		"applied_at": time.Now().Format("2006-01-02 15:04:05"),
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(sidecarPath, data, 0644)
}

func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
