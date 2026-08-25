package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/config"
	"github.com/AvenalJ/yt-archiver/internal/logger"
	"github.com/AvenalJ/yt-archiver/internal/sysutil"
)

type StoryboardMetadata struct {
	StoryboardFilename string  `json:"storyboard_filename"`
	Cols               int     `json:"cols"`
	Rows               int     `json:"rows"`
	TotalFrames        int     `json:"total_frames"`
	IntervalSeconds    float64 `json:"interval_seconds"`
	FrameWidth         int     `json:"frame_width"`
	FrameHeight        int     `json:"frame_height"`
}

// GenerateOrExtractStoryboard creates or extracts a storyboard thumbnail spritesheet for hover scrubbing
func GenerateOrExtractStoryboard(ctx context.Context, mediaFilePath string, duration int64, outputDir string) *StoryboardMetadata {
	if outputDir == "" || duration <= 0 {
		return nil
	}

	destImage := filepath.Join(outputDir, "storyboard.jpg")
	destJSON := filepath.Join(outputDir, "storyboard.json")

	// If already generated, load existing metadata
	if _, err := os.Stat(destImage); err == nil {
		if data, err := os.ReadFile(destJSON); err == nil {
			var meta StoryboardMetadata
			if json.Unmarshal(data, &meta) == nil && meta.Cols > 0 {
				return &meta
			}
		}
	}

	if mediaFilePath == "" {
		return nil
	}
	if _, err := os.Stat(mediaFilePath); err != nil {
		return nil
	}

	// Target a 10x10 (100 frame) grid
	cols := 10
	rows := 10
	totalFrames := cols * rows // 100
	interval := float64(duration) / float64(totalFrames)
	if interval < 0.5 {
		interval = 0.5
	}

	frameW := 160
	frameH := 90

	// Use ffmpeg to generate the tiled spritesheet
	ffmpegPath := config.GlobalConfig.FFmpegPath
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	fpsExpr := fmt.Sprintf("fps=1/%.4f,scale=%d:%d,tile=%dx%d", interval, frameW, frameH, cols, rows)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, ffmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-ss", "0",
		"-i", mediaFilePath,
		"-vf", fpsExpr,
		"-frames:v", "1",
		"-q:v", "3",
		"-y", destImage,
	)
	sysutil.HideWindow(cmd)

	if err := cmd.Run(); err != nil {
		logger.Warnf("[Storyboard] Failed to generate spritesheet for %s: %v", filepath.Base(mediaFilePath), err)
		return nil
	}

	meta := &StoryboardMetadata{
		StoryboardFilename: "storyboard.jpg",
		Cols:               cols,
		Rows:               rows,
		TotalFrames:        totalFrames,
		IntervalSeconds:    math.Round(interval*100) / 100,
		FrameWidth:         frameW,
		FrameHeight:        frameH,
	}

	if jsonBytes, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(destJSON, jsonBytes, 0644)
	}

	logger.Infof("[Storyboard] Generated %dx%d timeline spritesheet (%.2fs interval) for %s",
		cols, rows, meta.IntervalSeconds, filepath.Base(mediaFilePath))
	return meta
}
