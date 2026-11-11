package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// keepAlive keeps proxies from closing a quiet log stream.
const keepAlive = 15 * time.Second

// streamLines sends each line of rc as a Server-Sent Event until the client
// disconnects or rc ends. When rc ends by itself (the service's container
// went away, say) an `end` event is sent, so the browser's EventSource knows
// not to reconnect; its data is the error, if any.
func streamLines(w http.ResponseWriter, r *http.Request, rc io.ReadCloser) {
	defer rc.Close()
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // nginx: don't buffer the stream
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	lines := make(chan string, 64)
	var readErr error
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-r.Context().Done():
				return
			}
		}
		readErr = sc.Err()
	}()

	tick := time.NewTicker(keepAlive)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case line, ok := <-lines:
			if !ok {
				msg := ""
				if readErr != nil && !errors.Is(readErr, io.EOF) {
					msg = readErr.Error()
				}
				fmt.Fprintf(w, "event: end\ndata: %s\n\n", oneLine(msg))
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", oneLine(line))
			// Send a burst (like the initial backlog) in one flush.
			for n := len(lines); n > 0; n-- {
				line, ok := <-lines
				if !ok {
					break
				}
				fmt.Fprintf(w, "data: %s\n\n", oneLine(line))
			}
			flusher.Flush()
		}
	}
}

// oneLine keeps a payload on one SSE data line.
func oneLine(s string) string {
	return strings.NewReplacer("\r", "", "\n", " ").Replace(s)
}
