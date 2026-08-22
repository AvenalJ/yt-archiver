package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/engine"
	"youtube-downloader/internal/logger"
	"youtube-downloader/internal/queue"

	"github.com/google/uuid"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	db    *db.DB
	queue *queue.QueueManager
	mux   *http.ServeMux
}

func NewServer(database *db.DB, qm *queue.QueueManager) *Server {
	s := &Server{
		db:    database,
		queue: qm,
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("POST /api/info", s.handleInspectInfo)
	s.mux.HandleFunc("POST /api/download", s.handleAddDownload)
	s.mux.HandleFunc("GET /api/downloads", s.handleGetDownloads)
	s.mux.HandleFunc("GET /api/downloads/{id}", s.handleGetDownloadItem)
	s.mux.HandleFunc("POST /api/downloads/{id}/pause", s.handlePauseDownload)
	s.mux.HandleFunc("POST /api/downloads/{id}/resume", s.handleResumeDownload)
	s.mux.HandleFunc("POST /api/downloads/{id}/cancel", s.handleCancelDownload)
	s.mux.HandleFunc("POST /api/downloads/{id}/retry", s.handleRetryDownload)
	s.mux.HandleFunc("POST /api/downloads/{id}/retry-alternative", s.handleRetryAltClient)
	s.mux.HandleFunc("DELETE /api/downloads/{id}", s.handleDeleteDownload)
	s.mux.HandleFunc("POST /api/downloads/{id}/open-folder", s.handleOpenFolder)
	s.mux.HandleFunc("POST /api/downloads/{id}/open-html", s.handleOpenHTML)

	s.mux.HandleFunc("POST /api/queue/pause-all", s.handlePauseAll)
	s.mux.HandleFunc("POST /api/queue/resume-all", s.handleResumeAll)
	s.mux.HandleFunc("POST /api/queue/clear", s.handleClearQueue)
	s.mux.HandleFunc("POST /api/queue/reorder", s.handleReorderQueue)
	s.mux.HandleFunc("POST /api/queue/retry-all-failed", s.handleRetryAllFailed)
	s.mux.HandleFunc("POST /api/queue/fetch-missing", s.handleFetchMissingAssets)
	s.mux.HandleFunc("GET /api/queue/circuit-breaker", s.handleGetCircuitBreaker)
	s.mux.HandleFunc("POST /api/queue/circuit-breaker/reset", s.handleResetCircuitBreaker)

	s.mux.HandleFunc("GET /api/preferences", s.handleGetPreferences)
	s.mux.HandleFunc("POST /api/preferences", s.handleSavePreferences)
	s.mux.HandleFunc("POST /api/preferences/reset", s.handleResetPreferences)
	s.mux.HandleFunc("GET /api/profiles", s.handleGetProfiles)
	s.mux.HandleFunc("POST /api/profiles", s.handleSaveProfiles)
	s.mux.HandleFunc("GET /api/data/export", s.handleExportData)
	s.mux.HandleFunc("POST /api/data/import", s.handleImportData)
	s.mux.HandleFunc("GET /api/engine/health", s.handleEngineHealth)
	s.mux.HandleFunc("POST /api/engine/update", s.handleEngineUpdate)
	s.mux.HandleFunc("GET /api/logs/latest", s.handleGetLatestLog)
	s.mux.HandleFunc("POST /api/logs/open-folder", s.handleOpenLogsFolder)

	// Batch Download & Cookie Upload
	s.mux.HandleFunc("POST /api/download/batch", s.handleBatchDownload)
	s.mux.HandleFunc("POST /api/cookies/upload", s.handleCookieUpload)
	s.mux.HandleFunc("DELETE /api/cookies", s.handleDeleteCookies)

	// Channels & RSS Auto-Archiving
	s.mux.HandleFunc("GET /api/channels", s.handleGetChannels)
	s.mux.HandleFunc("POST /api/channels/add", s.handleAddChannel)
	s.mux.HandleFunc("GET /api/channels/{id}/catalog", s.handleGetChannelCatalog)
	s.mux.HandleFunc("POST /api/channels/{id}/rules", s.handleSaveChannelRules)
	s.mux.HandleFunc("POST /api/channels/{id}/enqueue-selected", s.handleEnqueueSelectedChannelVideos)
	s.mux.HandleFunc("POST /api/channels/{id}/sync", s.handleSyncChannel)
	s.mux.HandleFunc("DELETE /api/channels/{id}", s.handleDeleteChannel)

	s.mux.HandleFunc("GET /api/comments/{videoId}", s.handleGetComments)
	s.mux.HandleFunc("GET /api/events", queue.Broadcaster.ServeHTTP)

	// Favicon SVG handler
	s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="32" height="32"><path fill="#ff0033" d="M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z"/></svg>`))
	})

	// Static assets: serve from disk if local static folder exists (live updates), otherwise fallback to embedded FS
	var staticHttpFS http.FileSystem
	if info, err := os.Stat("internal/server/static"); err == nil && info.IsDir() {
		staticHttpFS = http.Dir("internal/server/static")
	} else if subFS, err := fs.Sub(staticFS, "static"); err == nil {
		staticHttpFS = http.FS(subFS)
	}
	if staticHttpFS != nil {
		fsHandler := http.FileServer(staticHttpFS)
		s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			fsHandler.ServeHTTP(w, r)
		})
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This is a local desktop app. Only permit browser origins served from localhost.
	origin := r.Header.Get("Origin")
	if origin != "" && (strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:")) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

type InspectRequest struct {
	URL string `json:"url"`
}

func (s *Server) handleInspectInfo(w http.ResponseWriter, r *http.Request) {
	var req InspectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, `{"error":"Invalid URL requested"}`, http.StatusBadRequest)
		return
	}

	res, err := engine.InspectURLWithContext(r.Context(), req.URL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Check if already in library
	if len(res.Items) == 1 {
		dup, _ := engine.CheckDuplicate(res.Items[0].ID, req.URL, s.db)
		if dup != nil && dup.IsDuplicate && dup.FileExists {
			res.IsDuplicate = true
			res.DuplicateReason = dup.Reason
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

type AddDownloadRequest struct {
	URL             string                    `json:"url"`
	PlaylistTitle   string                    `json:"playlist_title,omitempty"`
	PlaylistID      string                    `json:"playlist_id,omitempty"`
	ChannelURL      string                    `json:"channel_url,omitempty"`
	SelectedIDs     []string                  `json:"selected_ids,omitempty"` // For playlist partial selection
	Items           []engine.InspectVideoItem `json:"items,omitempty"`        // Client can pass inspected items directly!
	CustomPrefs     *db.UserPreferences       `json:"custom_prefs,omitempty"`
	ForceRedownload bool                      `json:"force_redownload"`
}

type AddDownloadResponse struct {
	QueuedCount  int      `json:"queued_count"`
	SkippedCount int      `json:"skipped_count"`
	ItemIDs      []string `json:"item_ids"`
	Message      string   `json:"message"`
}

func (s *Server) handleAddDownload(w http.ResponseWriter, r *http.Request) {
	var req AddDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.URL == "" && len(req.Items) == 0) {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	var info *engine.InspectResult
	if len(req.Items) > 0 {
		// Re-use items already inspected by the frontend!
		info = &engine.InspectResult{
			Title:      req.PlaylistTitle,
			PlaylistID: req.PlaylistID,
			ChannelURL: req.ChannelURL,
			IsPlaylist: len(req.Items) > 1,
			ItemCount:  len(req.Items),
			Items:      req.Items,
		}
	} else {
		// Fallback to inspecting on backend
		var err error
		info, err = engine.InspectURLWithContext(r.Context(), req.URL)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}

	// Always load saved base global preferences first
	prefs, err := s.db.GetPreferences()
	if err != nil || prefs == nil {
		prefs = db.DefaultPreferences(config.GlobalConfig.DefaultOut)
	}

	// Merge any per-download overrides onto the global preferences
	if req.CustomPrefs != nil {
		if req.CustomPrefs.VideoQuality != "" {
			prefs.VideoQuality = req.CustomPrefs.VideoQuality
		}
		if req.CustomPrefs.VideoFormat != "" {
			prefs.VideoFormat = req.CustomPrefs.VideoFormat
		}
		if req.CustomPrefs.AudioFormat != "" {
			prefs.AudioFormat = req.CustomPrefs.AudioFormat
		}
		if req.CustomPrefs.AudioQuality != "" {
			prefs.AudioQuality = req.CustomPrefs.AudioQuality
		}
		prefs.DownloadVideo = req.CustomPrefs.DownloadVideo
		prefs.DownloadAudioOnly = req.CustomPrefs.DownloadAudioOnly
		prefs.DownloadComments = req.CustomPrefs.DownloadComments
		prefs.CommentLimit = req.CustomPrefs.CommentLimit
		prefs.GenerateHTMLReport = req.CustomPrefs.GenerateHTMLReport
		if req.CustomPrefs.DuplicateAction != "" {
			prefs.DuplicateAction = req.CustomPrefs.DuplicateAction
		}
	}

	resp := AddDownloadResponse{}

	// Filter items to queue
	var itemsToProcess []engine.InspectVideoItem
	if info.IsPlaylist || len(req.SelectedIDs) > 0 {
		selectedSet := make(map[string]bool)
		for _, sid := range req.SelectedIDs {
			selectedSet[sid] = true
		}

		for _, item := range info.Items {
			if len(req.SelectedIDs) == 0 || selectedSet[item.ID] {
				itemsToProcess = append(itemsToProcess, item)
			}
		}
	} else {
		itemsToProcess = info.Items
	}

	// Fast Batch Duplicate Detection
	var duplicateMap map[string]*engine.DuplicateCheckResult
	if !req.ForceRedownload && (prefs.DuplicateAction == "skip" || prefs.DuplicateAction == "") {
		duplicateMap, _ = engine.CheckDuplicatesBatch(itemsToProcess, s.db)
	}

	var itemsToEnqueue []*db.DownloadItem
	totalItems := len(itemsToProcess)
	for i, v := range itemsToProcess {
		key := v.ID
		if key == "" {
			key = v.URL
		}

		if duplicateMap != nil {
			if dup := duplicateMap[key]; dup != nil && dup.IsDuplicate && (dup.FileExists || (dup.ExistingItem != nil && (dup.ExistingItem.Status == "downloading" || dup.ExistingItem.Status == "queued"))) {
				resp.SkippedCount++
				continue
			}
		}

		itemID := uuid.New().String()
		vIndex := v.Index
		if vIndex == 0 {
			vIndex = i + 1
		}

		playlistID := ""
		playlistTitle := ""
		if info.IsPlaylist || info.PlaylistID != "" || (totalItems > 1 && req.PlaylistTitle != "") {
			playlistID = info.PlaylistID
			playlistTitle = req.PlaylistTitle
			if playlistTitle == "" && info.IsPlaylist {
				playlistTitle = info.Title
			}
		}

		dItem := &db.DownloadItem{
			ID:            itemID,
			URL:           v.URL,
			VideoID:       v.ID,
			PlaylistID:    playlistID,
			PlaylistTitle: playlistTitle,
			PlaylistIndex: vIndex,
			PlaylistTotal: totalItems,
			Title:         v.Title,
			Channel:       v.Channel,
			ChannelURL:    info.ChannelURL,
			Duration:      v.Duration,
			ThumbnailURL:  v.Thumbnail,
			Format:        prefs.VideoFormat,
			Quality:       prefs.VideoQuality,
			IsAudioOnly:   prefs.DownloadAudioOnly,
			Status:        "queued",
			CurrentStep:   "Queued in download list",
			CreatedAt:     time.Now(),
		}
		if prefs.DownloadAudioOnly {
			dItem.Format = prefs.AudioFormat
			dItem.Quality = prefs.AudioQuality
		}

		itemsToEnqueue = append(itemsToEnqueue, dItem)
		resp.ItemIDs = append(resp.ItemIDs, itemID)
	}

	if len(itemsToEnqueue) > 0 {
		queuedCount, err := s.queue.EnqueueBatchItems(itemsToEnqueue, prefs)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to enqueue items: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		resp.QueuedCount = queuedCount
	}

	if resp.QueuedCount == 0 && resp.SkippedCount > 0 {
		resp.Message = fmt.Sprintf("Already downloaded in your library! Skipped %d item(s).", resp.SkippedCount)
	} else if resp.SkippedCount > 0 {
		resp.Message = fmt.Sprintf("Successfully queued %d item(s) (%d duplicate skipped)", resp.QueuedCount, resp.SkippedCount)
	} else {
		resp.Message = fmt.Sprintf("Successfully queued %d item(s)", resp.QueuedCount)
	}

	logger.Infof("[API] AddDownload complete: queued %d items (skipped %d duplicates) from %s", resp.QueuedCount, resp.SkippedCount, req.URL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetDownloads(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	w.Header().Set("Content-Type", "application/json")

	if pageStr != "" || limitStr != "" {
		page := 1
		limit := 50
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
		offset := (page - 1) * limit
		items, total, err := s.db.GetDownloadsPaged(status, search, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		totalPages := 1
		if limit > 0 {
			totalPages = (total + limit - 1) / limit
		}
		if totalPages < 1 {
			totalPages = 1
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       items,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		})
		return
	}

	items, err := s.db.GetAllDownloads(status, search)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []*db.DownloadItem{}
	}

	json.NewEncoder(w).Encode(items)
}

func (s *Server) handleGetDownloadItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := s.db.GetDownload(id)
	if err != nil || item == nil {
		http.Error(w, `{"error":"Download not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func (s *Server) handlePauseDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.queue.Pause(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleResumeDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.queue.Resume(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleCancelDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.queue.Cancel(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleRetryDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.queue.Retry(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleRetryAltClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.queue.RetryWithAltClient(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleGetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.queue.GetCircuitBreakerStatus())
}

func (s *Server) handleResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	s.queue.ResetCircuitBreaker()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleDeleteDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deleteFiles := r.URL.Query().Get("delete_files") == "true"
	if err := s.queue.Delete(id, deleteFiles); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handlePauseAll(w http.ResponseWriter, r *http.Request) {
	s.queue.PauseAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleResumeAll(w http.ResponseWriter, r *http.Request) {
	s.queue.ResumeAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleClearQueue(w http.ResponseWriter, r *http.Request) {
	count, err := s.queue.ClearQueue()
	if err != nil {
		logger.Errorf("[API] Failed to clear queue: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   count,
		"message": fmt.Sprintf("Cleared %d items from queue", count),
	})
}

func (s *Server) handleReorderQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		http.Error(w, `{"error":"Invalid request: ids array required"}`, http.StatusBadRequest)
		return
	}

	if err := s.queue.ReorderQueue(req.IDs); err != nil {
		logger.Errorf("[API] Failed to reorder queue: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(req.IDs),
		"message": fmt.Sprintf("Queue reordered successfully (%d items)", len(req.IDs)),
	})
}

func (s *Server) handleRetryAllFailed(w http.ResponseWriter, r *http.Request) {
	count, err := s.queue.RetryAllFailed()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"retried_count": count,
		"message":       fmt.Sprintf("Re-queued %d failed downloads", count),
	})
}

func (s *Server) handleFetchMissingAssets(w http.ResponseWriter, r *http.Request) {
	scanned, queued, err := s.queue.ScanAndQueueMissingAssets()
	if err != nil {
		logger.Errorf("[API] Failed to scan missing assets: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var msg string
	if queued == 0 {
		msg = fmt.Sprintf("Scanned %d completed downloads. All comments and assets are already fully archived!", scanned)
	} else {
		msg = fmt.Sprintf("Scanned %d completed downloads. Queued %d items to fetch missing comments/assets.", scanned, queued)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"scanned": scanned,
		"queued":  queued,
		"message": msg,
	})
}

func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	prefs, err := s.db.GetPreferences()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prefs)
}

func (s *Server) handleSavePreferences(w http.ResponseWriter, r *http.Request) {
	var prefs db.UserPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if cookies.txt exists in data directory
	cookiePath := filepath.Join(config.GlobalConfig.DataDir, "cookies.txt")
	hasCookieFile := false
	if _, err := os.Stat(cookiePath); err == nil {
		hasCookieFile = true
	}

	existing, _ := s.db.GetPreferences()
	if existing != nil && existing.CookieFilePath != "" {
		if _, err := os.Stat(existing.CookieFilePath); err == nil {
			hasCookieFile = true
			cookiePath = existing.CookieFilePath
		}
	}

	if hasCookieFile {
		prefs.CookieFilePath = cookiePath
		// If user didn't explicitly pick a browser, use the uploaded file
		if prefs.CookieSource != "browser" || prefs.CookieBrowser == "none" || prefs.CookieBrowser == "" {
			prefs.CookieSource = "file"
			prefs.CookieBrowser = "none"
		}
	}

	if err := s.db.SavePreferences(&prefs); err != nil {
		logger.Errorf("[API] Failed to save preferences: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Infof("[API] Preferences updated: VideoQuality=%s, VideoFormat=%s, AudioFormat=%s, AudioQuality=%s, MaxConcurrent=%d",
		prefs.VideoQuality, prefs.VideoFormat, prefs.AudioFormat, prefs.AudioQuality, prefs.MaxConcurrentDownloads)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleResetPreferences(w http.ResponseWriter, r *http.Request) {
	def := db.DefaultPreferences("./downloads")
	if err := s.db.SavePreferences(def); err != nil {
		logger.Errorf("[API] Failed to reset preferences: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Infof("[API] Preferences reset to default recommendations")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(def)
}

func (s *Server) handleGetComments(w http.ResponseWriter, r *http.Request) {
	videoID := r.PathValue("videoId")
	comments, err := s.db.GetComments(videoID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if comments == nil {
		comments = []*db.CommentItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

func (s *Server) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := s.db.GetDownload(id)
	if err != nil || item == nil || item.OutputDir == "" {
		http.Error(w, `{"error":"Output folder not found"}`, http.StatusNotFound)
		return
	}

	openInFileManager(item.OutputDir)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleOpenHTML(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := s.db.GetDownload(id)
	if err != nil || item == nil || item.HTMLFilePath == "" {
		http.Error(w, `{"error":"HTML viewer file not found"}`, http.StatusNotFound)
		return
	}

	openInBrowser(item.HTMLFilePath)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func openInFileManager(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	switch runtime.GOOS {
	case "windows":
		exec.Command("explorer", filepath.Clean(path)).Start()
	case "darwin":
		exec.Command("open", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
}

func openInBrowser(target string) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		abs, err := filepath.Abs(target)
		if err == nil {
			target = abs
		}
	}

	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		exec.Command("open", target).Start()
	default:
		exec.Command("xdg-open", target).Start()
	}
}

type BatchDownloadRequest struct {
	URLs        []string            `json:"urls"`
	CustomPrefs *db.UserPreferences `json:"custom_prefs,omitempty"`
}

func (s *Server) handleBatchDownload(w http.ResponseWriter, r *http.Request) {
	var req BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.URLs) == 0 {
		http.Error(w, `{"error":"No URLs provided"}`, http.StatusBadRequest)
		return
	}

	queued, err := s.queue.EnqueueBatch(req.URLs, req.CustomPrefs)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"queued_count": queued,
		"message":      fmt.Sprintf("Successfully queued %d items", queued),
	})
}

func (s *Server) handleCookieUpload(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(2 << 20) // cookies are text; keep sensitive uploads small
	if err != nil {
		http.Error(w, `{"error":"Failed to parse multipart form"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("cookies")
	if err != nil {
		http.Error(w, `{"error":"No file uploaded under key 'cookies'"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.EqualFold(filepath.Ext(header.Filename), ".txt") {
		http.Error(w, `{"error":"Only a cookies.txt file is accepted"}`, http.StatusBadRequest)
		return
	}
	contents, err := io.ReadAll(io.LimitReader(file, 2<<20))
	if err != nil || !validCookieFile(contents) {
		http.Error(w, `{"error":"That file is not a valid Netscape cookies.txt export"}`, http.StatusBadRequest)
		return
	}
	destPath := filepath.Join(config.GlobalConfig.DataDir, "cookies.txt")
	tempPath := destPath + ".uploading"
	out, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		http.Error(w, `{"error":"Failed to save cookies.txt"}`, http.StatusInternalServerError)
		return
	}
	defer out.Close()

	if _, err := out.Write(contents); err != nil {
		_ = out.Close()
		_ = os.Remove(tempPath)
		http.Error(w, `{"error":"Failed to save cookies.txt"}`, http.StatusInternalServerError)
		return
	}
	_ = out.Close()
	if err := os.Rename(tempPath, destPath); err != nil {
		_ = os.Remove(tempPath)
		http.Error(w, `{"error":"Failed to activate cookies.txt"}`, http.StatusInternalServerError)
		return
	}

	// Update preferences to use file
	prefs, _ := s.db.GetPreferences()
	if prefs != nil {
		prefs.CookieSource = "file"
		prefs.CookieFilePath = destPath
		_ = s.db.SavePreferences(prefs)
	}

	logger.Infof("[API] Uploaded cookies.txt (%d bytes) -> %s", len(contents), destPath)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Cookies file saved successfully to " + destPath,
	})
}

func (s *Server) handleDeleteCookies(w http.ResponseWriter, r *http.Request) {
	destPath := filepath.Join(config.GlobalConfig.DataDir, "cookies.txt")
	_ = os.Remove(destPath)

	prefs, _ := s.db.GetPreferences()
	if prefs != nil {
		prefs.CookieSource = "none"
		prefs.CookieFilePath = ""
		_ = s.db.SavePreferences(prefs)
	}

	logger.Infof("[API] Deleted custom cookies.txt")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Cookie file removed",
	})
}

func (s *Server) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.db.GetAllChannels()
	if err != nil {
		http.Error(w, `{"error":"Failed to fetch channels"}`, http.StatusInternalServerError)
		return
	}
	if channels == nil {
		channels = []*db.ChannelSubscription{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

type AddChannelRequest struct {
	URL string `json:"url"`
}

func (s *Server) handleAddChannel(w http.ResponseWriter, r *http.Request) {
	var req AddChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, `{"error":"Channel URL is required"}`, http.StatusBadRequest)
		return
	}

	catalog, err := engine.InspectChannelCatalog(r.Context(), req.URL, 0)
	if err != nil {
		logger.Errorf("[API] Failed to inspect channel for subscription: %s | %v", req.URL, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to inspect channel: " + err.Error()})
		return
	}

	channelID := catalog.ChannelID
	if channelID == "" {
		channelID = uuid.New().String()
	}

	// Check if already subscribed by channel ID, URL, or handle
	existingChannel, _ := s.db.GetChannelByURLOrID(channelID, req.URL)
	if existingChannel == nil && catalog.URL != "" {
		existingChannel, _ = s.db.GetChannelByURLOrID(channelID, catalog.URL)
	}
	if existingChannel == nil && catalog.Handle != "" {
		channels, _ := s.db.GetAllChannels()
		for _, ch := range channels {
			if strings.EqualFold(strings.TrimPrefix(ch.Handle, "@"), strings.TrimPrefix(catalog.Handle, "@")) {
				existingChannel = ch
				break
			}
		}
	}

	if existingChannel != nil {
		// Update existing channel with fresh metadata and return it
		_ = s.db.UpdateChannelFromCatalog(existingChannel.ID, catalog.Title, catalog.Handle, catalog.AvatarURL, catalog.SubscriberCount, catalog.TotalVideos)
		updated, err := s.db.GetChannel(existingChannel.ID)
		if err == nil && updated != nil {
			logger.Infof("[API] Channel already subscribed: %q (Refreshed metadata)", updated.Title)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(updated)
			return
		}
	}

	channel := &db.ChannelSubscription{
		ID:              uuid.New().String(),
		ChannelID:       channelID,
		Title:           catalog.Title,
		Handle:          catalog.Handle,
		URL:             req.URL,
		AvatarURL:       catalog.AvatarURL,
		SubscriberCount: catalog.SubscriberCount,
		AutoDownload:    false,
		TotalVideos:     catalog.TotalVideos,
		CreatedAt:       time.Now(),
	}

	if err := s.db.SaveChannel(channel); err != nil {
		logger.Errorf("[API] Failed to save channel subscription %q: %v", channel.Title, err)
		http.Error(w, `{"error":"Failed to save channel"}`, http.StatusInternalServerError)
		return
	}

	logger.Infof("[API] Subscribed to channel: %q (Handle: @%s, ID: %s)", channel.Title, channel.Handle, channel.ChannelID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channel)
}

func (s *Server) handleGetChannelCatalog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ch, err := s.db.GetChannel(id)
	if err != nil || ch == nil {
		http.Error(w, `{"error":"Channel not found"}`, http.StatusNotFound)
		return
	}

	catalog, err := engine.InspectChannelCatalog(r.Context(), ch.URL, 0)
	if err != nil {
		logger.Errorf("[API] Failed to inspect channel catalog for %s: %v", ch.Title, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to inspect channel: " + err.Error()})
		return
	}

	// Update channel metadata with fresh data
	_ = s.db.UpdateChannelFromCatalog(ch.ID, catalog.Title, catalog.Handle, catalog.AvatarURL, catalog.SubscriberCount, catalog.TotalVideos)

	var videoIDs []string
	for _, v := range catalog.Videos {
		if v.ID != "" {
			videoIDs = append(videoIDs, v.ID)
		}
	}

	existingMap, _ := s.db.FindDuplicatesBatch(videoIDs, nil)

	type CatalogVideoItem struct {
		ID             string `json:"id"`
		URL            string `json:"url"`
		Title          string `json:"title"`
		Duration       int64  `json:"duration"`
		Thumbnail      string `json:"thumbnail"`
		Category       string `json:"category"`
		IsArchived     bool   `json:"is_archived"`
		ArchivedStatus string `json:"archived_status"`
		Index          int    `json:"index"`
	}

	var items []CatalogVideoItem
	archivedCount := 0

	for _, v := range catalog.Videos {
		cat := v.Category
		if cat == "" {
			cat = "Videos"
			if strings.Contains(v.URL, "/shorts/") || (v.Duration > 0 && v.Duration <= 60) {
				cat = "Shorts"
			} else if strings.Contains(v.URL, "/live") {
				cat = "Live Streams"
			}
		}

		isArch := false
		status := ""
		if dup := existingMap[v.ID]; dup != nil {
			isArch = (dup.Status == "completed")
			status = dup.Status
			if isArch {
				archivedCount++
			}
		}

		items = append(items, CatalogVideoItem{
			ID:             v.ID,
			URL:            v.URL,
			Title:          v.Title,
			Duration:       v.Duration,
			Thumbnail:      v.Thumbnail,
			Category:       cat,
			IsArchived:     isArch,
			ArchivedStatus: status,
			Index:          v.Index,
		})
	}

	freshCh, _ := s.db.GetChannel(id)
	if freshCh == nil {
		freshCh = ch
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"channel":        freshCh,
		"total_videos":   catalog.TotalVideos,
		"archived_count": archivedCount,
		"videos":         items,
	})
}

func (s *Server) handleSaveChannelRules(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		AutoDownload       bool  `json:"auto_download"`
		MinDurationSec     int64 `json:"min_duration_sec"`
		ExcludeShorts      bool  `json:"exclude_shorts"`
		ExcludeLiveStreams bool  `json:"exclude_livestreams"`
		MaxAutoSyncCount   int   `json:"max_auto_sync_count"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateChannelRules(id, req.AutoDownload, req.MinDurationSec, req.ExcludeShorts, req.ExcludeLiveStreams, req.MaxAutoSyncCount); err != nil {
		logger.Errorf("[API] Failed to update channel rules for %s: %v", id, err)
		http.Error(w, `{"error":"Failed to save rules"}`, http.StatusInternalServerError)
		return
	}

	freshCh, _ := s.db.GetChannel(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"channel": freshCh,
		"message": "Auto-archive rules updated successfully",
	})
}

func (s *Server) handleEnqueueSelectedChannelVideos(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		VideoIDs []string `json:"video_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.VideoIDs) == 0 {
		http.Error(w, `{"error":"No video IDs provided"}`, http.StatusBadRequest)
		return
	}

	queuedCount, err := s.queue.EnqueueChannelSelectedVideos(r.Context(), id, req.VideoIDs)
	if err != nil {
		logger.Errorf("[API] Failed to enqueue selected channel videos: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"queued_count": queuedCount,
		"message":      fmt.Sprintf("Queued %d selected videos for download", queuedCount),
	})
}

func (s *Server) handleSyncChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infof("[API] Triggering sync for channel ID: %s", id)
	queuedCount, err := s.queue.SyncChannel(r.Context(), id)
	if err != nil {
		logger.Errorf("[API] Channel sync failed for %s: %v", id, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	logger.Infof("[API] Channel sync succeeded for %s: queued %d new items", id, queuedCount)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"queued_count": queuedCount,
		"message":      fmt.Sprintf("Channel sync complete! Queued %d new videos", queuedCount),
	})
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.DeleteChannel(id); err != nil {
		http.Error(w, `{"error":"Failed to delete channel"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
