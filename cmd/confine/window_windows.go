package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func maybeDetachOwnConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleProcessList := kernel32.NewProc("GetConsoleProcessList")
	freeConsole := kernel32.NewProc("FreeConsole")
	if getConsoleProcessList.Find() != nil || freeConsole.Find() != nil {
		return
	}
	var pids [2]uint32
	count, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), 2)
	if count == 1 {
		_, _, _ = freeConsole.Call()
	}
}

func findAppBrowserExecutable() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	for _, name := range []string{"msedge", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func launchUI(url string, preferApp bool) (<-chan struct{}, func()) {
	if preferApp {
		if appExe := findAppBrowserExecutable(); appExe != "" {
			if doneCh, cleanup, ok := launchChromiumApp(appExe, url, "--disable-features=msEdgeFirstRunExperience"); ok {
				return doneCh, cleanup
			}
		}
	}

	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	return nil, func() {}
}
