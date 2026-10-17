package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"

	"luminous/internal/config"
	"luminous/internal/model"
)

func softsFixture() []model.RawSoft {
	// 平台的入库顺序来自上传时的 find，实际可能是任意顺序；
	// 这里故意把 x86_64 放在最前来复现线上问题。
	return []model.RawSoft{
		{Name: "Downloader app-x86_64-release.apk", SoftUrl: "https://example.com/app-x86_64-release.apk"},
		{Name: "Downloader app-armeabi-v7a-release.apk", SoftUrl: "https://example.com/app-armeabi-v7a-release.apk"},
		{Name: "Downloader app-arm64-v8a-release.apk", SoftUrl: "https://example.com/app-arm64-v8a-release.apk"},
	}
}

func assetNames(assets []model.AssetInfo) []string {
	names := make([]string, len(assets))
	for i, asset := range assets {
		names[i] = asset.Name
	}
	return names
}

func TestNormalizeABI(t *testing.T) {
	cases := map[string]string{
		"arm64-v8a":   abiArm64,
		"ARM64":       abiArm64,
		"aarch64":     abiArm64,
		"armeabi-v7a": abiArm,
		"armv7":       abiArm,
		"x86_64":      abiX8664,
		"x86":         abiX86,
		"":            "",
		"mips":        "",
	}
	for input, want := range cases {
		if got := normalizeABI(input); got != want {
			t.Errorf("normalizeABI(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestOrderSoftsDefaultsToArm64(t *testing.T) {
	assets := orderSofts(softsFixture(), "")

	want := []string{
		"Downloader app-arm64-v8a-release.apk",
		"Downloader app-armeabi-v7a-release.apk",
		"Downloader app-x86_64-release.apk",
	}
	got := assetNames(assets)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("asset[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestOrderSoftsPrefersRequestedABI(t *testing.T) {
	assets := orderSofts(softsFixture(), abiArm)
	if got := assets[0].Name; got != "Downloader app-armeabi-v7a-release.apk" {
		t.Fatalf("asset[0] = %q, want armeabi-v7a", got)
	}

	assets = orderSofts(softsFixture(), abiX8664)
	if got := assets[0].Name; got != "Downloader app-x86_64-release.apk" {
		t.Fatalf("asset[0] = %q, want x86_64", got)
	}
}

func TestOrderSoftsReturnsEverySoft(t *testing.T) {
	assets := orderSofts(softsFixture(), "")
	if len(assets) != 3 {
		t.Fatalf("got %d assets, want 3", len(assets))
	}

	assets = orderSofts([]model.RawSoft{
		{Name: "app-release.apk", SoftUrl: "https://example.com/app-release.apk"},
		{Name: "Downloader app-arm64-v8a-release.apk", SoftUrl: "https://example.com/app-arm64-v8a-release.apk"},
	}, "")
	if assets[0].Name != "Downloader app-arm64-v8a-release.apk" {
		t.Fatalf("universal package should rank after a known ABI, got %q first", assets[0].Name)
	}
}

func TestOrderSoftsKeepsStableOrderWithinABI(t *testing.T) {
	assets := orderSofts([]model.RawSoft{
		{Name: "first-arm64.apk", SoftUrl: "https://example.com/first-arm64.apk"},
		{Name: "second-arm64-v8a.apk", SoftUrl: "https://example.com/second-arm64-v8a.apk"},
	}, "")
	if assets[0].Name != "first-arm64.apk" || assets[1].Name != "second-arm64-v8a.apk" {
		t.Fatalf("unexpected order: %v", assetNames(assets))
	}
}

// TestGetTagModelReturnsBareReleaseArray 守护 /api/v1/app 的响应线上契约。
// App 端（1.2.2 及更早版本）把响应体直接解码成 []ReleaseInfo；一旦这里变成
// {code,message,data:{total,items}} 信封，客户端反序列化会抛异常并被静默吞掉，
// Android 就再也收不到更新提示。
func TestGetTagModelReturnsBareReleaseArray(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"releaseId": "1.2.2",
			"context": "release notes",
			"softs": [
				{"name": "Downloader app-armeabi-v7a-release.apk", "softUrl": "https://example.com/app-armeabi-v7a-release.apk"},
				{"name": "Downloader app-arm64-v8a-release.apk", "softUrl": "https://example.com/app-arm64-v8a-release.apk"}
			]
		}`)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse test upstream url: %v", err)
	}
	allowedUpstreamHosts[upstreamURL.Hostname()] = true
	defer delete(allowedUpstreamHosts, upstreamURL.Hostname())

	handler := NewAppHandler(config.ReleaseConfig{APIURL: upstream.URL})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/app", handler.GetTagModel)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/app", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", recorder.Code, recorder.Body.String())
	}

	var releases []model.ReleaseInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &releases); err != nil {
		t.Fatalf("response must be a bare release array, got %q: %v", recorder.Body.String(), err)
	}
	if len(releases) != 1 || releases[0].Name != "1.2.2" || releases[0].TagName != "1.2.2" {
		t.Fatalf("unexpected releases: %+v", releases)
	}
	if len(releases[0].Assets) != 2 {
		t.Fatalf("assets = %+v, want 2", releases[0].Assets)
	}
	if got := releases[0].Assets[0].Name; got != "Downloader app-arm64-v8a-release.apk" {
		t.Fatalf("asset[0] = %q, want arm64 first", got)
	}
}
