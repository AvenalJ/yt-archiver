package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/logger"
	"youtube-downloader/internal/queue"
)

// App struct represents the core desktop application
type App struct {
	ctx      context.Context
	db       *db.DB
	queueMgr *queue.QueueManager
}

// NewApp creates a new App application struct
func NewApp(database *db.DB, qm *queue.QueueManager) *App {
	return &App{
		db:       database,
		queueMgr: qm,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logger.Infof("[Desktop] YT Archiver Studio desktop window initialized")
}

// shutdown is called when the application terminates
func (a *App) shutdown(ctx context.Context) {
	logger.Infof("[Desktop] YT Archiver Studio desktop shutting down")
}

// domReady is called when the webview has finished loading the DOM
func (a *App) domReady(ctx context.Context) {
	logger.Infof("[Desktop] Frontend DOM ready")
}

// SelectDirectory opens a native OS folder selection dialog
func (a *App) SelectDirectory(title string) (string, error) {
	if title == "" {
		title = "Select Download Directory"
	}
	dir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// OpenFolder opens the specified directory path in the OS file explorer
func (a *App) OpenFolder(folderPath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", folderPath)
	case "darwin":
		cmd = exec.Command("open", folderPath)
	default:
		cmd = exec.Command("xdg-open", folderPath)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open folder: %w", err)
	}
	return nil
}

// WindowMinimise minimizes the desktop window
func (a *App) WindowMinimise() {
	if a.ctx != nil {
		wailsRuntime.WindowMinimise(a.ctx)
	}
}

// WindowToggleMaximise toggles maximize and restore
func (a *App) WindowToggleMaximise() {
	if a.ctx != nil {
		wailsRuntime.WindowToggleMaximise(a.ctx)
	}
}

// WindowClose terminates the application
func (a *App) WindowClose() {
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	}
}
