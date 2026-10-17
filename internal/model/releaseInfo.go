package model

import "time"

// ReleaseInfo 对应 C# 的 ReleaseInfo
type ReleaseInfo struct {
	Id        int64       `json:"id"`
	TagName   string      `json:"tag_name"`
	Name      string      `json:"name"`
	Body      string      `json:"body"`
	Author    *AuthorInfo `json:"author,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	Assets    []AssetInfo `json:"assets"`
}

type AuthorInfo struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

type AssetInfo struct {
	BrowserDownloadUrl string `json:"browser_download_url"`
	Name               string `json:"name"`
}

// RawSoft 是发布平台上的一条安装包记录。
type RawSoft struct {
	Name    string `json:"name"`
	SoftUrl string `json:"softUrl"`
}

// 用于解析 GetTagModel 中特殊的 JSON 结构
type RawApiResponse struct {
	ReleaseId string    `json:"releaseId"`
	Context   string    `json:"context"`
	Softs     []RawSoft `json:"softs"`
}
