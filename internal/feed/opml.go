package feed

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    struct {
		Title string `xml:"title"`
	} `xml:"head"`
	Body struct {
		Outlines []Outline `xml:"outline"`
	} `xml:"body"`
}

type Outline struct {
	Text        string    `xml:"text,attr"`
	Title       string    `xml:"title,attr"`
	XMLURL      string    `xml:"xmlUrl,attr"`
	HTMLURL     string    `xml:"htmlUrl,attr"`
	Description string    `xml:"description,attr"`
	Outlines    []Outline `xml:"outline"`
}

func (o Outline) GetTitle() string {
	if o.Title != "" {
		return o.Title
	}
	if o.Text != "" {
		return o.Text
	}
	if o.HTMLURL != "" {
		return o.HTMLURL
	}
	if o.XMLURL != "" {
		return o.XMLURL
	}
	return ""
}

func (o Outline) GetSiteURL() string {
	if o.HTMLURL != "" {
		return o.HTMLURL
	}
	return o.XMLURL
}

func (o Outline) IsSubscription() bool {
	return strings.TrimSpace(o.XMLURL) != ""
}

func (o Outline) HasChildren() bool {
	return len(o.Outlines) > 0
}

type FeedURL struct {
	URL         string
	Title       string
	SiteURL     string
	Description string
	Category    string
}

func ParseOPML(r io.Reader) (*OPML, error) {
	var opml OPML
	dec := xml.NewDecoder(r)
	dec.Strict = false
	if err := dec.Decode(&opml); err != nil {
		return nil, err
	}
	return &opml, nil
}

func ExtractFeedURLs(opml *OPML) []FeedURL {
	var urls []FeedURL
	extractOutlines(opml.Body.Outlines, "", &urls)
	return urls
}

func extractOutlines(outlines []Outline, category string, urls *[]FeedURL) {
	for _, o := range outlines {
		if o.IsSubscription() {
			*urls = append(*urls, FeedURL{
				URL:         o.XMLURL,
				Title:       o.GetTitle(),
				SiteURL:     o.GetSiteURL(),
				Description: o.Description,
				Category:    category,
			})
		} else if o.HasChildren() {
			extractOutlines(o.Outlines, o.GetTitle(), urls)
		}
	}
}

func GenerateOPML(feeds []FeedURL, title string) ([]byte, error) {
	opml := OPML{
		Version: "2.0",
	}
	opml.Head.Title = title

	for _, f := range feeds {
		outline := Outline{
			Text:    f.Title,
			Title:   f.Title,
			XMLURL:  f.URL,
			HTMLURL: f.SiteURL,
		}
		opml.Body.Outlines = append(opml.Body.Outlines, outline)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)

	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(opml); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
