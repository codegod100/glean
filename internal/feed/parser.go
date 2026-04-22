package feed

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	htmlcharset "golang.org/x/net/html/charset"
)

type Feed struct {
	URL          string
	Title        string
	SiteURL      string
	Description  string
	Type         string
	FaviconURL   string
	ETag         string
	LastModified string
}

type Article struct {
	FeedURL   string
	GUID      string
	Title     string
	URL       string
	Author    string
	Content   string
	Summary   string
	Published time.Time
	Updated   time.Time
}

type ParseResult struct {
	Feed     Feed
	Articles []Article
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Image       struct {
			URL string `xml:"url"`
		} `xml:"image"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			GUID        string `xml:"guid"`
			Description string `xml:"description"`
			Content     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
			Author      string `xml:"author"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomFeed struct {
	XMLName  xml.Name   `xml:"feed"`
	Title    string     `xml:"title"`
	Link     []atomLink `xml:"link"`
	Icon     string     `xml:"icon"`
	Logo     string     `xml:"logo"`
	Subtitle string     `xml:"subtitle"`
	Entry    []struct {
		Title   string     `xml:"title"`
		Link    []atomLink `xml:"link"`
		ID      string     `xml:"id"`
		Summary string     `xml:"summary"`
		Content string     `xml:"content"`
		Author  struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Published string `xml:"published"`
		Updated   string `xml:"updated"`
	} `xml:"entry"`
}

type rdfFeed struct {
	XMLName xml.Name `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# RDF"`
	Channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
	} `xml:"channel"`
	Items []struct {
		About       string `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# about,attr"`
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Content     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
		Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
		Date        string `xml:"http://purl.org/dc/elements/1.1/ date"`
	} `xml:"item"`
}

type jsonFeed struct {
	Version     string `json:"version"`
	Title       string `json:"title"`
	HomePageURL string `json:"home_page_url"`
	Description string `json:"description"`
	Favicon     string `json:"favicon"`
	Items       []struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		ContentHTML string `json:"content_html"`
		ContentText string `json:"content_text"`
		Summary     string `json:"summary"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
		DatePublished string `json:"date_published"`
		DateModified  string `json:"date_modified"`
	} `json:"items"`
}

func Parse(r io.Reader, feedURL string) (*ParseResult, error) {
	data, err := io.ReadAll(io.LimitReader(r, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading feed data: %w", err)
	}

	trimmed := strings.TrimLeft(string(data), " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return parseJSONFeed(data, feedURL)
	}

	return parseXMLFeed(data, feedURL)
}

func parseJSONFeed(data []byte, feedURL string) (*ParseResult, error) {
	var jf jsonFeed
	if err := json.Unmarshal(data, &jf); err != nil {
		return nil, fmt.Errorf("parsing JSON feed: %w", err)
	}

	result := &ParseResult{
		Feed: Feed{
			URL:         feedURL,
			Title:       jf.Title,
			SiteURL:     jf.HomePageURL,
			Description: jf.Description,
			FaviconURL:  jf.Favicon,
			Type:        "json",
		},
	}

	for _, item := range jf.Items {
		content := item.ContentHTML
		if content == "" {
			content = item.ContentText
		}

		article := Article{
			GUID:      item.ID,
			Title:     item.Title,
			URL:       item.URL,
			Content:   content,
			Summary:   item.Summary,
			Author:    item.Author.Name,
			Published: parseTime(item.DatePublished),
			Updated:   parseTime(item.DateModified),
		}
		if article.GUID == "" {
			article.GUID = article.URL
		}
		result.Articles = append(result.Articles, article)
	}

	return result, nil
}

func makeXMLDecoder(data []byte) *xml.Decoder {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	decoder.Strict = false
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return htmlcharset.NewReader(input, "text/xml; charset="+charset)
	}
	return decoder
}

func parseXMLFeed(data []byte, feedURL string) (*ParseResult, error) {
	var rss rssFeed
	if err := makeXMLDecoder(data).Decode(&rss); err == nil {
		if rss.XMLName.Local == "rss" {
			return convertRSS(&rss, feedURL), nil
		}
	}

	var atom atomFeed
	if err := makeXMLDecoder(data).Decode(&atom); err == nil {
		if atom.XMLName.Local == "feed" {
			return convertAtom(&atom, feedURL), nil
		}
	}

	var rdf rdfFeed
	if err := makeXMLDecoder(data).Decode(&rdf); err == nil {
		if rdf.XMLName.Local == "RDF" {
			return convertRDF(&rdf, feedURL), nil
		}
	}

	return nil, fmt.Errorf("unable to detect feed format")
}

func convertRSS(rss *rssFeed, feedURL string) *ParseResult {
	result := &ParseResult{
		Feed: Feed{
			URL:         feedURL,
			Title:       rss.Channel.Title,
			SiteURL:     rss.Channel.Link,
			Description: rss.Channel.Description,
			FaviconURL:  rss.Channel.Image.URL,
			Type:        "rss",
		},
	}

	for _, item := range rss.Channel.Items {
		article := Article{
			GUID:      item.GUID,
			Title:     item.Title,
			URL:       item.Link,
			Content:   item.Content,
			Summary:   item.Description,
			Author:    item.Author,
			Published: parseTime(item.PubDate),
		}
		if article.GUID == "" {
			article.GUID = article.URL
		}
		result.Articles = append(result.Articles, article)
	}

	return result
}

func convertRDF(rdf *rdfFeed, feedURL string) *ParseResult {
	result := &ParseResult{
		Feed: Feed{
			URL:         feedURL,
			Title:       rdf.Channel.Title,
			SiteURL:     rdf.Channel.Link,
			Description: rdf.Channel.Description,
			Type:        "rdf",
		},
	}

	for _, item := range rdf.Items {
		guid := item.About
		if guid == "" {
			guid = item.Link
		}
		article := Article{
			GUID:      guid,
			Title:     item.Title,
			URL:       item.Link,
			Content:   item.Content,
			Summary:   item.Description,
			Author:    item.Creator,
			Published: parseTime(item.Date),
		}
		if article.GUID == "" {
			article.GUID = article.URL
		}
		result.Articles = append(result.Articles, article)
	}

	return result
}

func convertAtom(atom *atomFeed, feedURL string) *ParseResult {
	favicon := atom.Icon
	if favicon == "" {
		favicon = atom.Logo
	}

	result := &ParseResult{
		Feed: Feed{
			URL:         feedURL,
			Title:       atom.Title,
			SiteURL:     pickAtomLink(atom.Link),
			Description: atom.Subtitle,
			FaviconURL:  favicon,
			Type:        "atom",
		},
	}

	for _, entry := range atom.Entry {
		article := Article{
			GUID:      entry.ID,
			Title:     entry.Title,
			URL:       pickAtomLink(entry.Link),
			Content:   entry.Content,
			Summary:   entry.Summary,
			Author:    entry.Author.Name,
			Published: parseTime(entry.Published),
			Updated:   parseTime(entry.Updated),
		}
		if article.GUID == "" {
			article.GUID = article.URL
		}
		result.Articles = append(result.Articles, article)
	}

	return result
}

func pickAtomLink(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "alternate" || l.Rel == "" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	formats := []string{
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		time.RFC1123,
		time.RFC1123Z,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}

	return time.Time{}
}
