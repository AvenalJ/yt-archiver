package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/logger"
	"youtube-downloader/internal/queue"
	"youtube-downloader/internal/server"
)

//go:embed all:internal/server/static
var rawAssets embed.FS

func main() {
	defaultPort := 8989
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			defaultPort = p
		}
	}
	port := flag.Int("port", defaultPort, "Port to run the web application server on")
	serverOnly := flag.Bool("server-only", false, "Run as headless HTTP server without desktop window")
	noBrowser := flag.Bool("no-browser", false, "Do not automatically open browser on startup (server-only mode)")
	flag.Parse()

	log.Println("==================================================")
	log.Println("  YT Archiver Studio - YouTube & Playlist Downloader  ")
	log.Println("==================================================")

	// 1. Initialize Configuration & Paths
	cfg, err := config.InitConfig(*port)
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Initialize Session Logger in data/logs (max 10 sessions retained)
	sessionLogger, err := logger.InitLogger(cfg.DataDir)
	if err != nil {
		log.Printf("[Warning] Failed to initialize file logger: %v", err)
	} else {
		defer sessionLogger.Close()
	}

	logger.Infof("Operating System: %s (%s)", runtime.GOOS, runtime.GOARCH)
	logger.Infof("[Config] Data Directory: %s", cfg.DataDir)
	logger.Infof("[Config] Default Downloads Directory: %s", cfg.DefaultOut)
	logger.Infof("[Config] yt-dlp Engine Command: %v", cfg.YtDlpCmd)
	logger.Infof("[Config] FFmpeg Executable: %s", cfg.FFmpegPath)
	logger.Infof("[Config] JavaScript Runtime: %s", cfg.JSRuntime)

	// 2. Initialize SQLite Database
	database, err := db.InitDB(cfg.DBPath, cfg.DefaultOut)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer database.Close()
	log.Printf("[Database] SQLite persistent storage initialized at %s", cfg.DBPath)

	// 3. Initialize Download Queue Manager & Worker Pool
	queueManager := queue.InitQueueManager(database)
	log.Printf("[Queue] Download manager initialized with live worker pool")

	// 4. Initialize HTTP Handler
	srvHandler := server.NewServer(database, queueManager)

	// Headless / Server-only mode
	if *serverOnly {
		runHeadlessServer(*port, *noBrowser, srvHandler)
		return
	}

	// 5. Desktop Application via Wails v2
	assets, err := fs.Sub(rawAssets, "internal/server/static")
	if err != nil {
		log.Fatalf("Failed to load embedded frontend assets: %v", err)
	}

	app := NewApp(database, queueManager)

	wailsErr := wails.Run(&options.App{
		Title:             "YT Archiver Studio",
		Width:             1280,
		Height:            820,
		MinWidth:          960,
		MinHeight:         640,
		Frameless:         true,
		Assets:            assets,
		AssetsHandler:     srvHandler,
		BackgroundColour: &options.RGBA{R: 8, G: 8, B: 12, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnDomReady:       app.domReady,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			BackdropType:         windows.Auto,
			DisableWindowIcon:    false,
		},
	})

	if wailsErr != nil {
		log.Fatalf("Error running desktop application: %v", wailsErr)
	}
}

func runHeadlessServer(port int, noBrowser bool, srvHandler http.Handler) {
	addr := fmt.Sprintf(":%d", port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srvHandler,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	appURL := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("[Server] YT Archiver Studio running on %s", appURL)

	if !noBrowser {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openBrowser(appURL)
		}()
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("\n[Shutdown] Shutting down YT Archiver Studio...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	log.Println("[Shutdown] Goodbye!")
}

func openBrowser(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	_ = cmd.Start()
}
