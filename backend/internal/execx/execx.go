// Package execx runs external commands behind an interface so providers can
// be tested against recorded CLI output.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Runner interface {
	// Output returns stdout; on failure the error includes stderr.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Combined returns stdout and stderr interleaved (for logs).
	Combined(ctx context.Context, name string, args ...string) ([]byte, error)
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
