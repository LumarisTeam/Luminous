package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"luminous/internal/config"
	"luminous/internal/model"
	"luminous/internal/response"
	"luminous/internal/util"

	"github.com/gin-gonic/gin"
)

var allowedUpstreamHosts = map[string]bool{
	"appapi.xauat.site": true,
}

type AppHandler struct {
	releaseCfg config.ReleaseConfig
	client     *util.HTTPClient
}

func NewAppHandler(cfg config.ReleaseConfig) *AppHandler {
	return &AppHandler{
		releaseCfg: cfg,
		client:     util.NewHTTPClientWithAllowedHosts(allowedUpstreamHosts),
	}
}

func (h *AppHandler) GetTagModel(c *gin.Context) {
	apiUrl := h.releaseCfg.APIURL
	if apiUrl == "" {
		apiUrl = fmt.Sprintf(
			"https://appapi.xauat.site/api/App/%s/latest?channelId=%s",
			h.releaseCfg.AppUUID,
			h.releaseCfg.ChannelID,
		)
	}

	parsed, err := url.Parse(apiUrl)
	if err != nil {
		slog.Error("invalid upstream URL", "request_id", c.GetString("request_id"), "error", err)
		response.Error(c, http.StatusInternalServerError, "invalid upstream URL configured")
		return
	}
	if !allowedUpstreamHosts[parsed.Host] && !allowedUpstreamHosts[strings.SplitN(parsed.Host, ":", 2)[0]] {
		slog.Error("upstream host not allowed", "request_id", c.GetString("request_id"), "host", parsed.Host)
		response.Error(c, http.StatusInternalServerError, "upstream host not allowed")
		return
	}

	resp, err := h.client.GetWithContext(c.Request.Context(), apiUrl)
	if err != nil {
		slog.Error("upstream unreachable", "request_id", c.GetString("request_id"), "error", err)
		response.Error(c, http.StatusBadGateway, "upstream unreachable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		slog.Error("upstream returned error", "request_id", c.GetString("request_id"), "status", resp.StatusCode)
		response.Error(c, http.StatusBadGateway, "upstream returned error")
		return
	}

	var rawData model.RawApiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rawData); err != nil {
		slog.Error("invalid upstream response", "request_id", c.GetString("request_id"), "error", err)
		response.Error(c, http.StatusInternalServerError, "invalid upstream response")
		return
	}

	result := []model.ReleaseInfo{
		{
			TagName: rawData.ReleaseId,
			Name:    rawData.ReleaseId,
			Body:    rawData.Context,
			Assets:  nil,
		},
	}

	if len(rawData.Softs) > 0 {
		result[0].Assets = orderSofts(rawData.Softs, normalizeABI(c.Query("abi")))
	}

	// App 端（1.2.2 及更早版本）把响应体直接当作 Gitee Release 的裸数组解析，
	// 包成 {code,message,data:{total,items}} 会让反序列化抛异常、更新检查静默失效。
	// 该接口必须保持裸数组；需要信封的调用方请使用其它接口。
	c.JSON(http.StatusOK, result)
}

const (
	abiArm64 = "arm64-v8a"
	abiArm   = "armeabi-v7a"
	abiX8664 = "x86_64"
	abiX86   = "x86"
)

// abiFallback 是客户端没有指定 ABI 时的优先顺序。绝大多数真机是 arm64，
// 所以默认把 arm64-v8a 放在最前——老版本 App 只会取 assets[0]，靠这个顺序
// 就能拿到可安装的包；新的 App 会自行按 Abi.current() 选择。
var abiFallback = []string{abiArm64, abiArm, abiX8664, abiX86}

// normalizeABI 接受客户端常见的几种写法，统一成 Android ABI 名。
func normalizeABI(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "arm64", "arm64-v8a", "arm64_v8a", "aarch64":
		return abiArm64
	case "arm", "armv7", "armv7a", "armeabi", "armeabi-v7a", "armeabi_v7a":
		return abiArm
	case "x64", "x86-64", "x86_64":
		return abiX8664
	case "x86", "i386", "ia32":
		return abiX86
	default:
		return ""
	}
}

// apkABI 从安装包名或 URL 中识别 ABI。名如 app-arm64-v8a-release.apk。
func apkABI(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "arm64-v8a"), strings.Contains(lower, "arm64_v8a"), strings.Contains(lower, "arm64"):
		return abiArm64
	case strings.Contains(lower, "armeabi-v7a"), strings.Contains(lower, "armeabi_v7a"), strings.Contains(lower, "armeabi"):
		return abiArm
	case strings.Contains(lower, "x86_64"), strings.Contains(lower, "x86-64"):
		return abiX8664
	case strings.Contains(lower, "x86"):
		return abiX86
	default:
		return ""
	}
}

// abiRank 越小越靠前：请求的 ABI 排第一，其余按 abiFallback 顺序，通用包垫底。
func abiRank(abi, requested string) int {
	if requested != "" && abi == requested {
		return 0
	}
	offset := 1
	for i, candidate := range abiFallback {
		if abi == candidate {
			return offset + i
		}
	}
	return offset + len(abiFallback)
}

// orderSofts 返回按 ABI 排序后的完整 assets 列表。排序是稳定的，同一 ABI
// 内保持平台返回的原始顺序。
func orderSofts(softs []model.RawSoft, requested string) []model.AssetInfo {
	sorted := make([]model.RawSoft, len(softs))
	copy(sorted, softs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return abiRank(softNameABI(sorted[i]), requested) < abiRank(softNameABI(sorted[j]), requested)
	})

	assets := make([]model.AssetInfo, 0, len(sorted))
	for _, soft := range sorted {
		assets = append(assets, model.AssetInfo{
			Name:               soft.Name,
			BrowserDownloadUrl: soft.SoftUrl,
		})
	}
	return assets
}

func softNameABI(soft model.RawSoft) string {
	if abi := apkABI(soft.Name); abi != "" {
		return abi
	}
	return apkABI(soft.SoftUrl)
}
