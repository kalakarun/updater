package installer

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Runner struct {
	Log     io.Writer
	Timeout time.Duration
}

type bounded struct {
	mu        sync.Mutex
	writer    io.Writer
	remaining int
	truncated bool
}

func (buffer *bounded) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	length := len(data)
	if len(data) > buffer.remaining {
		data = data[:buffer.remaining]
		buffer.truncated = true
	}
	buffer.remaining -= len(data)
	if len(data) > 0 && buffer.writer != nil {
		if _, err := buffer.writer.Write(data); err != nil {
			return 0, err
		}
	}
	return length, nil
}

func (runner Runner) Run(parent context.Context, directory string) error {
	timeout := runner.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", "./run.sh")
	command.Dir = directory
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &bounded{remaining: 1048576}
	stderr := &bounded{remaining: 1048576, writer: runner.Log}
	output.writer = runner.Log
	command.Stdout = output
	command.Stderr = stderr
	command.WaitDelay = 5 * time.Second
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	err := command.Run()
	if command.Process != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	if output.truncated || stderr.truncated {
		return errors.Join(err, errors.New("installer output exceeded limit"))
	}
	return err
}
