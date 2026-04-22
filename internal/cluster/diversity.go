package cluster

import (
	"net/url"
	"strings"
)

const maxPerDomain = 2
const maxPerCategory = 3

func ApplyDiversity(candidates []*FeedRecommendation, topN int) []*FeedRecommendation {
	domainCount := make(map[string]int, len(candidates))
	categoryCount := make(map[string]int)
	result := make([]*FeedRecommendation, 0, topN)

	for _, c := range candidates {
		if len(result) >= topN {
			break
		}

		domain := extractDomain(c.SiteURL)
		if domain != "" && domainCount[domain] >= maxPerDomain {
			continue
		}

		cat := extractCategory(c.Description)
		if cat != "" && categoryCount[cat] >= maxPerCategory {
			continue
		}

		if domain != "" {
			domainCount[domain]++
		}
		if cat != "" {
			categoryCount[cat]++
		}
		result = append(result, c)
	}

	return result
}

func extractDomain(siteURL string) string {
	if siteURL == "" {
		return ""
	}
	u, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return host
}

func extractCategory(description string) string {
	if description == "" {
		return ""
	}
	words := strings.Fields(strings.ToLower(description))
	if len(words) == 0 {
		return ""
	}
	return words[0]
}
