package server

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	tagRe       = regexp.MustCompile(`<[^>]+>`)
	entityRe    = regexp.MustCompile(`&[^;]+;`)
	scriptRe    = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	objectRe    = regexp.MustCompile(`(?is)<object[^>]*>.*?</object>`)
	appletRe    = regexp.MustCompile(`(?is)<applet[^>]*>.*?</applet>`)
	formRe      = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)
	metaRe      = regexp.MustCompile(`(?is)<meta[^>]*/?>`)
	baseRe      = regexp.MustCompile(`(?is)<base[^>]*/?>`)
	linkRe      = regexp.MustCompile(`(?is)<link[^>]*/?>`)
	onAttrRe    = regexp.MustCompile(`(?i)\s+on\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)
	jsHrefRe    = regexp.MustCompile(`(?i)href\s*=\s*"\s*javascript:[^"]*"`)
	jsHrefSRe   = regexp.MustCompile(`(?i)href\s*=\s*'\s*javascript:[^']*'`)
	styleExprRe = regexp.MustCompile(`(?i)expression\s*\(`)
	styleUrlRe  = regexp.MustCompile(`(?i)url\s*\(\s*javascript:`)
	iframeSrcRe = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
)

var (
	youtubeLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="https?://(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/shorts/|m\.youtube\.com/watch\?v=)([a-zA-Z0-9_-]{11})"[^>]*>.*?</a>`)
	youtuBeLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="https?://youtu\.be/([a-zA-Z0-9_-]{11})"[^>]*>.*?</a>`)
	vimeoLinkRe   = regexp.MustCompile(`(?i)<a[^>]+href="https?://(?:www\.)?vimeo\.com/(\d+)"[^>]*>.*?</a>`)
)

var allowedIframeHosts = []string{
	"www.youtube.com",
	"youtube.com",
	"youtu.be",
	"www.youtube-nocookie.com",
	"player.vimeo.com",
	"vimeo.com",
	"open.spotify.com",
	"embed.spotify.com",
	"w.soundcloud.com",
	"bandcamp.com",
}

func isAllowedIframe(tag string) bool {
	lower := strings.ToLower(tag)
	for _, host := range allowedIframeHosts {
		if strings.Contains(lower, host) {
			return true
		}
	}
	return false
}

func sanitizeHTML(input string) string {
	return sanitizeHTMLWithBase(input, "")
}

// sanitizeHTMLWithBase runs sanitizeHTML and additionally resolves relative
// URLs in url-bearing attributes (src, href, data, poster, srcset) against
// baseURL. An empty baseURL leaves relative URLs untouched. Non-http(s)
// schemes (data:, javascript:, mailto:, anchors, etc.) are preserved as-is.
func sanitizeHTMLWithBase(input, baseURL string) string {
	s := input
	s = scriptRe.ReplaceAllString(s, "")

	s = convertMediaLinks(s)

	s = filterIframes(s)

	s = objectRe.ReplaceAllString(s, "")
	s = appletRe.ReplaceAllString(s, "")
	s = formRe.ReplaceAllString(s, "")
	s = metaRe.ReplaceAllString(s, "")
	s = baseRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "")
	s = onAttrRe.ReplaceAllString(s, "")
	s = jsHrefRe.ReplaceAllString(s, `href="#"`)
	s = jsHrefSRe.ReplaceAllString(s, `href="#"`)
	s = styleExprRe.ReplaceAllString(s, "")
	s = styleUrlRe.ReplaceAllString(s, "")

	if baseURL != "" {
		if base, err := url.Parse(baseURL); err == nil && base.IsAbs() {
			s = resolveURLAttrs(s, base)
		}
	}

	return strings.TrimSpace(s)
}

// urlAttrRe matches a url-bearing attribute (src, href, data, poster) with
// its quoted value. Capture group 1 is the opening quote, group 2 the URL.
var urlAttrRe = regexp.MustCompile(`(?i)\b(src|href|data|poster)\s*=\s*("([^"]*)"|'([^']*)')`)

// srcsetAttrRe matches a srcset attribute value (double-quoted only for now).
var srcsetAttrRe = regexp.MustCompile(`(?i)\bsrcset\s*=\s*"([^"]*)"`)

// resolveURLAttrs rewrites relative URLs in url-bearing attributes to be
// absolute, resolved against base. Already-absolute URLs and non-http(s)
// schemes (data:, mailto:, #anchors) are left untouched.
func resolveURLAttrs(s string, base *url.URL) string {
	s = urlAttrRe.ReplaceAllStringFunc(s, func(match string) string {
		m := urlAttrRe.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		raw := m[3]
		if raw == "" {
			raw = m[4]
		}
		resolved, ok := resolveURL(raw, base)
		if !ok {
			return match
		}
		// Preserve the original quote style.
		if m[3] != "" {
			return m[1] + `="` + resolved + `"`
		}
		return m[1] + `='` + resolved + `'`
	})

	s = srcsetAttrRe.ReplaceAllStringFunc(s, func(match string) string {
		m := srcsetAttrRe.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		value := m[1]
		parts := strings.Split(value, ",")
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			fields := strings.Fields(p)
			if resolved, ok := resolveURL(fields[0], base); ok {
				fields[0] = resolved
				parts[i] = strings.Join(fields, " ")
			}
		}
		return `srcset="` + strings.Join(parts, ", ") + `"`
	})

	return s
}

// resolveURL resolves raw against base. Returns false when raw is already
// absolute with a non-http(s) scheme, or is empty, or is a fragment-only URL.
func resolveURL(raw string, base *url.URL) (string, bool) {
	if raw == "" || strings.HasPrefix(raw, "#") {
		return raw, false
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw, false
	}
	if ref.IsAbs() {
		// Keep http(s) as-is; preserve other schemes (data:, mailto:, etc.).
		return raw, ref.Scheme == "http" || ref.Scheme == "https"
	}
	return base.ResolveReference(ref).String(), true
}

func convertMediaLinks(s string) string {
	s = youtubeLinkRe.ReplaceAllString(s, `<iframe src="https://www.youtube-nocookie.com/embed/$1" allowfullscreen loading="lazy"></iframe>`)
	s = youtuBeLinkRe.ReplaceAllString(s, `<iframe src="https://www.youtube-nocookie.com/embed/$1" allowfullscreen loading="lazy"></iframe>`)
	s = vimeoLinkRe.ReplaceAllString(s, `<iframe src="https://player.vimeo.com/video/$1" allowfullscreen loading="lazy"></iframe>`)
	return s
}

func filterIframes(s string) string {
	return iframeSrcRe.ReplaceAllStringFunc(s, func(match string) string {
		if isAllowedIframe(match) {
			return match
		}
		return ""
	})
}

var htmlEntities = map[string]string{
	"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": `"`, "&#39;": "'",
	"&apos;": "'", "&nbsp;": " ", "&hellip;": "...", "&mdash;": "—",
	"&ndash;": "–", "&laquo;": "«", "&raquo;": "»", "&rsquo;": "'",
	"&lsquo;": "'", "&rdquo;": "\"", "&ldquo;": "\"",
}

func plainText(input string) string {
	s := tagRe.ReplaceAllString(input, " ")
	for entity, replacement := range htmlEntities {
		s = strings.ReplaceAll(s, entity, replacement)
	}
	s = entityRe.ReplaceAllStringFunc(s, func(e string) string {
		if strings.HasPrefix(e, "&#") {
			return ""
		}
		return e
	})
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
