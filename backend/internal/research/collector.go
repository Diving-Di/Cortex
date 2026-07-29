package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	allowedHosts = map[string]bool{
		"xiaohongshu.com": true, "www.xiaohongshu.com": true,
		"xhslink.com": true, "www.xhslink.com": true,
	}
	noteLinkPattern = regexp.MustCompile(`https?://(?:www\.)?xiaohongshu\.com/(?:explore|discovery/item)/[A-Za-z0-9]+[^"'<> ]*`)
	metaPattern     = regexp.MustCompile(`(?is)<meta\s+[^>]*(?:property|name)=["']([^"']+)["'][^>]*content=["']([^"']*)["'][^>]*>`)
	metaReverse     = regexp.MustCompile(`(?is)<meta\s+[^>]*content=["']([^"']*)["'][^>]*(?:property|name)=["']([^"']+)["'][^>]*>`)
	tagPattern      = regexp.MustCompile(`#([\p{Han}A-Za-z0-9_-]{1,40})`)
)

type Collected struct {
	URL         string
	Title       string
	Author      string
	Content     string
	Tags        []string
	ImageURLs   []string
	PublishedAt *time.Time
}

type Collector struct {
	Client          *http.Client
	MaxBodyBytes    int64
	MaxBodyChars    int
	MaxImages       int
	RequestInterval time.Duration
	CookieHeader    string
}

func NormalizeURL(raw string) (string, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Scheme != "https" || value.User != nil || !allowedHosts[strings.ToLower(value.Hostname())] {
		return "", errors.New("RESEARCH_INVALID_URL")
	}
	value.Fragment = ""
	query := value.Query()
	kept := url.Values{}
	for _, key := range []string{"xsec_token", "xsec_source"} {
		if item := strings.TrimSpace(query.Get(key)); item != "" {
			kept.Set(key, item)
		}
	}
	value.RawQuery = kept.Encode()
	value.Host = strings.ToLower(value.Hostname())
	if value.Host == "xiaohongshu.com" {
		value.Host = "www.xiaohongshu.com"
	}
	value.Path = strings.TrimRight(value.EscapedPath(), "/")
	if value.Path == "" {
		value.Path = "/"
	}
	return value.String(), nil
}

func ValidateDestination(ctx context.Context, hostname string, resolver *net.Resolver) error {
	host := strings.ToLower(strings.TrimSuffix(hostname, "."))
	if !allowedHosts[host] {
		return errors.New("RESEARCH_INVALID_URL")
	}
	return ValidatePublicDestination(ctx, host, resolver)
}

func ValidatePublicDestination(ctx context.Context, hostname string, resolver *net.Resolver) error {
	host := strings.ToLower(strings.TrimSuffix(hostname, "."))
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return errors.New("RESEARCH_INVALID_URL")
		}
	}
	return nil
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isPublicIP(value net.IP) bool {
	address, ok := netip.AddrFromSlice(value)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Environment proxies can turn an allow-listed URL into a request to an
	// unvalidated internal hop, so the collector never inherits proxy settings.
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid destination")
		}
		if err := ValidatePublicDestination(ctx, host, nil); err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("XHS_SOURCE_UNAVAILABLE")
		}
		for _, resolved := range addresses {
			if !isPublicIP(resolved.IP) {
				return nil, errors.New("RESEARCH_INVALID_URL")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
	client := &http.Client{Timeout: timeout, Transport: transport}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if request.URL.Scheme != "https" || !allowedHosts[strings.ToLower(request.URL.Hostname())] {
			return errors.New("unsafe redirect")
		}
		return ValidateDestination(request.Context(), request.URL.Hostname(), nil)
	}
	return client
}

func (c Collector) Search(ctx context.Context, keyword string, count int) ([]string, error) {
	searchURL := "https://www.xiaohongshu.com/search_result?keyword=" + url.QueryEscape(keyword) + "&source=web_search_result_notes"
	body, finalURL, err := c.fetch(ctx, searchURL)
	if err != nil {
		return nil, err
	}
	_ = finalURL
	matches := noteLinkPattern.FindAllString(string(body), count*3)
	seen := map[string]bool{}
	var result []string
	for _, match := range matches {
		normalized, err := NormalizeURL(html.UnescapeString(match))
		if err == nil && !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
			if len(result) >= count {
				break
			}
		}
	}
	if len(result) == 0 {
		if strings.Contains(string(body), "登录") || strings.Contains(strings.ToLower(string(body)), "login") {
			return nil, errors.New("XHS_AUTH_REQUIRED")
		}
		return nil, errors.New("XHS_LAYOUT_CHANGED")
	}
	return result, nil
}

func (c Collector) Collect(ctx context.Context, rawURL string) (Collected, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return Collected{}, err
	}
	body, finalURL, err := c.fetch(ctx, normalized)
	if err != nil {
		return Collected{}, err
	}
	metadata := parseMetadata(string(body))
	title := first(metadata["og:title"], metadata["twitter:title"])
	content := first(metadata["og:description"], metadata["description"])
	author := first(metadata["author"], metadata["og:article:author"])
	if strings.TrimSpace(title) == "" && strings.TrimSpace(content) == "" {
		if strings.Contains(string(body), "登录") {
			return Collected{}, errors.New("XHS_AUTH_REQUIRED")
		}
		return Collected{}, errors.New("XHS_LAYOUT_CHANGED")
	}
	images := metadataValues(metadata, "og:image", c.MaxImages)
	tags := uniqueMatches(tagPattern, content, 20)
	if c.MaxBodyChars > 0 {
		content = truncate(content, c.MaxBodyChars)
	}
	return Collected{
		URL: finalURL, Title: truncate(title, 500), Author: truncate(author, 200),
		Content: content, Tags: tags, ImageURLs: images,
	}, nil
}

func (c Collector) fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return nil, "", errors.New("RESEARCH_INVALID_URL")
	}
	if err := ValidateDestination(ctx, parsed.Hostname(), nil); err != nil {
		return nil, "", err
	}
	client := c.Client
	if client == nil {
		client = NewHTTPClient(20 * time.Second)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CortexResearch/1.0)")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	if c.CookieHeader != "" {
		request.Header.Set("Cookie", c.CookieHeader)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, "", errors.New("XHS_RATE_LIMITED")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, "", errors.New("XHS_AUTH_REQUIRED")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") {
		return nil, "", errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	maxBytes := c.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = 4 << 20
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		return nil, "", errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	finalURL, err := NormalizeURL(response.Request.URL.String())
	if err != nil {
		return nil, "", errors.New("RESEARCH_INVALID_URL")
	}
	return body, finalURL, nil
}

func parseMetadata(document string) map[string][]string {
	result := map[string][]string{}
	for _, match := range metaPattern.FindAllStringSubmatch(document, -1) {
		result[strings.ToLower(match[1])] = append(result[strings.ToLower(match[1])], html.UnescapeString(match[2]))
	}
	for _, match := range metaReverse.FindAllStringSubmatch(document, -1) {
		result[strings.ToLower(match[2])] = append(result[strings.ToLower(match[2])], html.UnescapeString(match[1]))
	}
	var nextData struct {
		Props map[string]any `json:"props"`
	}
	if index := strings.Index(document, `id="__NEXT_DATA__"`); index >= 0 {
		start := strings.Index(document[index:], ">")
		end := strings.Index(document[index+start+1:], "</script>")
		if start >= 0 && end >= 0 {
			_ = json.Unmarshal([]byte(document[index+start+1:index+start+1+end]), &nextData)
		}
	}
	return result
}

func metadataValues(values map[string][]string, key string, limit int) []string {
	seen := map[string]bool{}
	var result []string
	for _, item := range values[key] {
		parsed, err := url.Parse(strings.TrimSpace(item))
		if err != nil || parsed.Scheme != "https" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func uniqueMatches(pattern *regexp.Regexp, value string, limit int) []string {
	seen := map[string]bool{}
	var result []string
	for _, match := range pattern.FindAllStringSubmatch(value, -1) {
		item := match[1]
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
		if len(result) >= limit {
			break
		}
	}
	sort.Strings(result)
	return result
}

func truncate(value string, count int) string {
	if utf8.RuneCountInString(value) <= count {
		return strings.TrimSpace(value)
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:count]))
}

func first(values ...[]string) string {
	for _, list := range values {
		if len(list) > 0 && strings.TrimSpace(list[0]) != "" {
			return list[0]
		}
	}
	return ""
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	code := strings.TrimSpace(err.Error())
	switch code {
	case "RESEARCH_INVALID_URL", "XHS_AUTH_REQUIRED", "XHS_RATE_LIMITED",
		"XHS_SOURCE_UNAVAILABLE", "XHS_LAYOUT_CHANGED":
		return code
	default:
		return "XHS_COLLECTOR_UNAVAILABLE"
	}
}

func PublicError(code string) string {
	switch code {
	case "XHS_AUTH_REQUIRED":
		return "小红书访问授权已过期"
	case "XHS_RATE_LIMITED":
		return "平台访问频率受限，请稍后重试"
	case "XHS_LAYOUT_CHANGED":
		return "平台页面结构已变化，暂时无法解析"
	case "RESEARCH_INVALID_URL":
		return "来源链接无效"
	default:
		return "来源暂时无法访问"
	}
}

func ValidateKeywords(values []string, max int) ([]string, error) {
	if len(values) == 0 || len(values) > max {
		return nil, fmt.Errorf("RESEARCH_INVALID_KEYWORD")
	}
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" || utf8.RuneCountInString(item) > 80 {
			return nil, fmt.Errorf("RESEARCH_INVALID_KEYWORD")
		}
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result, nil
}
