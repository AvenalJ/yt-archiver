package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AvenalJ/yt-archiver/internal/sysutil"
)

type AppConfig struct {
	Port       int
	DataDir    string
	DBPath     string
	DefaultOut string
	YtDlpCmd   []string // Command args to invoke yt-dlp, e.g. ["python", "-m", "yt_dlp"] or ["yt-dlp"]
	FFmpegPath string
	JSRuntime  string   // Detected JS runtime: "node", "deno", "bun", "quickjs", or ""
}

var GlobalConfig *AppConfig

func InitConfig(customPort int) (*AppConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	dataDir := filepath.Join(cwd, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	downloadsDir := filepath.Join(cwd, "downloads")
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		return nil, err
	}

	cfg := &AppConfig{
		Port:       customPort,
		DataDir:    dataDir,
		DBPath:     filepath.Join(dataDir, "yt_downloader.db"),
		DefaultOut: downloadsDir,
	}

	// Detect yt-dlp
	cfg.YtDlpCmd = detectYtDlp()

	// Detect ffmpeg (absolute path)
	cfg.FFmpegPath = detectFFmpeg()

	// Detect JavaScript runtime for YouTube n-challenge solver (node, deno, bun, quickjs)
	cfg.JSRuntime = detectJSRuntime()

	GlobalConfig = cfg
	return cfg, nil
}

// BuildYtDlpArgs constructs base yt-dlp command arguments with detected JS runtime, FFmpeg location, and network resilience flags
func (c *AppConfig) BuildYtDlpArgs(extraArgs ...string) []string {
	args := append([]string{}, c.YtDlpCmd...)
	if c.JSRuntime != "" {
		args = append(args, "--js-runtimes", c.JSRuntime)
	}
	if c.FFmpegPath != "" {
		args = append(args, "--ffmpeg-location", c.FFmpegPath)
	}

	// Automatic network & download resilience flags
	args = append(args,
		"--retries", "10",
		"--fragment-retries", "10",
		"--file-access-retries", "3",
		"--retry-sleep", "5",
		"--socket-timeout", "30",
		"--extractor-retries", "3",
		"--http-chunk-size", "10M",
	)

	args = append(args, extraArgs...)
	return args
}

func detectYtDlp() []string {
	// Check standalone yt-dlp in PATH
	if p, err := exec.LookPath("yt-dlp"); err == nil && p != "" {
		return []string{"yt-dlp"}
	}
	if p, err := exec.LookPath("yt-dlp.exe"); err == nil && p != "" {
		return []string{p}
	}

	// Check python -m yt_dlp
	for _, py := range []string{"python", "python3", "py"} {
		if p, err := exec.LookPath(py); err == nil && p != "" {
			cmd := exec.Command(py, "-m", "yt_dlp", "--version")
			sysutil.HideWindow(cmd)
			if err := cmd.Run(); err == nil {
				return []string{py, "-m", "yt_dlp"}
			}
		}
	}

	// Fallback to "yt-dlp"
	return []string{"yt-dlp"}
}

func detectFFmpeg() string {
	for _, name := range []string{"ffmpeg", "ffmpeg.exe"} {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			if abs, err := filepath.Abs(p); err == nil {
				return abs
			}
			return p
		}
	}
	return ""
}

func detectJSRuntime() string {
	for _, rt := range []string{"node", "deno", "bun", "quickjs"} {
		if p, err := exec.LookPath(rt); err == nil && p != "" {
			return rt
		}
		if p, err := exec.LookPath(rt + ".exe"); err == nil && p != "" {
			return rt
		}
	}
	return ""
}

func SanitizeFilename(name string) string {
	invalid := `<>:"/\|?*` + string(rune(0))
	var sb strings.Builder
	for _, r := range name {
		if strings.ContainsRune(invalid, r) || r < 32 {
			sb.WriteRune('_')
		} else {
			sb.WriteRune(r)
		}
	}
	res := strings.TrimSpace(sb.String())
	res = strings.Trim(res, ". ")
	// Clamp length to 90 runes to prevent Windows MAX_PATH (260 char) errors when combined with subdirectories
	runes := []rune(res)
	if len(runes) > 90 {
		res = string(runes[:90])
	}
	res = strings.Trim(res, ". _")
	if res == "" {
		res = "video"
	}
	return res
}
