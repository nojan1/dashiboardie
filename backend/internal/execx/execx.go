// Package execx runs external commands behind an interface so providers can
// be tested against recorded CLI output.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	// Output returns stdout; on failure the error includes stderr.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Combined returns stdout and stderr interleaved (for logs).
	Combined(ctx context.Context, name string, args ...string) ([]byte, error)
	// Stream starts a long-running command (e.g. `journalctl -f`) and returns
	// its interleaved stdout and stderr. Closing the reader, or ctx ending,
	// kills the command. When the command exits by itself, reads return
	// io.EOF, or an *Error if it failed.
	Stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error)
}

// Exec runs real commands.
type Exec struct{}

func (Exec) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, cmdError(name, args, err, stderr.String())
	}
	return out, nil
}

func (Exec) Combined(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return out, cmdError(name, args, err, "")
	}
	return out, nil
}

func (Exec) Stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, name, args...)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	// Don't let a child that inherited the pipe keep Wait blocked.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, cmdError(name, args, err, "")
	}
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			err = cmdError(name, args, err, "")
		} else {
			err = io.EOF
		}
		pw.CloseWithError(err)
		cancel()
	}()
	return &stream{PipeReader: pr, cancel: cancel}, nil
}

type stream struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (s *stream) Close() error {
	s.cancel()
	return s.PipeReader.Close()
}

// Error is returned when a command fails. ExitCode is -1 if the command
// could not be started or was killed.
type Error struct {
	Cmd      string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s: %v", e.Cmd, e.Err)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func cmdError(name string, args []string, err error, stderr string) error {
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return &Error{
		Cmd:      strings.Join(append([]string{name}, args...), " "),
		ExitCode: code,
		Stderr:   strings.TrimSpace(stderr),
		Err:      err,
	}
}

// Lines splits command output into lines, dropping the trailing newline.
func Lines(out []byte) []string {
	s := strings.TrimRight(string(out), "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}
