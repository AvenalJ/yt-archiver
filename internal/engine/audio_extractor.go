package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/config"
	"github.com/AvenalJ/yt-archiver/internal/logger"
	"github.com/AvenalJ/yt-archiver/internal/sysutil"
)

// ExtractCompanionAudioFile extracts a high-quality companion audio file from the downloaded video using FFmpeg
func ExtractCompanionAudioFile(ctx context.Context, mediaFilePath string, outputDir string, format string) string {
	if mediaFilePath == "" || outputDir == "" {
		return ""
	}

	if _, err := os.Stat(mediaFilePath); err != nil {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(mediaFilePath))
	// If the media file is already an audio file, return it
	if ext == ".mp3" || ext == ".m4a" || ext == ".flac" || ext == ".wav" || ext == ".opus" {
		return filepath.Base(mediaFilePath)
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "mp3"
	}

	baseName := strings.TrimSuffix(filepath.Base(mediaFilePath), filepath.Ext(mediaFilePath))
	audioFilename := baseName + "." + format
	audioPath := filepath.Join(outputDir, audioFilename)

	// Check if already extracted
	if fi, err := os.Stat(audioPath); err == nil && fi.Size() > 0 {
		return audioFilename
	}

	ffmpegPath := "ffmpeg"
	if config.GlobalConfig != nil && config.GlobalConfig.FFmpegPath != "" {
		ffmpegPath = config.GlobalConfig.FFmpegPath
	}

	var args []string
	switch format {
	case "m4a", "aac":
		args = []string{"-y", "-i", mediaFilePath, "-vn", "-c:a", "aac", "-b:a", "256k", audioPath}
	case "opus", "ogg":
		args = []string{"-y", "-i", mediaFilePath, "-vn", "-c:a", "libopus", "-b:a", "160k", audioPath}
	case "flac":
		args = []string{"-y", "-i", mediaFilePath, "-vn", "-c:a", "flac", audioPath}
	case "wav":
		args = []string{"-y", "-i", mediaFilePath, "-vn", "-c:a", "pcm_s16le", audioPath}
	case "mp3":
		fallthrough
	default:
		audioFilename = baseName + ".mp3"
		audioPath = filepath.Join(outputDir, audioFilename)
		args = []string{"-y", "-i", mediaFilePath, "-vn", "-c:a", "libmp3lame", "-b:a", "320k", audioPath}
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, ffmpegPath, args...)
	sysutil.HideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warnf("[AudioExtractor] Failed to extract companion audio for %s: %v (output: %s)", mediaFilePath, err, string(out))
		return ""
	}

	logger.Infof("[AudioExtractor] Successfully extracted companion audio: %s", audioFilename)
	return audioFilename
}
