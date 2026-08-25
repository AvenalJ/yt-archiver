# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.5.0] - 2026-08-25

### Added
- Standalone native desktop application via Wails v2 with frameless window and custom titlebar.
- High-fidelity media archiving: video up to 8K HDR, companion audio extraction, hierarchical comments, subtitles, chapters, SponsorBlock, and Return YouTube Dislike.
- Dedicated Channel Studio with interactive catalog inspection and selective video archiving.
- Smart auto-archive monitoring rules per channel (duration thresholds, Shorts/Live exclusion, batch caps).
- Standalone offline player (`index.html`) with ambient glow, chapter markers, SponsorBlock timeline, and keyboard shortcuts.
- Multi-theme design system: Midnight Studio, Glass, Aurora, and Paper themes.
- 10 accent color palettes with specular cursor light tracking and micro-animations.
- Headless server mode (`--server-only`) for CLI and remote access.
- REST API for queue management, channel subscriptions, preferences, and engine health.
- Plex/Jellyfin/Kodi/Emby `.nfo` metadata generation.
- Channel portal HTML generation with avatars, banners, community posts, and playlists.
- Download profiles for saving reusable configuration presets.
- Data export/import with automatic credential stripping.
- Session logging with 10-log retention and in-app log viewer.
- `rebuild` CLI tool for batch regeneration of offline HTML reports.
- `themer` standalone tool for reskinning offline archive templates.
- CI pipeline with multi-platform builds (Windows, macOS, Linux) and automated testing.
