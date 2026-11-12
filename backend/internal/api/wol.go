package api

import (
	"log/slog"
	"net/http"

	"dashiboardie/internal/wol"
)

func (s *Server) wolHosts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]wol.Host{"hosts": s.wol.Hosts()})
}

func (s *Server) wake(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.wol == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.wol.Wake(r.Context(), name); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	slog.Info("sent wake-on-lan packet", "host", name)
	w.WriteHeader(http.StatusNoContent)
}
