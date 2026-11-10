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
)

type Server struct {
	reg  *service.Registry
	host *host.Reader
}

func New(reg *service.Registry, hostReader *host.Reader) *Server {
	return &Server{reg: reg, host: hostReader}
}

// Register mounts the API routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/host", s.hostStats)
	mux.HandleFunc("GET /api/services", s.listServices)
	mux.HandleFunc("POST /api/services/{source}/{id}/{action}", s.doAction)
	mux.HandleFunc("GET /api/services/{source}/{id}/logs", s.logs)
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
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid lines"))
			return
		}
		lines = n
	}
	src := service.Source(r.PathValue("source"))
	out, err := s.reg.Logs(r.Context(), src, r.PathValue("id"), lines)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if out == nil {
		out = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"lines": out})
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
