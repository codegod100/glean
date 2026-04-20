package sanitize

import (
	"strings"
	"testing"
)

func TestHTML_RemovesScriptTags(t *testing.T) {
	input := `<p>Hello</p><script>alert('xss')</script><p>World</p>`
	got := HTML(input)
	if strings.Contains(got, "<script") {
		t.Fatalf("script tag not removed: %s", got)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Fatalf("content removed: %s", got)
	}
}

func TestHTML_RemovesEvilIframeTags(t *testing.T) {
	input := `<p>Hello</p><iframe src="evil.com"></iframe>`
	got := HTML(input)
	if strings.Contains(got, "<iframe") {
		t.Fatalf("iframe tag not removed: %s", got)
	}
}

func TestHTML_PreservesYouTubeIframe(t *testing.T) {
	input := `<iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ" width="560" height="315"></iframe>`
	got := HTML(input)
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("youtube iframe removed: %s", got)
	}
	if !strings.Contains(got, "youtube.com") {
		t.Fatalf("youtube iframe src lost: %s", got)
	}
}

func TestHTML_PreservesVimeoIframe(t *testing.T) {
	input := `<iframe src="https://player.vimeo.com/video/12345" width="640" height="360"></iframe>`
	got := HTML(input)
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("vimeo iframe removed: %s", got)
	}
}

func TestHTML_PreservesSpotifyIframe(t *testing.T) {
	input := `<iframe src="https://open.spotify.com/embed/track/abc123" width="300" height="80"></iframe>`
	got := HTML(input)
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("spotify iframe removed: %s", got)
	}
}

func TestHTML_RemovesEventHandler(t *testing.T) {
	input := `<div onclick="alert('xss')">Hello</div>`
	got := HTML(input)
	if strings.Contains(got, "onclick") {
		t.Fatalf("onclick handler not removed: %s", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Fatalf("content removed: %s", got)
	}
}

func TestHTML_RemovesJavascriptHref(t *testing.T) {
	input := `<a href="javascript:alert('xss')">click</a>`
	got := HTML(input)
	if strings.Contains(got, "javascript:") {
		t.Fatalf("javascript: href not removed: %s", got)
	}
}

func TestHTML_RemovesObjectTags(t *testing.T) {
	input := `<object data="evil.swf"></object>`
	got := HTML(input)
	if strings.Contains(got, "<object") {
		t.Fatalf("object tag not removed: %s", got)
	}
}

func TestHTML_RemovesFormTags(t *testing.T) {
	input := `<form action="evil.com"><input type="submit"></form>`
	got := HTML(input)
	if strings.Contains(got, "<form") {
		t.Fatalf("form tag not removed: %s", got)
	}
}

func TestHTML_RemovesMetaTags(t *testing.T) {
	input := `<meta http-equiv="refresh" content="0;url=evil.com">`
	got := HTML(input)
	if strings.Contains(got, "<meta") {
		t.Fatalf("meta tag not removed: %s", got)
	}
}

func TestHTML_RemovesBaseTags(t *testing.T) {
	input := `<base href="evil.com">`
	got := HTML(input)
	if strings.Contains(got, "<base") {
		t.Fatalf("base tag not removed: %s", got)
	}
}

func TestHTML_RemovesStyleExpression(t *testing.T) {
	input := `<div style="background: expression(alert('xss'))">Hello</div>`
	got := HTML(input)
	if strings.Contains(got, "expression") {
		t.Fatalf("expression not removed: %s", got)
	}
}

func TestHTML_PreservesSafeContent(t *testing.T) {
	input := `<h1>Title</h1><p>Paragraph with <strong>bold</strong> and <em>italic</em>.</p><ul><li>item</li></ul>`
	got := HTML(input)
	if got != input {
		t.Fatalf("safe content modified:\ngot:  %s\nwant: %s", got, input)
	}
}

func TestHTML_PreservesImages(t *testing.T) {
	input := `<img src="photo.jpg" alt="photo">`
	got := HTML(input)
	if got != input {
		t.Fatalf("img tag modified: %s", got)
	}
}

func TestHTML_PreservesLinks(t *testing.T) {
	input := `<a href="https://example.com">link</a>`
	got := HTML(input)
	if got != input {
		t.Fatalf("link modified: %s", got)
	}
}

func TestHTML_HandlesCaseInsensitiveScript(t *testing.T) {
	input := `<SCRIPT>alert('xss')</SCRIPT>`
	got := HTML(input)
	if strings.Contains(got, "<SCRIPT") {
		t.Fatalf("case-insensitive script not removed: %s", got)
	}
}

func TestHTML_HandlesMultilineScript(t *testing.T) {
	input := "<script>\nalert('xss');\n</script>"
	got := HTML(input)
	if strings.Contains(got, "<script") {
		t.Fatalf("multiline script not removed: %s", got)
	}
}

func TestHTML_RemovesOnEventHandlers(t *testing.T) {
	cases := []string{
		`<div onmouseover="alert(1)">`,
		`<img onerror="alert(1)" src="x">`,
		`<body onload="alert(1)">`,
	}
	for _, input := range cases {
		got := HTML(input)
		if strings.Contains(got, " on") {
			t.Fatalf("event handler not removed from %q: %s", input, got)
		}
	}
}

func TestHTML_ConvertsYouTubeLink(t *testing.T) {
	input := `<p>Check this out:</p><a href="https://www.youtube.com/watch?v=dQw4w9WgXcQ">Watch on YouTube</a>`
	got := HTML(input)
	if !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"`) {
		t.Fatalf("youtube link not converted to iframe: %s", got)
	}
	if strings.Contains(got, "<a href") && strings.Contains(got, "youtube.com/watch") {
		t.Fatalf("original youtube link not replaced: %s", got)
	}
}

func TestHTML_ConvertsYoutuBeLink(t *testing.T) {
	input := `<a href="https://youtu.be/dQw4w9WgXcQ">Watch</a>`
	got := HTML(input)
	if !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"`) {
		t.Fatalf("youtu.be link not converted to iframe: %s", got)
	}
}

func TestHTML_ConvertsYouTubeShortsLink(t *testing.T) {
	input := `<a href="https://www.youtube.com/shorts/abc12345678">Short</a>`
	got := HTML(input)
	if !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/abc12345678"`) {
		t.Fatalf("youtube shorts link not converted to iframe: %s", got)
	}
}

func TestHTML_ConvertsVimeoLink(t *testing.T) {
	input := `<a href="https://vimeo.com/123456789">Watch on Vimeo</a>`
	got := HTML(input)
	if !strings.Contains(got, `<iframe src="https://player.vimeo.com/video/123456789"`) {
		t.Fatalf("vimeo link not converted to iframe: %s", got)
	}
}

func TestHTML_PreservesNonMediaLinks(t *testing.T) {
	input := `<a href="https://example.com/article">Read more</a>`
	got := HTML(input)
	if !strings.Contains(got, `<a href="https://example.com/article">Read more</a>`) {
		t.Fatalf("non-media link was modified: %s", got)
	}
}
