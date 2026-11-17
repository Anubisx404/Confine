package main

import (
	"os/exec"
)

func maybeDetachOwnConsole() {
}

func launchUI(url string, preferApp bool) (<-chan struct{}, func()) {
	if preferApp {
		for _, bin := range []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge"} {
			if appExe, err := exec.LookPath(bin); err == nil {
				if doneCh, cleanup, ok := launchChromiumApp(appExe, url); ok {
					return doneCh, cleanup
				}
			}
		}
	}

	_ = exec.Command("xdg-open", url).Start()
	return nil, func() {}
}
