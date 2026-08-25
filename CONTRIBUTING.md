# Contributing to YT Archiver Studio

Thank you for your interest in contributing to **YT Archiver Studio**! This document provides guidelines and instructions for setting up your local environment, building the project, and submitting contributions.

---

## 🛠️ Development Setup

### Prerequisites
1. **Go** (1.25+ recommended): [golang.org](https://golang.org/dl/)
2. **Wails v2 CLI**:
   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
3. **yt-dlp**:
   - Windows: `winget install yt-dlp` or `pip install yt-dlp`
   - macOS: `brew install yt-dlp`
   - Linux: `sudo apt install yt-dlp` or `pip install yt-dlp`
4. **FFmpeg**:
   - Windows: `winget install Gyan.FFmpeg`
   - macOS: `brew install ffmpeg`
   - Linux: `sudo apt install ffmpeg`

---

## 🏗️ Building & Running Locally

### 1. Run in Live Development Mode
```bash
wails dev
```

### 2. Build Desktop Executable (Production)
```bash
wails build -o yt-archiver.exe
```

### 3. Run Backend / Headless Server Only
```bash
go run main.go --server-only
```

---

## 📂 Project Architecture

```
├── app.go                      # Wails desktop lifecycle & native OS bindings
├── main.go                     # Desktop entry point & Wails config
├── wails.json                  # Wails v2 project configuration
├── internal/
│   ├── config/                 # Engine & runtime configuration (yt-dlp, ffmpeg)
│   ├── db/                     # SQLite persistence & migration layers (WAL mode)
│   ├── engine/                 # yt-dlp downloader, comment extractor, FFmpeg companion audio, spritesheets
│   ├── generator/              # Standalone offline HTML player & media portal generator
│   ├── logger/                 # Structured console & file logging
│   ├── queue/                  # Concurrent worker pool, rate limiter, circuit breaker, SSE broadcaster
│   ├── server/                 # REST API, static asset server, preferences, channel sync
│   │   └── static/             # Vanilla multi-theme frontend (HTML, CSS, JS)
│   └── sysutil/                # OS-specific process suppression (CREATE_NO_WINDOW)
├── cmd/
│   └── rebuild/                # Offline archive HTML & NFO batch regenerator
└── build/                      # App icon assets & Windows manifest
```

---

## 📝 Pull Request Guidelines

1. **Create a branch**: `git checkout -b feature/my-new-feature` or `git checkout -b fix/issue-description`.
2. **Code Style**:
   - Go: Run `go fmt ./...` and `go vet ./...` before committing.
   - Frontend: Vanilla CSS & JS only (no heavy UI frameworks or Tailwind unless discussed). Ensure UI changes work harmoniously across all 4 built-in themes (Midnight Studio, Glass, Aurora, Paper).
3. **Tests**: Ensure tests pass by running `go test ./...`.
4. **Submit PR**: Open a pull request targeting `main` with a clear description of your changes and any relevant screenshots.

---

## ⚖️ Code of Conduct
Please review and adhere to our [Code of Conduct](CODE_OF_CONDUCT.md) in all community interactions.
