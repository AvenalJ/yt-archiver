package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/logger"
	"youtube-downloader/internal/sysutil"
)

type exportArchive struct {
	Version     int                       `json:"version"`
	Preferences *db.UserPreferences       `json:"preferences"`
	Profiles    []db.DownloadProfile      `json:"profiles"`
	Channels    []*db.ChannelSubscription `json:"channels"`
	Downloads   []*db.DownloadItem        `json:"downloads"`
}

func validCookieFile(data []byte) bool {
	text := string(data)
	if strings.Contains(text, "# Netscape HTTP Cookie File") {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Count(line, "\t") >= 6 {
			return true
		}
	}
	return false
}

func (s *Server) handleGetProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.db.GetProfiles()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(profiles)
}

func (s *Server) handleSaveProfiles(w http.ResponseWriter, r *http.Request) {
	var profiles []db.DownloadProfile
	if err := json.NewDecoder(r.Body).Decode(&profiles); err != nil || len(profiles) > 20 {
		http.Error(w, `{"error":"Provide up to 20 valid profiles"}`, 400)
		return
	}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Name) == "" || profile.Preferences == nil {
			http.Error(w, `{"error":"Every profile needs a name and settings"}`, 400)
			return
		}
	}
	if err := s.db.SaveProfiles(profiles); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleExportData(w http.ResponseWriter, r *http.Request) {
	prefs, _ := s.db.GetPreferences()
	profiles, _ := s.db.GetProfiles()
	channels, _ := s.db.GetAllChannels()
	downloads, _ := s.db.GetAllDownloads("", "")
	// Never export browser or cookies-file credentials.
	if prefs != nil {
		prefs.CookieSource, prefs.CookieBrowser, prefs.CookieFilePath = "none", "none", ""
	}
	for i := range profiles {
		if profiles[i].Preferences != nil {
			profiles[i].Preferences.CookieSource, profiles[i].Preferences.CookieBrowser, profiles[i].Preferences.CookieFilePath = "none", "none", ""
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="yt-archiver-backup.json"`)
	json.NewEncoder(w).Encode(exportArchive{Version: 1, Preferences: prefs, Profiles: profiles, Channels: channels, Downloads: downloads})
}

func (s *Server) handleImportData(w http.ResponseWriter, r *http.Request) {
	var archive exportArchive
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 15<<20)).Decode(&archive); err != nil || archive.Version != 1 {
		http.Error(w, `{"error":"Invalid YT Archiver backup file"}`, 400)
		return
	}
	if archive.Preferences != nil {
		archive.Preferences.CookieSource, archive.Preferences.CookieBrowser, archive.Preferences.CookieFilePath = "none", "none", ""
		_ = s.db.SavePreferences(archive.Preferences)
	}
	if archive.Profiles != nil {
		_ = s.db.SaveProfiles(archive.Profiles)
	}
	for _, channel := range archive.Channels {
		if channel != nil && channel.ID != "" {
			_ = s.db.SaveChannel(channel)
		}
	}
	for _, item := range archive.Downloads {
		if item != nil && item.ID != "" {
			_ = s.db.CreateDownload(item)
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Imported %d channels and %d library records. Credentials were not imported.", len(archive.Channels), len(archive.Downloads))})
}

func (s *Server) handleEngineHealth(w http.ResponseWriter, r *http.Request) {
	probe := func(command []string) string {
		if len(command) == 0 {
			return "Not configured"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		versionFlag := "--version"
		if strings.Contains(strings.ToLower(command[0]), "ffmpeg") {
			versionFlag = "-version"
		}
		cmd := exec.CommandContext(ctx, command[0], append(command[1:], versionFlag)...)
		sysutil.HideWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "Unavailable: " + err.Error()
		}
		return strings.TrimSpace(strings.Split(string(out), "\n")[0])
	}
	ffmpegCmd := []string{}
	if config.GlobalConfig.FFmpegPath != "" {
		ffmpegCmd = []string{config.GlobalConfig.FFmpegPath}
	}
	jsRuntime := config.GlobalConfig.JSRuntime
	if jsRuntime == "" {
		jsRuntime = "None detected (Node.js/Deno recommended for YouTube challenges)"
	}
	json.NewEncoder(w).Encode(map[string]any{
		"yt_dlp":     probe(config.GlobalConfig.YtDlpCmd),
		"ffmpeg":     probe(ffmpegCmd),
		"js_runtime": jsRuntime,
		"log_file":   logger.GetCurrentLogPath(),
		"checked_at": time.Now(),
	})
}

func (s *Server) handleEngineUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engine string `json:"engine"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Engine != "yt-dlp" {
		http.Error(w, `{"error":"Automatic updates are only available for yt-dlp. Install FFmpeg with your system package manager, then re-run Health Check."}`, 400)
		return
	}
	cmdArgs := config.GlobalConfig.YtDlpCmd
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdArgs[0], append(cmdArgs[1:], "-U")...)
	sysutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "Update failed: "+string(out)), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true, "message": strings.TrimSpace(string(out))})
}

func (s *Server) handleGetLatestLog(w http.ResponseWriter, r *http.Request) {
	logPath := logger.GetCurrentLogPath()
	if logPath == "" {
		http.Error(w, `{"error":"No active log file"}`, 404)
		return
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to read log file: %v"}`, err), 500)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

func (s *Server) handleOpenLogsFolder(w http.ResponseWriter, r *http.Request) {
	logsDir := filepath.Join(config.GlobalConfig.DataDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	openInFileManager(logsDir)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

