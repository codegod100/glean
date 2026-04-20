package scraper

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestExtractContent_ArticleElement(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<nav>navigation</nav>
		<article><p>This is the article content with enough text to be meaningful and substantial for the reader to consume.</p></article>
		<footer>footer</footer>
	</body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "This is the article content"))
	assert.Assert(t, !strings.Contains(content, "navigation"))
	assert.Assert(t, !strings.Contains(content, "footer"))
}

func TestExtractContent_MainElement(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<nav>navigation</nav>
		<main><p>Main content area with sufficient text to be considered a proper article body for reading purposes.</p></main>
	</body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "Main content area"))
}

func TestExtractContent_RoleMain(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<div role="main"><p>Content in a role=main div with enough text to be useful for the reader to enjoy.</p></div>
	</body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "Content in a role=main div"))
}

func TestExtractContent_LargestDiv(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<div class="sidebar">small sidebar text</div>
		<div class="content">
			<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</p>
		</div>
	</body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "Lorem ipsum"))
}

func TestExtractContent_RemovesScripts(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<article>
			<p>Good content here that is long enough to pass the minimum threshold for content extraction logic.</p>
			<script>alert('xss')</script>
			<style>.foo { color: red; }</style>
		</article>
	</body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "Good content"))
	assert.Assert(t, !strings.Contains(content, "alert"))
	assert.Assert(t, !strings.Contains(content, "color: red"))
}

func TestExtractContent_EmptyBody(t *testing.T) {
	html := `<!DOCTYPE html><html><body></body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Equal(t, content, "")
}

func TestScrapeDirect_Integration(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<article><h1>Title</h1><p>Full article content that has enough substance to be extracted as the primary content of this webpage.</p></article>
		</body></html>`))
	}))
	defer ts.Close()

	s := New(slog.Default())
	content, err := s.scrapeDirect(t.Context(), ts.URL+"/article")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "Full article content"))
}

func TestScrapeDirect_NonHTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	s := New(slog.Default())
	_, err := s.scrapeDirect(t.Context(), ts.URL+"/doc.pdf")
	assert.Assert(t, err != nil)
}

func TestScrape_FallsBackToArchive(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/paywall-article" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<article><p>Archived content successfully retrieved from the archive service for reading.</p></article>
		</body></html>`))
	}))
	defer ts.Close()

	s := New(slog.Default())
	s.archiveURL = ts.URL + "/archive?q="

	content, err := s.Scrape(t.Context(), ts.URL+"/paywall-article")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "Archived content"))
}

func TestRenderNode_VoidElements(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<article>
			<p>Text with <br>break and <img src="test.jpg"> image</p>
		</article>
	</body></html>`

	content, err := extractContent(strings.NewReader(html))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(content, "<br>"))
	assert.Assert(t, strings.Contains(content, "<img"))
	assert.Assert(t, !strings.Contains(content, "</br>"))
}
