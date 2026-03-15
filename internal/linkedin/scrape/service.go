package scrape

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	reJSONLDScript = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	rePostAttrs    = regexp.MustCompile(`(?is)data-post-url=["']([^"']+)["'][^>]*data-post-text=["']([^"']*)["'][^>]*data-post-date=["']([^"']*)["']`)
)

type Service struct {
	httpClient *http.Client
	baseURL    string
}

func NewService(baseURL string) *Service {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = "https://www.linkedin.com"
	}
	trimmed = strings.TrimRight(trimmed, "/")

	return &Service{
		httpClient: &http.Client{Timeout: 12 * time.Second},
		baseURL:    trimmed,
	}
}

func (s *Service) ScrapeProfile(ctx context.Context, input ProfileInput) (ProfileResult, error) {
	path, fullURL, err := normalizeLinkedInPath(input.URL, true)
	if err != nil {
		return ProfileResult{}, err
	}

	body, err := s.fetch(ctx, path)
	if err != nil {
		return ProfileResult{}, err
	}

	nodes := extractJSONLDNodes(body)
	result := parseProfile(nodes)
	result.URL = fullURL

	if result.FullName == "" {
		return ProfileResult{}, errors.New("profile data not found")
	}

	if input.IncludePosts {
		result.Posts = collectPosts(body, nodes, input.Limit)
	}
	return result, nil
}

func (s *Service) ScrapeCompany(ctx context.Context, input CompanyInput) (CompanyResult, error) {
	path, fullURL, err := normalizeLinkedInPath(input.URL, false)
	if err != nil {
		return CompanyResult{}, err
	}

	body, err := s.fetch(ctx, path)
	if err != nil {
		return CompanyResult{}, err
	}

	nodes := extractJSONLDNodes(body)
	result := parseCompany(nodes)
	result.URL = fullURL

	if result.Name == "" {
		return CompanyResult{}, errors.New("company data not found")
	}

	if input.IncludePosts {
		result.Posts = collectPosts(body, nodes, input.Limit)
	}
	return result, nil
}

func normalizeLinkedInPath(raw string, expectProfile bool) (path string, fullURL string, err error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", errors.New("url is required")
	}

	if strings.Contains(value, "://") {
		parsed, parseErr := url.Parse(value)
		if parseErr != nil {
			return "", "", errors.New("invalid url")
		}
		value = parsed.Path
	}

	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, "/")
	if value == "" {
		return "", "", errors.New("invalid url path")
	}

	if expectProfile {
		if !strings.HasPrefix(value, "/in/") {
			return "", "", errors.New("profile url must use /in/ path")
		}
	} else {
		if !strings.HasPrefix(value, "/company/") && !strings.HasPrefix(value, "/school/") {
			return "", "", errors.New("company url must use /company/ or /school/ path")
		}
	}

	return value, "https://www.linkedin.com" + value, nil
}

func (s *Service) fetch(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "apitera-linkedin/1.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
	if err != nil {
		return "", fmt.Errorf("failed reading upstream body: %w", err)
	}
	return string(body), nil
}

func extractJSONLDNodes(page string) []map[string]any {
	blocks := reJSONLDScript.FindAllStringSubmatch(page, -1)
	if len(blocks) == 0 {
		return nil
	}

	nodes := make([]map[string]any, 0, len(blocks)*2)
	for _, block := range blocks {
		raw := strings.TrimSpace(block[1])
		if raw == "" {
			continue
		}
		raw = html.UnescapeString(raw)

		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			continue
		}
		collectMapNodes(decoded, &nodes)
	}
	return nodes
}

func collectMapNodes(value any, out *[]map[string]any) {
	switch v := value.(type) {
	case map[string]any:
		*out = append(*out, v)
		for _, inner := range v {
			collectMapNodes(inner, out)
		}
	case []any:
		for _, inner := range v {
			collectMapNodes(inner, out)
		}
	}
}

func parseProfile(nodes []map[string]any) ProfileResult {
	var result ProfileResult
	for _, node := range nodes {
		if !isType(node, "Person") {
			continue
		}
		result.FullName = asString(node["name"])
		result.Headline = firstNonEmpty(asString(node["jobTitle"]), asString(node["headline"]), asString(node["description"]))
		result.Location = extractLocation(node)
		result.About = asString(node["description"])
		result.ProfileImageURL = extractImage(node["image"])
		break
	}
	return result
}

func parseCompany(nodes []map[string]any) CompanyResult {
	var result CompanyResult
	for _, node := range nodes {
		if !isType(node, "Organization") {
			continue
		}
		result.Name = asString(node["name"])
		result.Industry = asString(node["industry"])
		result.Size = extractSize(node["numberOfEmployees"])
		result.Website = firstNonEmpty(asString(node["url"]), asString(node["sameAs"]))
		result.Headquarters = extractLocation(node)
		result.About = asString(node["description"])
		result.LogoURL = extractImage(node["logo"])
		break
	}
	return result
}

func collectPosts(page string, nodes []map[string]any, limit int) []Post {
	capLimit := normalizeLimit(limit)
	posts := make([]Post, 0, capLimit)
	seen := make(map[string]struct{}, capLimit)

	for _, node := range nodes {
		if !isType(node, "SocialMediaPosting") && !isType(node, "Article") && !isType(node, "BlogPosting") {
			continue
		}

		post := Post{
			Text:        firstNonEmpty(asString(node["articleBody"]), asString(node["description"]), asString(node["headline"]), asString(node["name"])),
			URL:         asString(node["url"]),
			PublishedAt: asString(node["datePublished"]),
		}
		if strings.TrimSpace(post.Text) == "" && strings.TrimSpace(post.URL) == "" {
			continue
		}
		key := post.URL + "|" + post.Text
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		posts = append(posts, post)
		if len(posts) >= capLimit {
			return posts
		}
	}

	matches := rePostAttrs.FindAllStringSubmatch(page, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		post := Post{
			URL:         html.UnescapeString(strings.TrimSpace(match[1])),
			Text:        html.UnescapeString(strings.TrimSpace(match[2])),
			PublishedAt: strings.TrimSpace(match[3]),
		}
		if post.URL == "" && post.Text == "" {
			continue
		}
		key := post.URL + "|" + post.Text
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		posts = append(posts, post)
		if len(posts) >= capLimit {
			return posts
		}
	}

	return posts
}

func isType(node map[string]any, want string) bool {
	raw, ok := node["@type"]
	if !ok {
		return false
	}
	wantLower := strings.ToLower(strings.TrimSpace(want))
	switch t := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(t)) == wantLower
	case []any:
		for _, item := range t {
			if strings.ToLower(strings.TrimSpace(asString(item))) == wantLower {
				return true
			}
		}
	}
	return false
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func extractLocation(node map[string]any) string {
	for _, key := range []string{"location", "homeLocation", "address"} {
		if value, ok := node[key]; ok {
			if text := asString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func extractSize(value any) string {
	size := asString(value)
	if size != "" {
		return size
	}

	if m, ok := value.(map[string]any); ok {
		min := asString(m["minValue"])
		max := asString(m["maxValue"])
		if min != "" && max != "" {
			return min + "-" + max
		}
	}
	return ""
}

func extractImage(value any) string {
	if text := asString(value); text != "" {
		return text
	}
	if m, ok := value.(map[string]any); ok {
		if text := asString(m["url"]); text != "" {
			return text
		}
	}
	return ""
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"name", "text", "url", "addressLocality", "addressRegion", "addressCountry"} {
			if text := asString(v[key]); text != "" {
				return text
			}
		}
		parts := make([]string, 0, 3)
		for _, key := range []string{"streetAddress", "addressLocality", "addressRegion", "postalCode", "addressCountry"} {
			if text := asString(v[key]); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	case []any:
		for _, item := range v {
			if text := asString(item); text != "" {
				return text
			}
		}
	case json.Number:
		return v.String()
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", v), "0"), ".")
	case int:
		return fmt.Sprintf("%d", v)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
