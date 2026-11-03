package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ProcessExecutor struct{}

func (ProcessExecutor) Execute(ctx context.Context, lease Lease, task Task, block Block, stdinPath, stdoutPath string) error {
	if block.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(block.TimeoutMS)*time.Millisecond)
		defer cancel()
	}

	var stdin *os.File
	var err error
	if stdinPath != "" {
		stdin, err = os.Open(stdinPath)
		if err != nil {
			return fmt.Errorf("open stdin: %w", err)
		}
		defer stdin.Close()
	}
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create stdout: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(strings.TrimSuffix(stdoutPath, ".out")+".err", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create stderr: %w", err)
	}
	defer stderr.Close()

	command := exec.CommandContext(ctx, block.Argv[0], block.Argv[1:]...)
	command.Dir = block.Dir
	command.Env = append(os.Environ(), block.Env...)
	command.Env = append(command.Env,
		"SKEIN_RUN_ID="+lease.RunID,
		"SKEIN_EPOCH="+strconv.FormatUint(lease.Epoch, 10),
		"SKEIN_FENCE="+strconv.FormatUint(lease.Sequence, 10),
		"SKEIN_TASK_ID="+task.ID,
		"SKEIN_PASS="+strconv.Itoa(lease.Pass),
		"SKEIN_OUTPUT="+stdoutPath,
	)
	if stdinPath != "" {
		command.Env = append(command.Env, "SKEIN_INPUT="+stdinPath)
	}
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	configureProcess(command)
	command.Cancel = func() error {
		return killProcessTree(command)
	}
	command.WaitDelay = 5 * time.Second
	runErr := command.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Errorf("exit code %d", exitErr.ExitCode())
		}
		return runErr
	}
	if err := stdout.Sync(); err != nil {
		return fmt.Errorf("sync stdout: %w", err)
	}
	return nil
}
