package server

import (
	"strings"
	"testing"
)

func TestSanitizeHTML_RemovesScriptTags(t *testing.T) {
	input := `<p>Hello</p><script>alert('xss')</script><p>World</p>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<script") {
		t.Fatalf("script tag not removed: %s", got)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Fatalf("content removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesEvilIframeTags(t *testing.T) {
	input := `<p>Hello</p><iframe src="evil.com"></iframe>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<iframe") {
		t.Fatalf("iframe tag not removed: %s", got)
	}
}

func TestSanitizeHTML_PreservesYouTubeIframe(t *testing.T) {
	input := `<iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ" width="560" height="315"></iframe>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("youtube iframe removed: %s", got)
	}
	if !strings.Contains(got, "youtube.com") {
		t.Fatalf("youtube iframe src lost: %s", got)
	}
}

func TestSanitizeHTML_PreservesVimeoIframe(t *testing.T) {
	input := `<iframe src="https://player.vimeo.com/video/12345" width="640" height="360"></iframe>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("vimeo iframe removed: %s", got)
	}
}

func TestSanitizeHTML_PreservesSpotifyIframe(t *testing.T) {
	input := `<iframe src="https://open.spotify.com/embed/track/abc123" width="300" height="80"></iframe>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("spotify iframe removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesEventHandler(t *testing.T) {
	input := `<div onclick="alert('xss')">Hello</div>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "onclick") {
		t.Fatalf("onclick handler not removed: %s", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Fatalf("content removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesJavascriptHref(t *testing.T) {
	input := `<a href="javascript:alert('xss')">click</a>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "javascript:") {
		t.Fatalf("javascript: href not removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesObjectTags(t *testing.T) {
	input := `<object data="evil.swf"></object>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<object") {
		t.Fatalf("object tag not removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesFormTags(t *testing.T) {
	input := `<form action="evil.com"><input type="submit"></form>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<form") {
		t.Fatalf("form tag not removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesMetaTags(t *testing.T) {
	input := `<meta http-equiv="refresh" content="0;url=evil.com">`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<meta") {
		t.Fatalf("meta tag not removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesBaseTags(t *testing.T) {
	input := `<base href="evil.com">`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<base") {
		t.Fatalf("base tag not removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesStyleExpression(t *testing.T) {
	input := `<div style="background: expression(alert('xss'))">Hello</div>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "expression") {
		t.Fatalf("expression not removed: %s", got)
	}
}

func TestSanitizeHTML_PreservesSafeContent(t *testing.T) {
	input := `<h1>Title</h1><p>Paragraph with <strong>bold</strong> and <em>italic</em>.</p><ul><li>item</li></ul>`
	got := sanitizeHTML(input)
	if got != input {
		t.Fatalf("safe content modified:\ngot:  %s\nwant: %s", got, input)
	}
}

func TestSanitizeHTML_PreservesImages(t *testing.T) {
	input := `<img src="photo.jpg" alt="photo">`
	got := sanitizeHTML(input)
	if got != input {
		t.Fatalf("img tag modified: %s", got)
	}
}

func TestSanitizeHTML_PreservesLinks(t *testing.T) {
	input := `<a href="https://example.com">link</a>`
	got := sanitizeHTML(input)
	if got != input {
		t.Fatalf("link modified: %s", got)
	}
}

func TestSanitizeHTML_HandlesCaseInsensitiveScript(t *testing.T) {
	input := `<SCRIPT>alert('xss')</SCRIPT>`
	got := sanitizeHTML(input)
	if strings.Contains(got, "<SCRIPT") {
		t.Fatalf("case-insensitive script not removed: %s", got)
	}
}

func TestSanitizeHTML_HandlesMultilineScript(t *testing.T) {
	input := "<script>\nalert('xss');\n</script>"
	got := sanitizeHTML(input)
	if strings.Contains(got, "<script") {
		t.Fatalf("multiline script not removed: %s", got)
	}
}

func TestSanitizeHTML_RemovesOnEventHandlers(t *testing.T) {
	cases := []string{
		`<div onmouseover="alert(1)">`,
		`<img onerror="alert(1)" src="x">`,
		`<body onload="alert(1)">`,
	}
	for _, input := range cases {
		got := sanitizeHTML(input)
		if strings.Contains(got, " on") {
			t.Fatalf("event handler not removed from %q: %s", input, got)
		}
	}
}

func TestSanitizeHTML_ConvertsYouTubeLink(t *testing.T) {
	input := `<p>Check this out:</p><a href="https://www.youtube.com/watch?v=dQw4w9WgXcQ">Watch on YouTube</a>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"`) {
		t.Fatalf("youtube link not converted to iframe: %s", got)
	}
	if strings.Contains(got, "<a href") && strings.Contains(got, "youtube.com/watch") {
		t.Fatalf("original youtube link not replaced: %s", got)
	}
}

func TestSanitizeHTML_ConvertsYoutuBeLink(t *testing.T) {
	input := `<a href="https://youtu.be/dQw4w9WgXcQ">Watch</a>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"`) {
		t.Fatalf("youtu.be link not converted to iframe: %s", got)
	}
}

func TestSanitizeHTML_ConvertsYouTubeShortsLink(t *testing.T) {
	input := `<a href="https://www.youtube.com/shorts/abc12345678">Short</a>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/abc12345678"`) {
		t.Fatalf("youtube shorts link not converted to iframe: %s", got)
	}
}

func TestSanitizeHTML_ConvertsVimeoLink(t *testing.T) {
	input := `<a href="https://vimeo.com/123456789">Watch on Vimeo</a>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, `<iframe src="https://player.vimeo.com/video/123456789"`) {
		t.Fatalf("vimeo link not converted to iframe: %s", got)
	}
}

func TestSanitizeHTML_PreservesNonMediaLinks(t *testing.T) {
	input := `<a href="https://example.com/article">Read more</a>`
	got := sanitizeHTML(input)
	if !strings.Contains(got, `<a href="https://example.com/article">Read more</a>`) {
		t.Fatalf("non-media link was modified: %s", got)
	}
}

func TestSanitizeHTMLWithBase_ResolvesRelativeImgSrc(t *testing.T) {
	input := `<img src="photo.jpg" alt="photo">`
	got := sanitizeHTMLWithBase(input, "https://example.com/posts/123")
	want := `<img src="https://example.com/posts/photo.jpg" alt="photo">`
	if got != want {
		t.Fatalf("relative img src not resolved:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSanitizeHTMLWithBase_ResolvesRelativeVideoSource(t *testing.T) {
	input := `<video><source src="clip.mp4" type="video/mp4"></video>`
	got := sanitizeHTMLWithBase(input, "https://example.com/a/b")
	want := `<video><source src="https://example.com/a/clip.mp4" type="video/mp4"></video>`
	if got != want {
		t.Fatalf("relative source src not resolved:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSanitizeHTMLWithBase_PreservesAbsoluteHttpURL(t *testing.T) {
	input := `<img src="https://cdn.example.com/photo.jpg" alt="photo">`
	got := sanitizeHTMLWithBase(input, "https://example.com/post")
	if got != input {
		t.Fatalf("absolute http url modified:\ngot:  %s\nwant: %s", got, input)
	}
}

func TestSanitizeHTMLWithBase_PreservesDataURI(t *testing.T) {
	input := `<img src="data:image/png;base64,iVBORw0KGgo=">`
	got := sanitizeHTMLWithBase(input, "https://example.com/post")
	if got != input {
		t.Fatalf("data uri modified:\ngot:  %s\nwant: %s", got, input)
	}
}

func TestSanitizeHTMLWithBase_PreservesAnchor(t *testing.T) {
	input := `<a href="#section">jump</a>`
	got := sanitizeHTMLWithBase(input, "https://example.com/post")
	if got != input {
		t.Fatalf("anchor modified:\ngot:  %s\nwant: %s", got, input)
	}
}

func TestSanitizeHTMLWithBase_ResolvesRootRelative(t *testing.T) {
	input := `<img src="/assets/photo.jpg">`
	got := sanitizeHTMLWithBase(input, "https://example.com/posts/123")
	want := `<img src="https://example.com/assets/photo.jpg">`
	if got != want {
		t.Fatalf("root-relative src not resolved:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSanitizeHTMLWithBase_NoBasePreservesRelative(t *testing.T) {
	// Without a base URL, relative URLs are left untouched (legacy behavior).
	input := `<img src="photo.jpg" alt="photo">`
	got := sanitizeHTML(input)
	if got != input {
		t.Fatalf("relative img src modified without base:\ngot:  %s\nwant: %s", got, input)
	}
}
