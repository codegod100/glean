package server

import "net/http"

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedRecs, _ := s.db.GetFeedRecommendations(r.Context(), user.DID, 20)
	people, _ := s.db.GetPeopleRecommendations(r.Context(), user.DID, 20)
	popular, _ := s.db.ListAllFeeds(r.Context(), 20, 0)
	s.render(w, r, "discover.html", map[string]any{
		"User":                 user,
		"FeedRecommendations":  feedRecs,
		"PeopleRecommendations": people,
		"PopularFeeds":         popular,
	})
}

func (s *Server) handleDiscoverFeeds(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	feedRecs, _ := s.db.GetFeedRecommendations(r.Context(), user.DID, 20)
	s.render(w, r, "discover.html", map[string]any{
		"User":                user,
		"FeedRecommendations": feedRecs,
	})
}

func (s *Server) handleDiscoverPeople(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	people, _ := s.db.GetPeopleRecommendations(r.Context(), user.DID, 20)
	s.render(w, r, "discover.html", map[string]any{
		"User":                  user,
		"PeopleRecommendations": people,
	})
}
