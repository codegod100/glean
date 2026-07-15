package feed

import (
	"strings"
	"testing"
)

func TestOutlineGetTitle(t *testing.T) {
	cases := []struct {
		name string
		o    Outline
		want string
	}{
		{"title wins", Outline{Title: "T", Text: "X"}, "T"},
		{"text fallback", Outline{Text: "X"}, "X"},
		{"htmlurl fallback", Outline{HTMLURL: "https://example.com"}, "https://example.com"},
		{"xmlurl fallback", Outline{XMLURL: "https://example.com/feed"}, "https://example.com/feed"},
		{"empty", Outline{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.o.GetTitle(); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestGenerateOPML(t *testing.T) {
	feeds := []FeedURL{
		{URL: "https://a.com/feed", Title: "A", SiteURL: "https://a.com"},
		{URL: "https://b.com/feed", Title: "B", Category: "News"},
	}
	out, err := GenerateOPML(feeds, "My Subscriptions")
	if err != nil {
		t.Fatalf("GenerateOPML: %v", err)
	}
	s := string(out)
	if !strings.HasPrefix(s, `<?xml`) {
		t.Fatalf("missing xml header: %s", s)
	}
	if !strings.Contains(s, "My Subscriptions") {
		t.Fatalf("missing title: %s", s)
	}
	if !strings.Contains(s, "https://a.com/feed") || !strings.Contains(s, "https://b.com/feed") {
		t.Fatalf("missing feed urls: %s", s)
	}
	if !strings.Contains(s, "News") {
		t.Fatalf("missing category: %s", s)
	}
}

func TestGenerateOPML_Empty(t *testing.T) {
	out, err := GenerateOPML(nil, "Empty")
	if err != nil {
		t.Fatalf("GenerateOPML: %v", err)
	}
	if !strings.Contains(string(out), "Empty") {
		t.Fatalf("missing title in empty opml: %s", out)
	}
}
