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
	"strings"
	"syscall"
	"time"
)

func main() {
	path := flag.String("path", ".", "target codebase path")
	port := flag.Int("port", 8080, "HTTP server port")
	openUI := flag.Bool("open", true, "open desktop window or browser on startup")
	appMode := flag.Bool("app", true, "open in a standalone desktop app window when available")
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

	if *openUI && *appMode {
		maybeDetachOwnConsole()
	}

	var appDoneCh <-chan struct{}
	appCleanup := func() {}
	if *openUI {
		appDoneCh, appCleanup = launchUI(url, *appMode)
	}
	defer appCleanup()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-sigCtx.Done():
		shutdownServer(srv, errCh)
	case <-appDoneCh:
		shutdownServer(srv, errCh)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}
}

func launchChromiumApp(appExe, url string, extraFlags ...string) (<-chan struct{}, func(), bool) {
	profileDir, err := os.MkdirTemp("", "confine-app-*")
	if err != nil {
		return nil, func() {}, false
	}

	args := []string{
		"--app=" + url,
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=1440,920",
	}
	args = append(args, extraFlags...)

	cmd := exec.Command(appExe, args...)
	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profileDir)
		return nil, func() {}, false
	}

	doneCh := make(chan struct{})
	go func() {
		waitErr := cmd.Wait()
		_ = os.RemoveAll(profileDir)
		if time.Since(startedAt) >= 500*time.Millisecond || waitErr != nil {
			close(doneCh)
		}
	}()

	cleanup := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = os.RemoveAll(profileDir)
	}
	return doneCh, cleanup, true
}

func shutdownServer(srv *server.Server, errCh <-chan error) {
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
}
