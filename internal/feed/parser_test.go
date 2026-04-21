package feed

import (
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestParse_RSS1(t *testing.T) {
	rdf := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dc="http://purl.org/dc/elements/1.1/">
 <channel>
  <title>Test Feed</title>
  <link>https://example.com</link>
  <description>A test feed</description>
 </channel>
 <item rdf:about="https://example.com/1">
  <title>First Item</title>
  <link>https://example.com/1</link>
  <dc:date>2026-04-21T10:00:00+09:00</dc:date>
  <dc:creator>Alice</dc:creator>
  <description>Description of first item</description>
 </item>
 <item rdf:about="https://example.com/2">
  <title>Second Item</title>
  <link>https://example.com/2</link>
  <dc:date>2026-04-20T08:00:00Z</dc:date>
  <description>Description of second item</description>
 </item>
</rdf:RDF>`

	result, err := Parse(strings.NewReader(rdf), "https://example.com/feed.rdf")
	assert.NilError(t, err)

	assert.Equal(t, result.Feed.URL, "https://example.com/feed.rdf")
	assert.Equal(t, result.Feed.Title, "Test Feed")
	assert.Equal(t, result.Feed.SiteURL, "https://example.com")
	assert.Equal(t, result.Feed.Description, "A test feed")
	assert.Equal(t, result.Feed.Type, "rdf")
	assert.Equal(t, len(result.Articles), 2)

	assert.Equal(t, result.Articles[0].Title, "First Item")
	assert.Equal(t, result.Articles[0].URL, "https://example.com/1")
	assert.Equal(t, result.Articles[0].GUID, "https://example.com/1")
	assert.Equal(t, result.Articles[0].Author, "Alice")
	assert.Equal(t, result.Articles[0].Summary, "Description of first item")
	assert.Assert(t, !result.Articles[0].Published.IsZero())

	assert.Equal(t, result.Articles[1].Title, "Second Item")
	assert.Equal(t, result.Articles[1].GUID, "https://example.com/2")
	assert.Equal(t, result.Articles[1].Author, "")
}

func TestParse_RSS1_FallsBackToLinkForGUID(t *testing.T) {
	rdf := `<?xml version="1.0"?>
<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
 <channel>
  <title>T</title>
  <link>https://example.com</link>
 </channel>
 <item>
  <title>No about attr</title>
  <link>https://example.com/noabout</link>
 </item>
</rdf:RDF>`

	result, err := Parse(strings.NewReader(rdf), "https://example.com/feed.rdf")
	assert.NilError(t, err)
	assert.Equal(t, len(result.Articles), 1)
	assert.Equal(t, result.Articles[0].GUID, "https://example.com/noabout")
}

func TestParse_RSS1_EmptyFeed(t *testing.T) {
	rdf := `<?xml version="1.0"?>
<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
 <channel>
  <title>Empty</title>
  <link>https://example.com</link>
 </channel>
</rdf:RDF>`

	result, err := Parse(strings.NewReader(rdf), "https://example.com/feed.rdf")
	assert.NilError(t, err)
	assert.Equal(t, result.Feed.Title, "Empty")
	assert.Equal(t, len(result.Articles), 0)
}

func TestParse_RSS1_DateParsing(t *testing.T) {
	rdf := `<?xml version="1.0"?>
<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dc="http://purl.org/dc/elements/1.1/">
 <channel>
  <title>T</title>
  <link>https://example.com</link>
 </channel>
 <item rdf:about="https://example.com/1">
  <title>T</title>
  <link>https://example.com/1</link>
  <dc:date>2026-04-21T10:00:00+09:00</dc:date>
 </item>
</rdf:RDF>`

	result, err := Parse(strings.NewReader(rdf), "https://example.com/feed.rdf")
	assert.NilError(t, err)

	expected, _ := time.Parse(time.RFC3339, "2026-04-21T01:00:00Z")
	assert.Equal(t, result.Articles[0].Published.UTC(), expected.UTC())
}

func TestParse_RSS1_WithContentEncoded(t *testing.T) {
	rdf := `<?xml version="1.0"?>
<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:dc="http://purl.org/dc/elements/1.1/">
 <channel>
  <title>T</title>
  <link>https://example.com</link>
 </channel>
 <item rdf:about="https://example.com/1">
  <title>T</title>
  <link>https://example.com/1</link>
  <description>Summary text</description>
  <content:encoded><![CDATA[<p>Full content here</p>]]></content:encoded>
  <dc:date>2026-04-21T10:00:00Z</dc:date>
 </item>
</rdf:RDF>`

	result, err := Parse(strings.NewReader(rdf), "https://example.com/feed.rdf")
	assert.NilError(t, err)
	assert.Equal(t, result.Articles[0].Content, "<p>Full content here</p>")
	assert.Equal(t, result.Articles[0].Summary, "Summary text")
}

func TestParse_RSS2StillWorks(t *testing.T) {
	rss := `<?xml version="1.0"?>
<rss version="2.0">
 <channel>
  <title>RSS2 Feed</title>
  <link>https://example.com</link>
  <description>A test</description>
  <item>
   <title>Item</title>
   <link>https://example.com/1</link>
   <guid>https://example.com/1</guid>
   <pubDate>Mon, 21 Apr 2026 10:00:00 UTC</pubDate>
  </item>
 </channel>
</rss>`

	result, err := Parse(strings.NewReader(rss), "https://example.com/feed.xml")
	assert.NilError(t, err)
	assert.Equal(t, result.Feed.Type, "rss")
	assert.Equal(t, len(result.Articles), 1)
}

func TestParse_AtomStillWorks(t *testing.T) {
	atom := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
 <title>Atom Feed</title>
 <link href="https://example.com" rel="alternate"/>
 <entry>
  <title>Entry</title>
  <link href="https://example.com/1" rel="alternate"/>
  <id>https://example.com/1</id>
  <updated>2026-04-21T10:00:00Z</updated>
 </entry>
</feed>`

	result, err := Parse(strings.NewReader(atom), "https://example.com/feed.atom")
	assert.NilError(t, err)
	assert.Equal(t, result.Feed.Type, "atom")
	assert.Equal(t, len(result.Articles), 1)
}
