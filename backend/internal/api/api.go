// Package api exposes the registry over HTTP. There is deliberately no
// authentication: access control is handled by the reverse proxy.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"dashiboardie/internal/host"
	"dashiboardie/internal/service"
	"dashiboardie/internal/units"
)

type Server struct {
	reg   *service.Registry
	host  *host.Reader
	units units.Source
}

func New(reg *service.Registry, hostReader *host.Reader, u units.Source) *Server {
	return &Server{reg: reg, host: hostReader, units: u}
}

// Register mounts the API routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/host", s.hostStats)
	mux.HandleFunc("GET /api/services", s.listServices)
	mux.HandleFunc("POST /api/services/{source}/{id}/{action}", s.doAction)
	mux.HandleFunc("GET /api/services/{source}/{id}/logs", s.logs)
	mux.HandleFunc("GET /api/services/{source}/{id}/logs/follow", s.followLogs)
	mux.HandleFunc("GET /api/units", s.unitsOverview)
	mux.HandleFunc("GET /api/units/{unit}/logs", s.unitLogs)
	mux.HandleFunc("GET /api/units/{unit}/logs/follow", s.followUnitLogs)
}

func (s *Server) hostStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.host.Read(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if stats.Filesystems == nil {
		stats.Filesystems = []host.Filesystem{}
	}
	writeJSON(w, http.StatusOK, stats)
}

type listResponse struct {
	Services []service.Service       `json:"services"`
	Errors   []service.ProviderError `json:"errors"`
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	svcs, errs := s.reg.List(r.Context())
	if svcs == nil {
		svcs = []service.Service{}
	}
	if errs == nil {
		errs = []service.ProviderError{}
	}
	writeJSON(w, http.StatusOK, listResponse{Services: svcs, Errors: errs})
}

func (s *Server) doAction(w http.ResponseWriter, r *http.Request) {
	action := service.Action(r.PathValue("action"))
	switch action {
	case service.ActionStart, service.ActionStop, service.ActionRestart:
	default:
		writeError(w, http.StatusBadRequest, errors.New("unknown action"))
		return
	}
	src := service.Source(r.PathValue("source"))
	if err := s.reg.Do(r.Context(), src, r.PathValue("id"), action); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	lines, ok := linesParam(w, r)
	if !ok {
		return
	}
	src := service.Source(r.PathValue("source"))
	out, err := s.reg.Logs(r.Context(), src, r.PathValue("id"), lines)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeLines(w, out)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrNotImplemented):
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

const maxLines = 10000

// linesParam reads ?lines=N (default 200); on a bad value it writes a 400.
func linesParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("lines")
	if v == "" {
		return 200, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > maxLines {
		writeError(w, http.StatusBadRequest, errors.New("invalid lines"))
		return 0, false
	}
	return n, true
}

func writeLines(w http.ResponseWriter, lines []string) {
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"lines": lines})
}

func (s *Server) followLogs(w http.ResponseWriter, r *http.Request) {
	lines, ok := linesParam(w, r)
	if !ok {
		return
	}
	src := service.Source(r.PathValue("source"))
	rc, err := s.reg.FollowLogs(r.Context(), src, r.PathValue("id"), lines)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	streamLines(w, r, rc)
}

func (s *Server) unitsOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.units.Overview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

func (s *Server) unitLogs(w http.ResponseWriter, r *http.Request) {
	lines, ok := linesParam(w, r)
	if !ok {
		return
	}
	out, err := s.units.Logs(r.Context(), r.PathValue("unit"), lines)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeLines(w, out)
}

func (s *Server) followUnitLogs(w http.ResponseWriter, r *http.Request) {
	lines, ok := linesParam(w, r)
	if !ok {
		return
	}
	rc, err := s.units.FollowLogs(r.Context(), r.PathValue("unit"), lines)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	streamLines(w, r, rc)
}
