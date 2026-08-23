package engine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/logger"
	"youtube-downloader/internal/sysutil"
)

type DownloadProgressCallback func(progress float64, speed, eta string, downloaded, total int64, step string)

type ActiveProcessManager struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd
	cancels   map[string]context.CancelFunc
}

var ProcessManager = &ActiveProcessManager{
	processes: make(map[string]*exec.Cmd),
	cancels:   make(map[string]context.CancelFunc),
}

func (m *ActiveProcessManager) Register(id string, cmd *exec.Cmd, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[id] = cmd
	m.cancels[id] = cancel
}

func (m *ActiveProcessManager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.processes, id)
	delete(m.cancels, id)
}

func (m *ActiveProcessManager) Stop(id string) bool {
	m.mu.Lock()
	cancel, hasCancel := m.cancels[id]
	cmd, hasCmd := m.processes[id]
	m.mu.Unlock()

	if hasCancel {
		cancel()
	}

	if hasCmd && cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		return true
	}
	return false
}

var (
	// Matches: [download]  45.2% of ~120.50MiB at 4.20MiB/s ETA 00:15
	// or: [download] 100% of 12.34MiB in 00:02
	progressRegex = regexp.MustCompile(`\[download\]\s+([\d\.]+)%\s+of\s+~?([\d\.]+\w+)\s+(?:at\s+([\d\.]+\w+\/s))?\s*(?:ETA\s+(\S+))?`)
)

type DownloadExecutionResult struct {
	MediaFilePath    string
	ThumbnailPath    string
	MetadataFilePath string
	SubtitlesPath    string
}

// ExecuteMediaDownload executes the yt-dlp download with progress parsing and automatic resiliency fallbacks
func ExecuteMediaDownload(
	ctx context.Context,
	downloadID string,
	item *db.DownloadItem,
	prefs *db.UserPreferences,
	onProgress DownloadProgressCallback,
) (*DownloadExecutionResult, error) {
	return executeMediaDownloadInternal(ctx, downloadID, item, prefs, onProgress, false)
}

// ExecuteMediaDownloadWithAltClient forces download using alternative player clients (android_vr, ios, mweb)
func ExecuteMediaDownloadWithAltClient(
	ctx context.Context,
	downloadID string,
	item *db.DownloadItem,
	prefs *db.UserPreferences,
	onProgress DownloadProgressCallback,
) (*DownloadExecutionResult, error) {
	return executeMediaDownloadInternal(ctx, downloadID, item, prefs, onProgress, true)
}

func executeMediaDownloadInternal(
	ctx context.Context,
	downloadID string,
	item *db.DownloadItem,
	prefs *db.UserPreferences,
	onProgress DownloadProgressCallback,
	useAltExtractor bool,
) (*DownloadExecutionResult, error) {
	cfg := config.GlobalConfig
	if len(cfg.YtDlpCmd) == 0 {
		err := fmt.Errorf("yt-dlp command not configured")
		logger.Errorf("[Downloader] %v", err)
		return nil, err
	}

	// Create dedicated folder structure:
	// - Playlists: <DownloadFolder>/Playlists/<SafePlaylistTitle> [<PlaylistID>]/<SafeVideoTitle> [<VideoID>]
	// - Channels:  <DownloadFolder>/Channels/<SafeChannelName>/<SafeVideoTitle> [<VideoID>]
	// - Single:    <DownloadFolder>/<SafeVideoTitle> [<VideoID>]
	safeTitle := config.SanitizeFilename(item.Title)
	if safeTitle == "" || safeTitle == "video" {
		safeTitle = "yt_" + item.VideoID
	}
	videoDirName := fmt.Sprintf("%s [%s]", safeTitle, item.VideoID)

	// Detect video category
	if item.Category == "" {
		if strings.Contains(item.URL, "/shorts/") || (item.Duration > 0 && item.Duration <= 60) {
			item.Category = "Shorts"
		} else if strings.Contains(item.URL, "/live") {
			item.Category = "Live Streams"
		} else {
			item.Category = "Videos"
		}
	}

	baseDir := prefs.DownloadFolder
	if item.PlaylistID != "" || (item.PlaylistTitle != "" && item.PlaylistTotal > 1) {
		playlistName := item.PlaylistTitle
		if playlistName == "" {
			playlistName = "Playlist"
		}
		safePlaylist := config.SanitizeFilename(playlistName)
		if item.PlaylistID != "" && !strings.Contains(safePlaylist, item.PlaylistID) {
			safePlaylist = fmt.Sprintf("%s [%s]", safePlaylist, item.PlaylistID)
		}
		baseDir = filepath.Join(baseDir, "Playlists", safePlaylist)
	} else if item.CurrentStep == "Queued from Channel Sync" || item.CurrentStep == "Queued from Channel Studio" || item.ChannelURL != "" || (item.Channel != "" && (strings.Contains(item.ChannelURL, "/@") || strings.Contains(item.URL, "/@"))) {
		cleanHandle := ""
		if strings.Contains(item.ChannelURL, "/@") {
			cleanHandle = item.ChannelURL[strings.Index(item.ChannelURL, "/@")+1:]
		} else if strings.Contains(item.URL, "/@") {
			cleanHandle = item.URL[strings.Index(item.URL, "/@")+1:]
		}
		cleanHandle = strings.TrimPrefix(cleanHandle, "@")
		cleanHandle = strings.TrimPrefix(cleanHandle, "/")
		cleanHandle = strings.TrimSuffix(cleanHandle, "/videos")
		cleanHandle = strings.TrimSuffix(cleanHandle, "/")

		var folderName string
		if cleanHandle != "" {
			folderName = fmt.Sprintf("@%s", cleanHandle)
		} else if item.Channel != "" {
			folderName = item.Channel
		}

		safeChannel := config.SanitizeFilename(folderName)
		if safeChannel != "" {
			baseDir = filepath.Join(baseDir, "Channels", safeChannel, item.Category)
		}
	}

	outDir := filepath.Join(baseDir, videoDirName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		err = fmt.Errorf("failed to create output directory: %w", err)
		logger.Errorf("[Downloader] %v", err)
		return nil, err
	}

	item.OutputDir = outDir

	// Build output template
	outputTemplate := filepath.Join(outDir, "%(title)s.%(ext)s")

	args := cfg.BuildYtDlpArgs("--continue", "--part", "--no-warnings", "--newline")

	if useAltExtractor {
		args = append(args, "--extractor-args", "youtube:player_client=android_vr,ios,mweb,web")
	}

	// Set output template
	args = append(args, "-o", outputTemplate)

	// Format selection with graceful fallback ladder
	if prefs.DownloadAudioOnly {
		args = append(args, "-x")
		audioFmt := prefs.AudioFormat
		if audioFmt == "" || audioFmt == "best" {
			audioFmt = "mp3"
		}
		args = append(args, "--audio-format", audioFmt)
		if prefs.AudioQuality != "" && prefs.AudioQuality != "best" {
			args = append(args, "--audio-quality", prefs.AudioQuality)
		}
	} else {
		// Video format and quality
		videoFmt := prefs.VideoFormat
		if videoFmt == "" {
			videoFmt = "mp4"
		}

		var formatArg string
		qualityMaxHeight := extractMaxHeight(prefs.VideoQuality)

		if qualityMaxHeight > 0 {
			formatArg = fmt.Sprintf("bestvideo[height<=%d]+bestaudio/best[height<=%d]/bestvideo+bestaudio/best", qualityMaxHeight, qualityMaxHeight)
		} else {
			formatArg = "bestvideo+bestaudio/best"
		}
		args = append(args, "-f", formatArg)

		// Merge output format
		if videoFmt != "best" {
			args = append(args, "--merge-output-format", videoFmt)
		}
	}

	// Always write thumbnail and info.json for complete metadata and offline player
	args = append(args, "--write-thumbnail", "--convert-thumbnails", "jpg", "--write-info-json")

	// Subtitles
	if prefs.DownloadSubtitles {
		args = append(args, "--write-subs", "--write-auto-subs")
		langs := prefs.SubtitleLangs
		if langs == "" {
			langs = "en.*,en,auto"
		}
		args = append(args, "--sub-langs", langs)
		args = append(args, "--sub-format", "vtt/srt/best")
	}

	// Speed limit
	if prefs.SpeedLimit != "" {
		args = append(args, "--limit-rate", prefs.SpeedLimit)
	}

	// Cookies & Authentication (Feature 1)
	cookieArgs := BuildCookieArgs(prefs)
	if len(cookieArgs) > 0 {
		args = append(args, cookieArgs...)
	}

	// SponsorBlock removal (Feature 3)
	if prefs.SponsorBlockAction == "remove" {
		cats := prefs.SponsorBlockCategories
		if cats == "" {
			cats = "sponsor,selfpromo,intro,outro"
		}
		args = append(args, "--sponsorblock-remove", cats)
	}

	// Media Tagging & Embedding (Feature 8)
	if prefs.EmbedMetadata {
		args = append(args, "--embed-metadata")
	}
	if prefs.EmbedCoverArt {
		args = append(args, "--embed-thumbnail")
	}
	if prefs.EmbedChapters {
		args = append(args, "--embed-chapters")
	}
	if prefs.EmbedSubtitles {
		args = append(args, "--embed-subs")
	}

	// Target URL
	args = append(args, item.URL)

	logger.Infof("[Downloader] Starting download: %q [VideoID: %s] -> %s", item.Title, item.VideoID, outDir)
	logger.Debugf("[Downloader] Command: %s %s", args[0], strings.Join(args[1:], " "))

	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, args[0], args[1:]...)
	sysutil.HideWindow(cmd)
	sysutil.SetProcessGroup(cmd)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logger.Errorf("[Downloader] Failed to create stdout pipe: %v", err)
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logger.Errorf("[Downloader] Failed to create stderr pipe: %v", err)
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	ProcessManager.Register(downloadID, cmd, cancel)
	defer ProcessManager.Unregister(downloadID)

	if err := cmd.Start(); err != nil {
		logger.Errorf("[Downloader] Failed to start yt-dlp: %v", err)
		return nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	if onProgress != nil {
		onProgress(0, "0 B/s", "--:--", 0, 0, "Downloading media stream...")
	}

	// Parse stdout for progress updates
	var stderrOutput strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			matches := progressRegex.FindStringSubmatch(line)
			if len(matches) >= 2 {
				pct, err := strconv.ParseFloat(matches[1], 64)
				if err == nil {
					totalStr := ""
					speedStr := ""
					etaStr := ""
					if len(matches) > 2 {
						totalStr = matches[2]
					}
					if len(matches) > 3 {
						speedStr = matches[3]
					}
					if len(matches) > 4 {
						etaStr = matches[4]
					}

					totalBytes := parseBytesString(totalStr)
					downloadedBytes := int64(float64(totalBytes) * (pct / 100.0))

					if onProgress != nil {
						onProgress(pct, speedStr, etaStr, downloadedBytes, totalBytes, "Downloading media stream...")
					}
				}
			} else if strings.Contains(line, "[Merger]") || strings.Contains(line, "[ffmpeg]") {
				logger.Infof("[Downloader] Muxing and finalizing video & audio with FFmpeg for %s", item.VideoID)
				if onProgress != nil {
					onProgress(95.0, "", "", 0, 0, "Muxing & finalizing media formats with FFmpeg...")
				}
			}
		}
		_ = scanner.Err()
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrOutput.WriteString(line)
			stderrOutput.WriteString("\n")
		}
		_ = scanner.Err()
	}()

	err = cmd.Wait()
	wg.Wait()

	if err != nil {
		if cmdCtx.Err() != nil {
			logger.Warnf("[Downloader] Download cancelled or paused for %s (%s)", item.Title, item.VideoID)
			return nil, fmt.Errorf("download was cancelled or paused")
		}
		errLower := strings.ToLower(stderrOutput.String())
		// 1. If cookies failed or caused session/consent errors (expired tokens, consent wall, HTTP 403):
		if len(cookieArgs) > 0 && (strings.Contains(errLower, "cookie") ||
			strings.Contains(errLower, "403") ||
			strings.Contains(errLower, "forbidden") ||
			strings.Contains(errLower, "page needs to be reloaded") ||
			strings.Contains(errLower, "consent") ||
			strings.Contains(errLower, "sign in") ||
			strings.Contains(errLower, "login required") ||
			strings.Contains(errLower, "dpapi") ||
			strings.Contains(errLower, "decrypt") ||
			strings.Contains(errLower, "locked") ||
			strings.Contains(errLower, "could not find")) {
			logger.Warnf("[Downloader] Cookies caused download failure for %s (%s): %s. Retrying without cookies...", item.Title, item.VideoID, strings.TrimSpace(stderrOutput.String()))

			// Clean stale .part and .ytdl files from failed cookie session to prevent HTTP 403 on range resume
			cleanPartFiles(outDir)

			if onProgress != nil {
				onProgress(0, "", "", 0, 0, "Cookies caused failure, retrying directly without cookies...")
			}
			noCookiePrefs := *prefs
			noCookiePrefs.CookieSource = "none"
			noCookiePrefs.CookieBrowser = "none"
			noCookiePrefs.CookieFilePath = ""
			return executeMediaDownloadInternal(ctx, downloadID, item, &noCookiePrefs, onProgress, useAltExtractor)
		}

		// 2. If format or client extraction failed, try alternative extractor client cascade (android_vr, ios, mweb)
		if !useAltExtractor && (strings.Contains(errLower, "requested format not available") ||
			strings.Contains(errLower, "403") ||
			strings.Contains(errLower, "sabr") ||
			strings.Contains(errLower, "signature") ||
			strings.Contains(errLower, "cipher") ||
			strings.Contains(errLower, "n challenge")) {
			logger.Warnf("[Downloader] Download hit stream/client restriction for %s (%s). Retrying with alternative extractor client cascade (android_vr,ios,mweb)...", item.Title, item.VideoID)

			// Clean stale .part files from previous client attempts
			cleanPartFiles(outDir)

			if onProgress != nil {
				onProgress(0, "", "", 0, 0, "Retrying with alternative extractor client & format cascade...")
			}
			return executeMediaDownloadInternal(ctx, downloadID, item, prefs, onProgress, true)
		}

		logger.LogFailure("Downloader", item.ID, item.Title, item.URL, err, stderrOutput.String())
		return nil, fmt.Errorf("download failed: %w (stderr: %s)", err, stderrOutput.String())
	}

	// Locate downloaded artifacts in output directory
	res := scanOutputDirArtifacts(outDir)
	logger.LogSuccess("Downloader", item.ID, item.Title, fmt.Sprintf("Media: %s", filepath.Base(res.MediaFilePath)), fmt.Sprintf("Directory: %s", outDir))
	return res, nil
}

func scanOutputDirArtifacts(dir string) *DownloadExecutionResult {
	res := &DownloadExecutionResult{}
	files, err := os.ReadDir(dir)
	if err != nil {
		return res
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		fullPath := filepath.Join(dir, name)
		ext := strings.ToLower(filepath.Ext(name))

		if ext == ".json" && strings.HasSuffix(name, ".info.json") {
			res.MetadataFilePath = fullPath
		} else if ext == ".jpg" || ext == ".png" || ext == ".webp" {
			if !strings.Contains(name, "avatar") {
				res.ThumbnailPath = fullPath
			}
		} else if ext == ".vtt" || ext == ".srt" {
			res.SubtitlesPath = fullPath
		} else if ext == ".mp4" || ext == ".mkv" || ext == ".webm" || ext == ".mp3" || ext == ".m4a" || ext == ".flac" || ext == ".wav" || ext == ".opus" {
			res.MediaFilePath = fullPath
		}
	}
	return res
}

func extractMaxHeight(quality string) int {
	switch quality {
	case "4320p", "8k", "8K":
		return 4320
	case "2160p", "4k", "4K":
		return 2160
	case "1440p", "2k", "2K":
		return 1440
	case "1080p":
		return 1080
	case "720p":
		return 720
	case "480p":
		return 480
	case "360p":
		return 360
	default:
		return 0
	}
}

func parseBytesString(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	multiplier := int64(1)
	if strings.HasSuffix(s, "GIB") || strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "GIB"), "GB")
	} else if strings.HasSuffix(s, "MIB") || strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "MIB"), "MB")
	} else if strings.HasSuffix(s, "KIB") || strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "KIB"), "KB")
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

func cleanPartFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") || strings.HasSuffix(name, ".temp") {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
}
