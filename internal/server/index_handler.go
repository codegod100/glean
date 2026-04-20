package server

import "net/http"

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	user := s.getUserFromSession(r)
	if user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	s.render(w, r, "index.html", map[string]any{})
}
