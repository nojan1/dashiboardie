package execx

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake is a Runner for tests. Responses are keyed by the full command line
// ("name arg1 arg2"); every call is recorded in Calls.
type Fake struct {
	mu        sync.Mutex
	Responses map[string]string
	Errors    map[string]error
	Calls     []string
}

func (f *Fake) run(name string, args []string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, line)
	if err, ok := f.Errors[line]; ok {
		return nil, err
	}
	if out, ok := f.Responses[line]; ok {
		return []byte(out), nil
	}
	return nil, fmt.Errorf("fake: unexpected command %q", line)
}

func (f *Fake) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	return f.run(name, args)
}

func (f *Fake) Combined(_ context.Context, name string, args ...string) ([]byte, error) {
	return f.run(name, args)
}

// Called reports whether the exact command line was run.
func (f *Fake) Called(line string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.Calls {
		if c == line {
			return true
		}
	}
	return false
}
