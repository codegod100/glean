package sanitize

import (
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

func HTML(input string) string {
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
	return strings.TrimSpace(s)
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

func PlainText(input string) string {
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
