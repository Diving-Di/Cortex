package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSanitizedPageStateFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "xhs_*.json"))
	if err != nil || len(files) < 3 {
		t.Fatalf("fixtures=%v err=%v", files, err)
	}
	for _, path := range files {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var fixture struct {
			Name          string    `json:"name"`
			URL           string    `json:"url"`
			Title         string    `json:"title"`
			Text          string    `json:"text"`
			Ready         bool      `json:"ready"`
			ExpectedState PageState `json:"expected_state"`
		}
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatal(err)
		}
		t.Run(fixture.Name, func(t *testing.T) {
			if got := DetectPageState(fixture.URL, fixture.Title, fixture.Text, fixture.Ready); got != fixture.ExpectedState {
				t.Fatalf("state=%q want=%q", got, fixture.ExpectedState)
			}
		})
	}
}

func TestDetectPageState(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		title string
		text  string
		ready bool
		want  PageState
	}{
		{"ready", "https://www.xiaohongshu.com/search_result", "搜索", "", true, PageReady},
		{"login", "https://www.xiaohongshu.com/login", "登录", "", false, PageLoginRequired},
		{"verification", "https://www.xiaohongshu.com", "安全验证", "请拖动滑块", false, PageVerificationRequired},
		{"limited", "https://www.xiaohongshu.com", "", "访问频繁，请稍后再试", false, PageRateLimited},
		{"missing", "https://www.xiaohongshu.com/explore/a", "404", "笔记不存在", false, PageNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DetectPageState(test.url, test.title, test.text, test.ready); got != test.want {
				t.Fatalf("DetectPageState()=%q want %q", got, test.want)
			}
		})
	}
}

func TestParseCountAndInteractionCounts(t *testing.T) {
	if got := ParseCount("1.2万"); got != 12_000 {
		t.Fatalf("ParseCount()=%d", got)
	}
	like, collect, comment := ParseInteractionCounts([]string{
		"like-wrapper:1.2万", "collect-count:345", "comment:共 16 条评论",
	})
	if like != 12_000 || collect != 345 || comment != 16 {
		t.Fatalf("counts=%d,%d,%d", like, collect, comment)
	}
}

func TestNormalizeSearchSort(t *testing.T) {
	if got := NormalizeSearchSort("time_descending"); got != "time_descending" {
		t.Fatalf("sort=%q", got)
	}
	if got := NormalizeSearchSort("unsafe"); got != "general" {
		t.Fatalf("unsafe sort=%q", got)
	}
}

func TestNoteURLKeyIgnoresAccessQuery(t *testing.T) {
	got := noteURLKey("https://www.xiaohongshu.com/explore/abc123?xsec_token=secret")
	if got != "/explore/abc123" {
		t.Fatalf("noteURLKey()=%q", got)
	}
}

func TestSearchResponseURLs(t *testing.T) {
	raw := []byte(`{"data":{"items":[
		{"id":"69c39f58000000001f006849","xsec_token":"token/value","note_card":{"title":"safe"}},
		{"id":"not-a-note","xsec_token":"ignored","note_card":{}},
		{"id":"69c39f58000000001f006849","xsec_token":"token/value","note_card":{}}
	]}}`)
	got := SearchResponseURLs(raw)
	if len(got) != 1 ||
		got[0] != "https://www.xiaohongshu.com/explore/69c39f58000000001f006849?xsec_source=pc_search&xsec_token=token%2Fvalue" {
		t.Fatalf("SearchResponseURLs()=%v", got)
	}
}

func TestSearchResponseMetadata(t *testing.T) {
	got := SearchResponseMetadata([]byte(
		`{"code":0,"success":true,"data":{"items":[{},{}],"has_more":true}}`,
	))
	want := "code=0 success=true data_type=map[string]interface {} items=2 " +
		"top_keys=[code data success] data_keys=[has_more items]"
	if got != want {
		t.Fatalf("SearchResponseMetadata()=%q want=%q", got, want)
	}
}

func TestParsePublishedAt(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if got := ParsePublishedAt("3天前", now); got == nil || got.Day() != 26 {
		t.Fatalf("relative date=%v", got)
	}
	if got := ParsePublishedAt("04-12 广东", now); got == nil || got.Year() != 2026 || got.Month() != 4 || got.Day() != 12 {
		t.Fatalf("month-day=%v", got)
	}
}
