package server

import (
	"encoding/xml"
	"net/http"
	"strings"
	"time"
)

type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc        string `xml:"loc"`
	Lastmod    string `xml:"lastmod,omitempty"`
	Changefreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

func (s *Server) handleSitemap(w http.ResponseWriter, _ *http.Request) {
	baseURL := s.baseURL()

	now := time.Now().UTC().Format("2006-01-02")

	urls := []sitemapURL{
		{Loc: baseURL + "/", Lastmod: now, Changefreq: "daily", Priority: "1.0"},
		{Loc: baseURL + "/trending", Lastmod: now, Changefreq: "hourly", Priority: "0.9"},
		{Loc: baseURL + "/terms", Changefreq: "monthly", Priority: "0.3"},
	}

	set := urlset{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(set)
}

func (s *Server) baseURL() string {
	if s.clientID == "" {
		return "http://localhost:8080"
	}
	host := s.clientID
	host, _ = strings.CutPrefix(host, "https://")
	host, _ = strings.CutPrefix(host, "http://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	return "https://" + host
}
