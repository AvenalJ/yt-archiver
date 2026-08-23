# Network Bandwidth Indicator & Data Usage Tracking

Add an app-scoped network bandwidth indicator to the Downloads & Queue page with optional persistent data usage tracking and an interactive chart modal.

## User Requirements Recap
1. **Hover / Floating Indicator**: Live network speed & session usage pill on the Downloads/Queue page.
2. **Appearance Settings**:
   - Toggle to enable/disable the bandwidth indicator.
   - Toggle to enable/disable persistent total data usage tracking in SQLite (disabled unless indicator is enabled).
3. **Interactive Modal**: Clicking the pill opens an expanded view with lifetime/total stats and an interactive chart.

---

## Proposed Changes

### 1. Backend: Bandwidth Tracker & Aggregator

#### [NEW] [bandwidth.go](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/queue/bandwidth.go)
- `BandwidthTracker` struct tracking real-time sliding window byte deltas across active downloads.
- Method `RecordProgress(itemID string, downloadedBytes int64)` called from the queue progress callback.
- Method `GetStats()` returning `{ speed_bps, session_bytes, active_streams }`.
- Background ticker broadcasting `network_stats` over SSE every second, and flushing usage to the SQLite DB if tracking is enabled.

#### [MODIFY] [manager.go](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/queue/manager.go)
- Wire `BandwidthTracker` into `QueueManager`.
- Hook progress callback to invoke `RecordProgress(item.ID, downloaded)`.

---

### 2. Database: Persistent Data Usage & Preferences

#### [MODIFY] [models.go](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/db/models.go)
- Add `ShowBandwidthIndicator bool` and `TrackDataUsage bool` to `UserPreferences`.
- Add `BandwidthHourEntry` struct `{ HourBucket string, BytesDown int64 }`.

#### [MODIFY] [db.go](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/db/db.go)
- Add migration creating `bandwidth_usage` table:
  ```sql
  CREATE TABLE IF NOT EXISTS bandwidth_usage (
      hour_bucket TEXT PRIMARY KEY,
      bytes_down  INTEGER NOT NULL DEFAULT 0
  );
  ```
- Add helper methods: `IncrementBandwidthUsage()`, `GetBandwidthUsage(fromHour, toHour string)`, `GetTotalBandwidthUsage()`, `ResetBandwidthUsage()`.

---

### 3. API Routes

#### [MODIFY] [router.go](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/server/router.go)
- Add `GET /api/bandwidth-usage` returning hourly breakdown and total tracked usage.
- Add `POST /api/bandwidth-usage/reset` to reset tracking if desired.

---

### 4. Frontend: UI & Chart

#### [MODIFY] [index.html](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/server/static/index.html)
- Add floating glassmorphic bandwidth pill on the Queue tab.
- Add "Network Bandwidth" settings card in the Appearance tab with dependent toggles.
- Add interactive usage panel modal containing lifetime statistics and chart canvas.

#### [MODIFY] [app.js](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/server/static/app.js)
- Handle `network_stats` SSE event to update speed and session bytes live.
- Wire up settings toggles with UI interlock (cannot enable tracking if indicator is off).
- Render lightweight interactive Canvas bar chart with hover tooltips and range filters (last 24h, 7d, 30d).

#### [MODIFY] [style.css](file:///c:/Users/jak/Desktop/Projects/New%20folder%20(4)/internal/server/static/style.css)
- Style the pill, pulse animations, modal dialog, and chart controls.

---

## Verification Plan

### Automated Tests
- Run `go test ./internal/db -v` to verify bandwidth DB migrations and queries.
- Run `go build -o yt-archiver.exe .` to ensure complete compilation without errors.

### Manual Verification
1. Open Appearance tab &rarr; toggle on "Show bandwidth indicator".
2. Verify floating pill appears on Queue tab.
3. Test dependency toggle &rarr; verify "Track total data usage" cannot be turned on if indicator is off.
4. Start a download &rarr; verify real-time download speed and session data counters update.
5. Click the pill &rarr; verify modal opens with lifetime usage stats and interactive chart.
