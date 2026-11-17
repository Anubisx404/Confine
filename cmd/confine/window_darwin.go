package main

import (
	"os/exec"
)

func maybeDetachOwnConsole() {
}

func launchUI(url string, preferApp bool) (<-chan struct{}, func()) {
	_ = exec.Command("open", url).Start()
	return nil, func() {}
}
