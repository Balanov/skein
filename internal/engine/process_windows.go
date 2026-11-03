//go:build windows

package engine

import (
	"os"
	"os/exec"
	"strconv"
)

func configureProcess(command *exec.Cmd) {}

func killProcessTree(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	pid := strconv.Itoa(command.Process.Pid)
	err := exec.Command("taskkill", "/PID", pid, "/T", "/F").Run()
	if err != nil {
		return command.Process.Kill()
	}
	return nil
}
