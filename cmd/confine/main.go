package main

import (
	"confine/internal/server"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func main() {
	path := flag.String("path", ".", "target codebase path")
	port := flag.Int("port", 8080, "HTTP server port")
	openBrowser := flag.Bool("open", true, "open browser on startup")
	excludeFlag := flag.String("exclude", "", "comma-separated exclude patterns")
	flag.Parse()

	if *port <= 0 || *port > 65535 {
		fmt.Fprintf(os.Stderr, "invalid port: %d\n", *port)
		os.Exit(1)
	}

	var initialExcludes []string
	if strings.TrimSpace(*excludeFlag) != "" {
		initialExcludes = strings.Split(*excludeFlag, ",")
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv, err := server.NewServerWithOptions(*path, addr, initialExcludes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize server: %v\n", err)
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "port %d unavailable (%v), falling back to an ephemeral port\n", *port, err)
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to listen: %v\n", err)
			os.Exit(1)
		}
	}

	actualPort := *port
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		actualPort = tcpAddr.Port
	}
	srv.Addr = ln.Addr().String()

	url := fmt.Sprintf("http:%s%s127.0.0.1:%d", "/", "/", actualPort)
	fmt.Printf("Confine serving %s at %s\n", srv.Root, url)

	if *openBrowser {
		go launchBrowser(url)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-sigCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
			os.Exit(1)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}
}

func launchBrowser(url string) {
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
