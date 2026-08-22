package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxSessionLogs = 10

type SessionLogger struct {
	mu       sync.Mutex
	file     *os.File
	filePath string
	writer   io.Writer
}

var (
	globalMu     sync.RWMutex
	globalLogger *SessionLogger
)

// InitLogger initializes session logging in dataDir/logs, creating a new timestamped log file
// and enforcing a maximum retention of 10 session logs.
func InitLogger(dataDir string) (*SessionLogger, error) {
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create session log file with timestamp
	sessionName := fmt.Sprintf("session_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(logsDir, sessionName)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create session log file: %w", err)
	}

	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	sl := &SessionLogger{
		file:     f,
		filePath: logPath,
		writer:   mw,
	}

	globalMu.Lock()
	globalLogger = sl
	globalMu.Unlock()

	// Enforce 10 log retention limit
	pruneOldLogs(logsDir, MaxSessionLogs)

	Infof("=== New YT Archiver Session Started: %s ===", sessionName)
	Infof("Session Log File: %s", logPath)

	return sl, nil
}

// GetCurrentLogPath returns the absolute path to the active session log file
func GetCurrentLogPath() string {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalLogger != nil {
		return globalLogger.filePath
	}
	return ""
}

// Close flushes and closes the session log file
func (l *SessionLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	Infof("=== YT Archiver Session Ended ===")
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func pruneOldLogs(logsDir string, maxLogs int) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return
	}

	type logFileEntry struct {
		path    string
		modTime time.Time
	}

	var logFiles []logFileEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "session_") && strings.HasSuffix(name, ".log") {
			info, err := entry.Info()
			if err == nil {
				logFiles = append(logFiles, logFileEntry{
					path:    filepath.Join(logsDir, name),
					modTime: info.ModTime(),
				})
			}
		}
	}

	if len(logFiles) <= maxLogs {
		return
	}

	// Sort newest first
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].modTime.After(logFiles[j].modTime)
	})

	// Delete everything beyond maxLogs
	for i := maxLogs; i < len(logFiles); i++ {
		_ = os.Remove(logFiles[i].path)
		log.Printf("[CLEANUP] Pruned old session log: %s", filepath.Base(logFiles[i].path))
	}
}

// Logging helper functions
func Infof(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	log.Printf("[INFO] %s", msg)
}

func Errorf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	log.Printf("[ERROR] %s", msg)
}

func Warnf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	log.Printf("[WARN] %s", msg)
}

func Debugf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	log.Printf("[DEBUG] %s", msg)
}

func LogFailure(component, id, title, url string, err error, stderr string, details ...string) {
	var sb strings.Builder
	sb.WriteString("\n=======================================================\n")
	sb.WriteString(fmt.Sprintf("[DOWNLOAD FAILURE] Component: %s\n", component))
	sb.WriteString(fmt.Sprintf("  ID:        %s\n", id))
	sb.WriteString(fmt.Sprintf("  Title:     %s\n", title))
	sb.WriteString(fmt.Sprintf("  URL:       %s\n", url))
	if err != nil {
		sb.WriteString(fmt.Sprintf("  Error:     %v\n", err))
	}
	for _, d := range details {
		sb.WriteString(fmt.Sprintf("  Detail:    %s\n", d))
	}
	if strings.TrimSpace(stderr) != "" {
		sb.WriteString(fmt.Sprintf("  --- Process Stderr Output ---\n%s\n", strings.TrimSpace(stderr)))
	}
	sb.WriteString("=======================================================")
	log.Println(sb.String())
}

func LogSuccess(component, id, title string, details ...string) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[DOWNLOAD SUCCESS] [%s] ID: %s | %s", component, id, title))
	for _, d := range details {
		sb.WriteString(fmt.Sprintf(" | %s", d))
	}
	log.Println(sb.String())
}
