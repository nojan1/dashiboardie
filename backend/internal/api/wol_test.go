package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dashiboardie/internal/wol"
)

func TestWake(t *testing.T) {
	hosts, err := wol.Parse([]string{"nas=aa:bb:cc:dd:ee:ff@192.168.1.255"})
	if err != nil {
		t.Fatal(err)
	}
	waker := wol.New(hosts)
	var sentTo []string
	waker.Send = func(_ context.Context, target string, packet []byte) error {
		sentTo = append(sentTo, target)
		return nil
	}
	mux := http.NewServeMux()
	New(nil, nil, nil, waker).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/wol", nil))
	var body struct{ Hosts []wol.Host }
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || len(body.Hosts) != 1 || body.Hosts[0].Name != "nas" {
		t.Fatalf("list: %v %+v", err, body)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/wol/nas", nil))
	if rec.Code != http.StatusNoContent || len(sentTo) != 1 || sentTo[0] != "192.168.1.255:9" {
		t.Fatalf("wake: %d %v", rec.Code, sentTo)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/wol/other", nil))
	if rec.Code != http.StatusNotFound || len(sentTo) != 1 {
		t.Fatalf("unknown host: %d %v", rec.Code, sentTo)
	}
}
