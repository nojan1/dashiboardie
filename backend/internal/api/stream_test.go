package api

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingReader struct{ io.Reader }

func (f failingReader) Read(p []byte) (int, error) {
	n, err := f.Reader.Read(p)
	if err == io.EOF {
		return n, errors.New("podman logs: exit status 125")
	}
	return n, err
}

func TestStreamLines(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x/follow", nil)
	streamLines(rec, req, io.NopCloser(strings.NewReader("one\ntwo\r\nthree\n")))

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	want := "data: one\n\ndata: two\n\ndata: three\n\nevent: end\ndata: \n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestStreamLinesReportsError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x/follow", nil)
	streamLines(rec, req, io.NopCloser(failingReader{strings.NewReader("one\n")}))
	if got := rec.Body.String(); !strings.HasSuffix(got, "event: end\ndata: podman logs: exit status 125\n\n") {
		t.Fatalf("got %q", got)
	}
}
