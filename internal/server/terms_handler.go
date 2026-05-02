package server

import "net/http"

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "terms.html", map[string]any{})
}
