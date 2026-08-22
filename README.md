<div align="center">

# 🎬 YT Archiver Studio

**The ultimate high-fidelity, standalone YouTube & media archiver with a modern frosted glass interface.**

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Wails Version](https://img.shields.io/badge/Wails-v2.15-DF1A51?style=flat-square&logo=wails)](https://wails.io)
[![License](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20|%20macOS%20|%20Linux-lightgrey?style=flat-square)](#system-requirements)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen?style=flat-square)](#installation--build)

*Archive full-resolution video streams, companion audio, rich comment trees, localized subtitles, chapters, SponsorBlock data, and complete channel portals into self-contained, offline-ready archives.*

</div>

---

## Table of Contents

- [Core Capabilities](#core-capabilities)
- [Design & Aesthetics](#design--aesthetics)
- [Architecture & Design](#architecture--design)
- [System Requirements](#system-requirements)
- [Installation & Build](#installation--build)
- [Running the Application](#running-the-application)
- [Directory Structure](#directory-structure)
- [Offline Archive Anatomy](#offline-archive-anatomy)
- [Configuration & Download Profiles](#configuration--download-profiles)
- [REST API Reference](#rest-api-reference)
- [Maintenance & Archive Rebuilding](#maintenance--archive-rebuilding)
- [Player Keyboard Shortcuts](#player-keyboard-shortcuts)
- [Troubleshooting & FAQ](#troubleshooting--faq)
- [License](#license)

---

## Core Capabilities

### 1. Standalone Native Desktop App (Wails v2)
- **Frameless Window**: Launches directly into a dedicated desktop window without browser chrome or external dependencies.
- **Custom Window Titlebar**: Features a frosted glass control capsule with reactive Minimize (`—`), Maximize/Restore (`□`), and Close (`✕`) buttons, glowing hover halos, and double-click maximize/restore support.
- **Pure GUI Subsystem**: Starts cleanly without any terminal or console window attached (`-H windowsgui`).
- **Optional Headless / Server-Only Mode**: Can be run via `--server-only` to serve the web interface on a local network port without opening the desktop window.

### 2. High-Fidelity Media & Metadata Archiving
- **Video Streams**: Up to 4K/8K 60FPS HDR with automated best-video/best-audio container merging via FFmpeg.
- **Dual-Track Companion Audio**: Automatic lossless/high-bitrate audio extraction (320kbps MP3, AAC, Opus, FLAC) saved alongside video files for drag-and-drop portability.
- **Hierarchical Comments Preservation**: Extracts top-level and nested reply comment threads, pinned comments, author badges, like counts, and localized commenter avatars bundled into ZIP containers.
- **Embedded Custom Subtitles (CC)**: Offline `.vtt` parsing and custom CSS rendering engine immune to browser `file:///` CORS restrictions.
- **SponsorBlock Integration**: Preserves crowd-sourced sponsor, self-promotion, and interaction timestamps; features automated timeline highlight markers and playback auto-skip.
- **Return YouTube Dislike (RYD)**: Fetches and displays accurate dislike counters alongside public like statistics.
- **Category Folder Routing**: Automatically routes downloads into clean `@username` handle folders segmented into `Videos/`, `Shorts/`, and `Live Streams/`.
- **Channel Archives**: Scrapes full channel metadata, avatars, banners, subscriber milestones, and links, generating standalone `channel.html` portals.
- **Media Server Compatibility**: Generates standard Plex, Jellyfin, Kodi, and Emby XML metadata (`.nfo`) files for automatic scraping by local media servers.

### 3. Dedicated Channel Studio & Smart Auto-Archive Rules
- **Interactive Channel Studio**: Inspect up to 100 recent uploads per channel, search titles, and filter by category (Full Videos, Shorts, Live Streams) or local archive status.
- **Selective Video Archiving**: Checkbox-select specific videos or one-click "Download Latest 5/10" directly into the queue.
- **Smart Monitoring Rules**: Configure per-channel auto-download policies:
  - Minimum video duration threshold (e.g. ignore clips < 5m).
  - Exclude vertical YouTube Shorts (≤ 60s).
  - Exclude Live Stream VODs.
  - Max auto-sync batch caps.

### 4. Standalone Offline Player (`index.html`)
- Completely self-contained HTML/CSS/JS document that works directly over the `file:///` protocol by double-clicking.
- Zero external CDN dependencies: all fonts, icons, avatars, and assets are embedded as inline SVGs and base64/local files.
- Dynamic Ambient Glow lighting that samples video colors in real time.
- Custom timeline scrubber with chapter tick-marks, SponsorBlock segments, and interactive tooltip previews.
- Top-right hover badge linking directly to the original online YouTube video.

---

## Design & Aesthetics

YT Archiver Studio features a modern multi-theme design system:

* **4 Built-in Themes**:
  - **Midnight Studio (Default)**: Sleek, high-contrast dark OLED aesthetic with deep black tones.
  - **Glass**: Translucent panels with frosted depth, ambient chromatic mesh, and specular light tracking.
  - **Aurora**: Vibrant dark aesthetic with northern lights gradients.
  - **Paper**: Ultra-clean, high-legibility light theme.
* **10 Accent Color Palettes**: Rose, Amber, Crimson, Lime, Sunset, Teal, Violet, Indigo, Ocean, Slate.
* **Materials & Blur**: Multi-layer frosted glass elements, translucent borders with bevel highlights, and ambient glow.
* **Specular Cursor Light Tracking**: Dynamic radial highlight that tracks your mouse cursor across interactive cards, pills, and window control buttons in real time.
* **Micro-Animations**: Spring hover lifts, active tactile button compression, and smooth tab transitions.

---

## Architecture & Design

YT Archiver is organized into decoupled internal packages within Go:

```
┌─────────────────────────────────────────────────────────────┐
│                    YT Archiver Desktop                      │
│                  (Native Wails v2 Window)                   │
├─────────────────────────────────────────────────────────────┤
│  [ Custom Draggable Titlebar & Window Controls ]            │
│  [ WebView2 Engine (Hardware Accelerated Chromium) ]        │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ Frontend Assets (index.html, style.css, app.js)       │  │
│  │ • Multi-theme system (Midnight Studio, Glass, etc.)   │  │
│  │ • 10 Accent Color Palettes                            │  │
│  │ • Specular light tracking, spring lifts, tab fades    │  │
│  └───────────────────────────────────────────────────────┘  │
│                            │                                │
│                            ▼ (AssetsHandler / Bindings)     │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ Go Desktop Backend (app.go / main.go)                 │  │
│  │ • Internal Router & REST API (server.NewServer)       │  │
│  │ • SQLite DB (internal/db)                             │  │
│  │ • Download Queue & Worker Pool (internal/queue)       │  │
│  │ • Native Dialogs & Window Lifecycle (Runtime)         │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## System Requirements

- **Operating System**: Windows 10/11 (64-bit), macOS 12+, or modern Linux distribution.
- **Go**: Version 1.22 or higher (if compiling from source).
- **yt-dlp**: Required on the system PATH, or placed directly in the working directory.
- **FFmpeg & FFprobe**: Required for media stream merging, audio extraction, and storyboard generation.
- **Microsoft Edge WebView2**: Pre-installed on Windows 10/11.

---

## Installation & Build

### 1. Clone the Repository
```bash
git clone https://github.com/your-username/yt-archiver.git
cd yt-archiver
```

### 2. Verify External Dependencies
Ensure `yt-dlp` and `ffmpeg` are installed and available on your system path:
```bash
yt-dlp --version
ffmpeg -version
```

### 3. Compile the Desktop Application
Compile the standalone desktop executable using Go build tags:

```bash
# Windows (Pure GUI without console window)
go build -tags desktop,production -ldflags "-H windowsgui -s -w" -o yt-archiver.exe .

# Linux / macOS
go build -tags desktop,production -ldflags "-s -w" -o yt-archiver .
```

Alternatively, build using the Wails v2 CLI:
```bash
wails build -o yt-archiver.exe
```

### 4. Compile Auxiliary Tools (Optional)

Compile the offline archive rebuild tool:
```bash
go build -o rebuild.exe ./cmd/rebuild/main.go
```

Compile the standalone template reskinning tool:
```bash
go build -o themer.exe ./themer/main.go
```

### 5. Run Unit Tests
```bash
go test ./internal/...
```

---

## Running the Application

### Native Desktop App
Double-click `yt-archiver.exe` or run:
```powershell
.\yt-archiver.exe
```

### Headless Server Mode (CLI)
To run YT Archiver as a headless web server accessible over HTTP (without opening the desktop window):
```bash
./yt-archiver.exe -server-only -port 8989
```

Then open your browser and navigate to `http://localhost:8989`.

---

## Directory Structure

```
.
├── cmd/
│   └── rebuild/                 # Standalone tool to regenerate HTMLs from disk
├── themer/                      # Standalone template reskinning studio
├── internal/
│   ├── config/                  # Global application configuration & tool detection
│   ├── db/                      # SQLite persistence layer and schemas
│   ├── engine/                  # yt-dlp, FFmpeg, InnerTube, SponsorBlock integrations
│   ├── generator/               # HTML template generators (Video, Channel, Portal)
│   ├── logger/                  # Structured session file and console logging
│   ├── queue/                   # Concurrent worker pool and SSE broadcaster
│   └── server/                  # HTTP router, REST handlers, and embedded dashboard
├── data/                        # SQLite database (yt_downloader.db) and session logs
├── downloads/                   # Default destination for archived videos
├── app.go                       # Desktop application lifecycle & native bindings
├── main.go                      # Application entrypoint (Wails v2 + CLI fallback)
├── wails.json                   # Wails project configuration
├── go.mod
└── go.sum
```

---

## Offline Archive Anatomy

Every archived video is stored in its own folder inside `downloads/`. Each folder is completely self-sustaining:

```
downloads/Channels/@Creator/Videos/Video Title [VideoID]/
├── index.html                   # Standalone offline player and full report
├── Video Title.mp4              # Full-resolution video stream
├── Video Title.mp3              # Companion 320kbps audio track
├── Video Title.jpg              # High-resolution video thumbnail
├── Video Title.info.json        # Raw yt-dlp metadata document
├── Video Title.en.vtt           # Subtitles / Closed Captions file
├── Video Title.nfo              # Plex / Jellyfin / Kodi metadata XML
├── movie.nfo                    # Folder-level media server metadata XML
├── channel.html                 # Offline channel archive and community portal
├── channel.json                 # Structured channel metadata
├── channel_avatar.jpg           # Creator avatar image
├── channel_banner.jpg           # Creator wide banner image (if available)
├── comments.json                # Hierarchical comments tree with replies
├── avatars.zip                  # Compressed archive of all commenter avatars
├── storyboard.jpg               # Timeline hover preview sprite sheet (if available)
└── storyboard.json              # Frame coordinates for scrubber preview
```

---

## Configuration & Download Profiles

YT Archiver provides configurable settings stored in `data/yt_downloader.db` and accessible via Preferences:

### Download Settings
- **Output Directory**: Custom folder location for archives.
- **Concurrent Downloads**: Number of simultaneous download threads (1 to 8).
- **Rate Limit**: Bandwidth ceiling in KB/s (0 for unlimited).
- **Preferred Quality**: Best Available, 4K, 1440p, 1080p, 720p, 480p, or Audio Only.
- **Video Format**: MP4, MKV, or WebM.
- **Companion Audio**: Toggle companion audio extraction (MP3, AAC, Opus, FLAC).
- **Offline HTML Generator**: Toggle interactive offline YouTube experience generation.
- **Plex/Jellyfin NFO**: Toggle `.nfo` metadata generation.

---

## REST API Reference

When running the internal server or desktop app, the backend exposes:

### Queue & Download Management
- `GET /api/queue` - Retrieve all active, queued, and completed items.
- `POST /api/queue/add` - Enqueue a URL for download.
- `POST /api/queue/{id}/pause` - Pause an active download.
- `POST /api/queue/{id}/resume` - Resume a paused download.
- `POST /api/queue/{id}/cancel` - Cancel a queued or active download.
- `POST /api/queue/clear` - Clear pending, active, and failed downloads from the queue.
- `POST /api/queue/retry-all-failed` - Re-enqueue all failed tasks.
- `POST /api/download/batch` - Enqueue multiple URLs simultaneously.
- `GET /api/events` - Server-Sent Events (SSE) stream for live progress updates.

### Channel Subscriptions & Studio
- `GET /api/channels` - List all monitored channels and sync statuses.
- `POST /api/channels/add` - Add a channel URL or handle for tracking.
- `GET /api/channels/{id}/catalog` - Inspect channel video catalog and local archive statuses.
- `POST /api/channels/{id}/rules` - Configure per-channel auto-download monitoring rules.
- `POST /api/channels/{id}/enqueue-selected` - Enqueue specific selected video IDs for download.
- `POST /api/channels/{id}/sync` - Trigger an immediate RSS sync for new uploads.
- `DELETE /api/channels/{id}` - Remove a channel from tracking.

### System & Preferences
- `GET /api/preferences` - Fetch user configuration and download preferences.
- `POST /api/preferences` - Save updated configuration.
- `GET /api/profiles` - Retrieve all custom download profiles.
- `POST /api/profiles` - Save download profiles.
- `POST /api/cookies/upload` - Upload a Netscape `cookies.txt` file.
- `DELETE /api/cookies` - Clear stored cookies.
- `GET /api/engine/health` - Check `yt-dlp` and `ffmpeg` binary health and versions.
- `POST /api/engine/update` - Trigger an automatic update for `yt-dlp`.

---

## Maintenance & Archive Rebuilding

If you edit the HTML template in `internal/generator/template.html` or want to regenerate all existing offline reports on disk:

```bash
go run ./cmd/rebuild
```

This scans your `downloads/` folder, connects to the SQLite comment store, loads cached assets, and writes updated `index.html` and `.nfo` files in seconds.

---

## Player Keyboard Shortcuts

When viewing an offline `index.html` report in any modern web browser:

| Key | Action |
| :--- | :--- |
| `Space` or `k` | Play / Pause playback |
| `j` | Rewind 10 seconds |
| `l` | Fast-forward 10 seconds |
| `Left Arrow` | Rewind 5 seconds |
| `Right Arrow` | Fast-forward 5 seconds |
| `Up Arrow` | Increase volume by 5% |
| `Down Arrow` | Decrease volume by 5% |
| `m` | Mute / Unmute audio |
| `c` | Toggle Subtitles / Closed Captions |
| `t` | Toggle Theater Mode |
| `f` | Toggle Fullscreen |
| `0` - `9` | Seek to 0% - 90% of the video |
| `<` / `>` | Decrease / Increase playback speed |

---

## Troubleshooting & FAQ

#### Why do subtitles not show when opening `index.html` directly (`file:///`)?
Standard browser `<track>` elements block local subtitles under `file:///` due to browser CORS rules. YT Archiver parses `.vtt` files directly and renders captions through a custom floating overlay that functions offline without restriction.

#### Can I move the archive folder to an external hard drive?
Yes. Every video folder inside `downloads/` is self-contained. You can move individual video folders or the entire directory to a NAS, USB drive, or another computer, and all features (video, subtitles, comments, avatars, chapters) will function without modification.

---

## License

This project is licensed under the MIT License.
