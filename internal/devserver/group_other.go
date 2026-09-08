//go:build windows || js

package devserver

import "os/exec"

func group(*exec.Cmd) {}

func terminate(command *exec.Cmd) {
	_ = command.Process.Kill()
}
