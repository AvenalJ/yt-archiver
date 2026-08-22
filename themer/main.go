package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"themer/internal/server"
)

func main() {
	port := flag.Int("port", 9090, "Port to run YT Themer on")
	noBrowser := flag.Bool("no-browser", false, "Do not auto-open browser")
	flag.Parse()

	log.Println("==============================================")
	log.Println("  YT Themer — Template Reskinning Studio      ")
	log.Println("==============================================")

	srv := server.NewServer()
	addr := fmt.Sprintf(":%d", *port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	appURL := fmt.Sprintf("http://localhost:%d", *port)
	log.Printf("[Server] YT Themer running on %s", appURL)

	if !*noBrowser {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(appURL)
		}()
	}

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
