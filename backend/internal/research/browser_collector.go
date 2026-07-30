package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type PageState string

const (
	PageReady                PageState = "ready"
	PageLoginRequired        PageState = "login_required"
	PageVerificationRequired PageState = "verification_required"
	PageRateLimited          PageState = "rate_limited"
	PageNotFound             PageState = "not_found"
	PageLayoutChanged        PageState = "layout_changed"
)

type BrowserCollector struct {
	ctx                context.Context
	cancel             context.CancelFunc
	maxImages          int
	mu                 sync.Mutex
	searchAPIStatuses  []int
	searchRequestIDs   []network.RequestID
	searchResponseMeta []string
	searchCards        map[string]Collected
}

func NewBrowserCollector(
	parent context.Context, chromePath string, session SessionState, maxImages int,
) (*BrowserCollector, error) {
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath), chromedp.Headless, chromedp.NoSandbox,
		chromedp.Flag("disable-gpu", true), chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.WindowSize(1280, 900),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125.0.0.0 Safari/537.36"),
	)
	allocator, cancelAllocator := chromedp.NewExecAllocator(parent, options...)
	browserContext, cancelBrowser := chromedp.NewContext(allocator)
	cancel := func() {
		cancelBrowser()
		cancelAllocator()
	}
	if err := chromedp.Run(browserContext, network.Enable()); err != nil {
		cancel()
		return nil, fmt.Errorf("XHS_BROWSER_UNAVAILABLE: %w", err)
	}
	for _, cookie := range session.Cookies {
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		if domain != "xiaohongshu.com" && !strings.HasSuffix(domain, ".xiaohongshu.com") {
			continue
		}
		action := network.SetCookie(cookie.Name, cookie.Value).
			WithDomain(cookie.Domain).WithPath(firstString(cookie.Path, "/")).
			WithSecure(cookie.Secure).WithHTTPOnly(cookie.HTTPOnly)
		if cookie.Expires > 0 {
			expires := cdp.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))
			action = action.WithExpires(&expires)
		}
		if err := chromedp.Run(browserContext, action); err != nil {
			cancel()
			return nil, errors.New("XHS_SESSION_INVALID")
		}
	}
	if len(session.LocalStorage) > 0 {
		if err := chromedp.Run(browserContext, chromedp.Navigate("https://www.xiaohongshu.com/")); err != nil {
			cancel()
			return nil, errors.New("XHS_BROWSER_UNAVAILABLE")
		}
		for _, entry := range researchStorageEntries(session.LocalStorage) {
			name, _ := json.Marshal(entry.Name)
			value, _ := json.Marshal(entry.Value)
			expression := fmt.Sprintf("window.localStorage.setItem(%s,%s)", name, value)
			if err := chromedp.Run(browserContext, chromedp.Evaluate(expression, nil)); err != nil {
				cancel()
				return nil, errors.New("XHS_SESSION_INVALID")
			}
		}
	}
	collector := &BrowserCollector{
		ctx: browserContext, cancel: cancel, maxImages: maxImages,
		searchCards: make(map[string]Collected),
	}
	chromedp.ListenTarget(browserContext, func(event any) {
		response, ok := event.(*network.EventResponseReceived)
		if !ok {
			return
		}
		responseURL, err := url.Parse(response.Response.URL)
		if err != nil || !strings.HasSuffix(strings.ToLower(responseURL.Hostname()), "xiaohongshu.com") ||
			!strings.HasPrefix(responseURL.Path, "/api/sns/web/v1/") {
			return
		}
		collector.mu.Lock()
		if len(collector.searchRequestIDs) < 100 {
			collector.searchAPIStatuses = append(collector.searchAPIStatuses, int(response.Response.Status))
			collector.searchRequestIDs = append(collector.searchRequestIDs, response.RequestID)
		}
		collector.mu.Unlock()
	})
	return collector, nil
}

func (c *BrowserCollector) SearchAPIStatuses() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.searchAPIStatuses...)
}

func (c *BrowserCollector) SearchResponseMeta() []string {
	c.mu.Lock()
	result := append([]string(nil), c.searchResponseMeta...)
	c.mu.Unlock()
	var structure struct {
		Path           string   `json:"path"`
		Inputs         []string `json:"inputs"`
		Classes        []string `json:"classes"`
		ExploreLinks   int      `json:"explore_links"`
		DiscoveryLinks int      `json:"discovery_links"`
	}
	diagnosticContext, cancel := context.WithTimeout(c.ctx, 3*time.Second)
	defer cancel()
	if err := chromedp.Run(diagnosticContext, chromedp.Evaluate(`(()=>{
		const clean=v=>String(v||"").trim().slice(0,120);
		return {
			path:location.pathname,
			inputs:Array.from(document.querySelectorAll("input")).slice(0,10).map(e=>clean(e.className)),
			classes:Array.from(new Set(Array.from(document.querySelectorAll("main *,#app *"))
				.map(e=>clean(e.className)).filter(Boolean))).slice(0,80),
			explore_links:document.querySelectorAll("a[href*='/explore/']").length,
			discovery_links:document.querySelectorAll("a[href*='/discovery/item/']").length
		};
	})()`, &structure)); err == nil {
		if raw, marshalErr := json.Marshal(structure); marshalErr == nil {
			result = append(result, "dom="+string(raw))
		}
	}
	return result
}

func researchStorageEntries(entries []SessionStorageEntry) []SessionStorageEntry {
	return SanitizeLocalStorage(entries)
}

func (c *BrowserCollector) Close() {
	if c != nil && c.cancel != nil {
		c.cancel()
	}
}

func (c *BrowserCollector) Search(ctx context.Context, keyword string, count int, sortMode string) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}
	c.mu.Lock()
	c.searchAPIStatuses = nil
	c.searchRequestIDs = nil
	c.searchResponseMeta = nil
	c.mu.Unlock()
	searchURL := "https://www.xiaohongshu.com/search_result?keyword=" +
		url.QueryEscape(keyword) + "&sort=" +
		url.QueryEscape(NormalizeSearchSort(sortMode))
	navigateContext, cancelNavigate := context.WithTimeout(c.ctx, 20*time.Second)
	err := chromedp.Run(navigateContext, chromedp.Navigate(searchURL))
	cancelNavigate()
	if err != nil {
		return nil, errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	if err := c.waitForContent(ctx, 15*time.Second); err != nil {
		if urls := c.searchResponseURLs(count); len(urls) > 0 {
			return urls, nil
		}
		return nil, err
	}
	c.cacheSearchCards()
	// Search cards may expose a bare /explore/{id} link which cannot be opened
	// reliably outside the result page. Prefer the API result because it carries
	// the short-lived xsec_token required by the detail page.
	if urls := c.searchResponseURLs(count); len(urls) > 0 {
		return urls, nil
	}
	if urls := c.searchPageStateURLs(count); len(urls) > 0 {
		return urls, nil
	}
	if urls := c.searchEmbeddedStateURLs(count); len(urls) > 0 {
		return urls, nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, count)
	stagnant := 0
	for scroll := 0; scroll < 30 && len(result) < count && stagnant < 4; scroll++ {
		var links []string
		if err := chromedp.Run(c.ctx, chromedp.Evaluate(`Array.from(document.querySelectorAll(
			"section.note-item a[href*='/explore/'],.note-item a[href*='/explore/'],a[href*='/explore/']"
		)).map(a=>a.href)`, &links)); err != nil {
			return nil, errors.New("XHS_LAYOUT_CHANGED")
		}
		before := len(result)
		for _, item := range links {
			normalized, err := NormalizeURL(item)
			if err == nil && !seen[normalized] {
				seen[normalized] = true
				result = append(result, normalized)
				if len(result) >= count {
					break
				}
			}
		}
		if len(result) == before {
			stagnant++
		} else {
			stagnant = 0
		}
		if len(result) < count {
			if err := chromedp.Run(c.ctx,
				chromedp.Evaluate(`window.scrollBy(0,window.innerHeight*2)`, nil),
				chromedp.Sleep(900*time.Millisecond)); err != nil {
				return nil, errors.New("XHS_SOURCE_UNAVAILABLE")
			}
		}
	}
	if len(result) == 0 {
		return nil, errors.New("XHS_LAYOUT_CHANGED")
	}
	return result, nil
}

func (c *BrowserCollector) cacheSearchCards() {
	var cards []struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Author  string `json:"author"`
		Content string `json:"content"`
		Image   string `json:"image"`
	}
	actionContext, cancel := context.WithTimeout(c.ctx, 3*time.Second)
	defer cancel()
	const expression = `Array.from(document.querySelectorAll(
		"section.note-item a[href*='/explore/'],.note-item a[href*='/explore/'],a[href*='/explore/']"
	)).slice(0,100).map(link=>{
		const card=link.closest("section.note-item,.note-item")||link.parentElement||link;
		const text=selector=>(card.querySelector(selector)?.innerText||"").trim();
		const image=card.querySelector("img");
		const author=text(".author .name,[class*='author'] [class*='name'],[class*='author']")
			.split("\n")[0].trim();
		return {
			url:link.href,
			title:text(".title,[class*='title'],[class*='desc'],a[class*='cover']")||
				(image?.alt||"").trim(),
			author,
			content:(card.innerText||"").trim().slice(0,4000),
			image:image?(image.currentSrc||image.src||""):""
		};
	})`
	if err := chromedp.Run(actionContext, chromedp.Evaluate(expression, &cards)); err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, card := range cards {
		key := noteURLKey(card.URL)
		if key == "" || (card.Title == "" && card.Content == "") {
			continue
		}
		images := []string(nil)
		if strings.HasPrefix(card.Image, "https://") {
			images = []string{card.Image}
		}
		c.searchCards[key] = Collected{
			URL: card.URL, Title: truncate(card.Title, 500), Author: truncate(card.Author, 200),
			Content: truncate(card.Content, 100_000), ImageURLs: images,
			ParseStrategy: "browser_search_card",
		}
	}
}

func noteURLKey(raw string) string {
	value, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(value.Path, "/explore/") {
		return ""
	}
	return value.Path
}

func (c *BrowserCollector) cachedSearchCard(rawURL string) (Collected, bool) {
	key := noteURLKey(rawURL)
	c.mu.Lock()
	defer c.mu.Unlock()
	card, ok := c.searchCards[key]
	return card, ok
}

func (c *BrowserCollector) searchEmbeddedStateURLs(count int) []string {
	if count <= 0 {
		return nil
	}
	var items []struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	actionContext, cancel := context.WithTimeout(c.ctx, 3*time.Second)
	defer cancel()
	const expression = `(()=>{
		const result=[],seen=new Set();
		for(const script of document.scripts){
			const text=script.textContent||"";
			if(!/xsec[_-]?token|xsecToken/.test(text))continue;
			const tokenPattern=/(?:xsec[_-]?token|xsecToken)\s*["']?\s*[:=]\s*["']([^"'\\<>&]{8,512})/g;
			for(const match of text.matchAll(tokenPattern)){
				const start=Math.max(0,match.index-2000),end=Math.min(text.length,match.index+2000);
				const nearby=text.slice(start,end),ids=Array.from(nearby.matchAll(/[0-9a-f]{24}/g));
				if(!ids.length)continue;
				const token=match[1].replace(/\\u002F/g,"/").replace(/\\\//g,"/");
				const nearest=ids.reduce((best,current)=>
					Math.abs((start+current.index)-match.index)<Math.abs((start+best.index)-match.index)?current:best
				);
				const id=nearest[0],key=id+"|"+token;
				if(!seen.has(key)){seen.add(key);result.push({id,token});}
				if(result.length>=100)return result;
			}
		}
		return result;
	})()`
	if err := chromedp.Run(actionContext, chromedp.Evaluate(expression, &items)); err != nil {
		return nil
	}
	result := make([]string, 0, min(count, len(items)))
	for _, item := range items {
		candidate := "https://www.xiaohongshu.com/explore/" + item.ID +
			"?xsec_token=" + url.QueryEscape(item.Token) + "&xsec_source=pc_search"
		if normalized, err := NormalizeURL(candidate); err == nil {
			result = append(result, normalized)
			if len(result) >= count {
				break
			}
		}
	}
	return result
}

func (c *BrowserCollector) searchPageStateURLs(count int) []string {
	if count <= 0 {
		return nil
	}
	var items []struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	actionContext, cancel := context.WithTimeout(c.ctx, 3*time.Second)
	defer cancel()
	const expression = `(()=>{
		const roots=[window.__INITIAL_STATE__,window.__INITIAL_SSR_STATE__].filter(Boolean);
		const result=[],seen=new Set(),visited=new WeakSet();
		const walk=value=>{
			if(!value||typeof value!=="object"||visited.has(value)||result.length>=100)return;
			visited.add(value);
			if(typeof value.id==="string"&&/^[0-9a-f]{24}$/.test(value.id)&&
				typeof value.xsec_token==="string"&&value.xsec_token&&value.note_card){
				const key=value.id+"|"+value.xsec_token;
				if(!seen.has(key)){seen.add(key);result.push({id:value.id,token:value.xsec_token});}
			}
			for(const child of Array.isArray(value)?value:Object.values(value))walk(child);
		};
		roots.forEach(walk);
		return result;
	})()`
	if err := chromedp.Run(actionContext, chromedp.Evaluate(expression, &items)); err != nil {
		return nil
	}
	result := make([]string, 0, min(count, len(items)))
	for _, item := range items {
		candidate := "https://www.xiaohongshu.com/explore/" + item.ID +
			"?xsec_token=" + url.QueryEscape(item.Token) + "&xsec_source=pc_search"
		if normalized, err := NormalizeURL(candidate); err == nil {
			result = append(result, normalized)
			if len(result) >= count {
				break
			}
		}
	}
	return result
}

func (c *BrowserCollector) searchResponseURLs(count int) []string {
	c.mu.Lock()
	requestIDs := append([]network.RequestID(nil), c.searchRequestIDs...)
	c.mu.Unlock()
	seen := map[string]bool{}
	result := make([]string, 0, count)
	for _, requestID := range requestIDs {
		var body []byte
		err := chromedp.Run(c.ctx, chromedp.ActionFunc(func(actionContext context.Context) error {
			value, err := network.GetResponseBody(requestID).Do(actionContext)
			if err == nil {
				body = value
			}
			return err
		}))
		if err != nil {
			continue
		}
		meta := SearchResponseMetadata(body)
		c.mu.Lock()
		c.searchResponseMeta = append(c.searchResponseMeta, meta)
		c.mu.Unlock()
		for _, item := range SearchResponseURLs(body) {
			if !seen[item] {
				seen[item] = true
				result = append(result, item)
				if len(result) >= count {
					return result
				}
			}
		}
	}
	return result
}

func SearchResponseMetadata(raw []byte) string {
	if len(raw) == 0 {
		return "empty"
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return "non_json"
	}
	topKeys := sortedMapKeys(payload)
	code := payload["code"]
	success := payload["success"]
	dataType := fmt.Sprintf("%T", payload["data"])
	dataKeys := []string(nil)
	itemsCount := -1
	if data, ok := payload["data"].(map[string]any); ok {
		dataKeys = sortedMapKeys(data)
		if items, ok := data["items"].([]any); ok {
			itemsCount = len(items)
		}
	}
	return fmt.Sprintf("code=%v success=%v data_type=%s items=%d top_keys=%v data_keys=%v",
		code, success, dataType, itemsCount, topKeys, dataKeys)
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func SearchResponseURLs(raw []byte) []string {
	var payload any
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	seen := map[string]bool{}
	var result []string
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case map[string]any:
			id, _ := current["id"].(string)
			token, _ := current["xsec_token"].(string)
			_, hasNoteCard := current["note_card"]
			if hasNoteCard && regexp.MustCompile(`^[0-9a-f]{24}$`).MatchString(id) {
				item := "https://www.xiaohongshu.com/explore/" + id
				if token != "" {
					item += "?xsec_token=" + url.QueryEscape(token) + "&xsec_source=pc_search"
				}
				if normalized, err := NormalizeURL(item); err == nil && !seen[normalized] {
					seen[normalized] = true
					result = append(result, normalized)
				}
			}
			for _, child := range current {
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(payload)
	return result
}

func (c *BrowserCollector) Collect(ctx context.Context, rawURL string) (Collected, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return Collected{}, err
	}
	openedFromResults := false
	targetURL, _ := url.Parse(normalized)
	if strings.HasPrefix(targetURL.Path, "/explore/") {
		selector := fmt.Sprintf(`a[href*="%s"]`, targetURL.Path)
		clickContext, cancelClick := context.WithTimeout(c.ctx, 5*time.Second)
		var onSearchPage bool
		_ = chromedp.Run(clickContext,
			chromedp.Evaluate(`location.pathname==="/search_result"`, &onSearchPage),
			chromedp.Evaluate(fmt.Sprintf(`(()=>{
				const target=document.querySelector(%q);
				if(!target)return false;
				target.removeAttribute("target");
				return true;
			})()`, selector), &openedFromResults),
		)
		if onSearchPage && openedFromResults {
			_ = chromedp.Run(clickContext, chromedp.Click(selector, chromedp.ByQuery))
		} else {
			openedFromResults = false
		}
		cancelClick()
		if openedFromResults {
			routeReached := false
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				var routeState struct {
					Path   string `json:"path"`
					Detail bool   `json:"detail"`
				}
				routeContext, cancelRoute := context.WithTimeout(c.ctx, 2*time.Second)
				routeErr := chromedp.Run(routeContext, chromedp.Evaluate(`({
					path:location.pathname,
					detail:Boolean(document.querySelector(
						"#noteContainer,[class*='note-detail'],[class*='detail-container']"
					))
				})`, &routeState))
				cancelRoute()
				if routeErr == nil && (routeState.Path == targetURL.Path || routeState.Detail) {
					routeReached = true
					break
				}
				if ctx.Err() != nil {
					return Collected{}, ctx.Err()
				}
				time.Sleep(200 * time.Millisecond)
			}
			openedFromResults = routeReached
		}
	}
	if !openedFromResults {
		navigateContext, cancelNavigate := context.WithTimeout(c.ctx, 20*time.Second)
		err = chromedp.Run(navigateContext, chromedp.Navigate(normalized))
		cancelNavigate()
	}
	if err != nil {
		return Collected{}, errors.New("XHS_SOURCE_UNAVAILABLE")
	}
	if err := c.waitForContent(ctx, 15*time.Second); err != nil {
		if card, ok := c.cachedSearchCard(normalized); ok {
			card.URL = normalized
			return card, nil
		}
		return Collected{}, err
	}
	var detail struct {
		Title     string   `json:"title"`
		Author    string   `json:"author"`
		Content   string   `json:"content"`
		DateText  string   `json:"date_text"`
		Stats     []string `json:"stats"`
		ImageURLs []string `json:"image_urls"`
	}
	expression := fmt.Sprintf(`(()=>{
		const root=document.querySelector("#noteContainer,[class*='note-detail'],[class*='note-container']")||document;
		const text=(sel)=>{const e=root.querySelector(sel);return e?(e.innerText||e.textContent||"").trim():""};
		const carousel=root.querySelector("[class*='swiper'],[class*='carousel'],[class*='image-container'],[class*='note-image'],[class*='slider']")||root;
		const images=Array.from(carousel.querySelectorAll("img")).filter(img=>{
			const src=img.currentSrc||img.src||img.dataset.src||"";
			const cls=((img.className||"")+" "+(img.parentElement?.className||"")).toLowerCase();
			return src.startsWith("https://")&&!/avatar|author|user-img/.test(cls)&&
				!/_mw_1|avatar/i.test(src)&&(img.naturalWidth===0||img.naturalWidth>=240);
		}).map(img=>img.currentSrc||img.src||img.dataset.src).filter((v,i,a)=>a.indexOf(v)===i).slice(0,%d);
		return {
			title:text(".title,[class*='title']"),
			author:text(".author .name,[class*='author'] [class*='name'],[class*='author']"),
			content:text("#detail-desc,[class*='desc'],[class*='content']"),
			date_text:text("[class*='date'],[class*='time']"),
			stats:Array.from(root.querySelectorAll("[class*='like'],[class*='collect'],[class*='comment']")).map(e=>(e.className||"")+":"+(e.innerText||"").trim()),
			image_urls:images
		};
	})()`, max(1, c.maxImages))
	if err := chromedp.Run(c.ctx, chromedp.Evaluate(expression, &detail)); err != nil {
		return Collected{}, errors.New("XHS_LAYOUT_CHANGED")
	}
	if detail.Title == "" && detail.Content == "" {
		return Collected{}, errors.New("XHS_LAYOUT_CHANGED")
	}
	likeCount, collectCount, commentCount := ParseInteractionCounts(detail.Stats)
	return Collected{
		URL: normalized, Title: truncate(detail.Title, 500), Author: truncate(detail.Author, 200),
		Content: truncate(detail.Content, 100_000), Tags: uniqueMatches(tagPattern, detail.Content, 20),
		ImageURLs: detail.ImageURLs, PublishedAt: ParsePublishedAt(detail.DateText, time.Now()),
		LikeCount: likeCount, CollectCount: collectCount, CommentCount: commentCount,
		ParseStrategy: "browser_detail",
	}, nil
}

func (c *BrowserCollector) waitForContent(requestContext context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastSnapshot struct {
		URL          string `json:"url"`
		Title        string `json:"title"`
		Text         string `json:"text"`
		Ready        bool   `json:"ready"`
		NoteItems    int    `json:"note_items"`
		ExploreLinks int    `json:"explore_links"`
		LoginVisible bool   `json:"login_visible"`
	}
	for time.Now().Before(deadline) {
		if requestContext.Err() != nil {
			return requestContext.Err()
		}
		err := chromedp.Run(c.ctx, chromedp.Evaluate(`({
			url:location.href,title:document.title,text:(document.body?.innerText||"").slice(0,20000),
			ready:Boolean(document.querySelector(
				"section.note-item,.note-item,#noteContainer,[class*='note-detail'],a[href*='/explore/']"
			)),
			note_items:document.querySelectorAll("section.note-item,.note-item").length,
			explore_links:document.querySelectorAll("a[href*='/explore/']").length,
			login_visible:Array.from(document.querySelectorAll(
				".login-modal,.login-container,.side-bar-component.login-btn"
			)).some(element=>{
				const rect=element.getBoundingClientRect(),style=getComputedStyle(element);
				return rect.width>0&&rect.height>0&&style.display!=="none"&&
					style.visibility!=="hidden"&&Number(style.opacity)!==0;
			})
		})`, &lastSnapshot))
		if err == nil {
			if lastSnapshot.LoginVisible {
				return pageStateError(PageLoginRequired)
			}
			state := DetectPageState(lastSnapshot.URL, lastSnapshot.Title, lastSnapshot.Text, lastSnapshot.Ready)
			if state == PageReady {
				return nil
			}
			if state != PageLayoutChanged {
				return pageStateError(state)
			}
		}
		if err := chromedp.Run(c.ctx, chromedp.Sleep(500*time.Millisecond)); err != nil {
			return errors.New("XHS_SOURCE_UNAVAILABLE")
		}
	}
	return errors.New("XHS_LAYOUT_CHANGED")
}

func DetectPageState(rawURL, title, text string, ready bool) PageState {
	value := strings.ToLower(rawURL + "\n" + title + "\n" + text)
	switch {
	case strings.Contains(value, "captcha") || strings.Contains(value, "安全验证") ||
		(strings.Contains(value, "验证") && strings.Contains(value, "滑块")):
		return PageVerificationRequired
	case strings.Contains(value, "访问频繁") || strings.Contains(value, "请稍后再试"):
		return PageRateLimited
	case strings.Contains(strings.ToLower(rawURL), "login") || strings.Contains(title, "登录") ||
		strings.Contains(text, "登录后查看更多"):
		return PageLoginRequired
	case strings.Contains(title, "404") || strings.Contains(text, "笔记不存在"):
		return PageNotFound
	case ready:
		return PageReady
	default:
		return PageLayoutChanged
	}
}

func pageStateError(state PageState) error {
	switch state {
	case PageLoginRequired:
		return errors.New("XHS_AUTH_REQUIRED")
	case PageVerificationRequired:
		return errors.New("XHS_VERIFICATION_REQUIRED")
	case PageRateLimited:
		return errors.New("XHS_RATE_LIMITED")
	case PageNotFound:
		return errors.New("XHS_SOURCE_NOT_FOUND")
	default:
		return errors.New("XHS_LAYOUT_CHANGED")
	}
}

func ParseCount(value string) int64 {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, ",", ""), " ", ""))
	multiplier := float64(1)
	if strings.Contains(value, "万") {
		multiplier = 10_000
		value = strings.ReplaceAll(value, "万", "")
	}
	var numeric strings.Builder
	for _, char := range value {
		if (char >= '0' && char <= '9') || char == '.' {
			numeric.WriteRune(char)
		}
	}
	number, err := strconv.ParseFloat(numeric.String(), 64)
	if err != nil || number < 0 {
		return 0
	}
	return int64(math.Round(number * multiplier))
}

func ParseInteractionCounts(values []string) (int64, int64, int64) {
	var like, collect, comment int64
	for _, value := range values {
		lower := strings.ToLower(value)
		switch {
		case strings.Contains(lower, "赞") || strings.Contains(lower, "like"):
			like = max(like, ParseCount(value))
		case strings.Contains(lower, "收藏") || strings.Contains(lower, "collect"):
			collect = max(collect, ParseCount(value))
		case strings.Contains(lower, "评论") || strings.Contains(lower, "comment"):
			comment = max(comment, ParseCount(value))
		}
	}
	return like, collect, comment
}

func ParsePublishedAt(value string, now time.Time) *time.Time {
	value = strings.TrimSpace(value)
	if fields := strings.Fields(value); len(fields) > 0 {
		value = fields[0]
	}
	for _, format := range []string{"2006-01-02 15:04", "2006-01-02", "2006/01/02 15:04", "2006/01/02"} {
		if parsed, err := time.ParseInLocation(format, value, now.Location()); err == nil {
			return &parsed
		}
	}
	if parsed, err := time.ParseInLocation("01-02", value, now.Location()); err == nil {
		result := time.Date(now.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, now.Location())
		if result.After(now.Add(24 * time.Hour)) {
			result = result.AddDate(-1, 0, 0)
		}
		return &result
	}
	var amount int
	switch {
	case value == "刚刚":
		result := now
		return &result
	case strings.Contains(value, "分钟前"):
		_, _ = fmt.Sscanf(value, "%d分钟前", &amount)
		result := now.Add(-time.Duration(amount) * time.Minute)
		return &result
	case strings.Contains(value, "小时前"):
		_, _ = fmt.Sscanf(value, "%d小时前", &amount)
		result := now.Add(-time.Duration(amount) * time.Hour)
		return &result
	case strings.Contains(value, "天前"):
		_, _ = fmt.Sscanf(value, "%d天前", &amount)
		result := now.AddDate(0, 0, -amount)
		return &result
	default:
		return nil
	}
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
