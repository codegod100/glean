package feed

import (
	"bytes"
	"encoding/xml"
	"io"
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
	Text     string    `xml:"text,attr"`
	Title    string    `xml:"title,attr"`
	XMLURL   string    `xml:"xmlUrl,attr"`
	HTMLURL  string    `xml:"htmlUrl,attr"`
	Outlines []Outline `xml:"outline"`
}

type FeedURL struct {
	URL      string
	Title    string
	Category string
}

func ParseOPML(r io.Reader) (*OPML, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var opml OPML
	if err := xml.Unmarshal(data, &opml); err != nil {
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
		if o.XMLURL != "" {
			*urls = append(*urls, FeedURL{
				URL:      o.XMLURL,
				Title:    o.Title,
				Category: category,
			})
		}
		childCategory := o.Text
		if childCategory == "" {
			childCategory = category
		}
		extractOutlines(o.Outlines, childCategory, urls)
	}
}

func GenerateOPML(feeds []FeedURL, title string) ([]byte, error) {
	opml := OPML{
		Version: "2.0",
	}
	opml.Head.Title = title

	for _, f := range feeds {
		opml.Body.Outlines = append(opml.Body.Outlines, Outline{
			Text:   f.Title,
			Title:  f.Title,
			XMLURL: f.URL,
		})
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
