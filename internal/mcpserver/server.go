package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"luminous/internal/config"
	"luminous/internal/model"
	"luminous/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Handler struct{ handler *mcp.StreamableHTTPHandler }

func NewHandler(repo repository.SchoolRepository, release config.ReleaseConfig) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "luminous-mcp-server", Version: "1.0.0"}, nil)
	api := &API{repo: repo, client: &http.Client{Timeout: 30 * time.Second}, release: release}

	mcp.AddTool(server, &mcp.Tool{Name: "query_school", Description: "根据学校代码查询学校详情。"}, api.querySchool)
	mcp.AddTool(server, &mcp.Tool{Name: "list_schools", Description: "列出所有已启用的学校。"}, api.listSchools)
	mcp.AddTool(server, &mcp.Tool{Name: "get_app_info", Description: "获取最新的 App 版本信息和更新公告。"}, api.appInfo)
	mcp.AddTool(server, &mcp.Tool{Name: "get_course", Description: "获取学生课程表。"}, api.course)
	mcp.AddTool(server, &mcp.Tool{Name: "get_exam_arrangements", Description: "获取学生考试安排。"}, api.exam)
	mcp.AddTool(server, &mcp.Tool{Name: "get_scores", Description: "获取学生成绩列表。"}, api.scores)
	mcp.AddTool(server, &mcp.Tool{Name: "get_current_semester", Description: "获取当前学期信息。"}, api.currentSemester)
	mcp.AddTool(server, &mcp.Tool{Name: "get_bus", Description: "查询校车时刻表。"}, api.bus)
	mcp.AddTool(server, &mcp.Tool{Name: "get_electricity_balance", Description: "查询宿舍电费余额和本周用电量。"}, api.electricity)

	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}

func TokenMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		const p = "Bearer "
		got := c.GetHeader("Authorization")
		if !strings.HasPrefix(got, p) || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(got, p)), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "unauthorized", "data": nil})
			return
		}
		c.Next()
	}
}

type QuerySchoolArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码，例如 XAUAT"`
}
type CourseArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码"`
	StudentID  string `json:"student_id" jsonschema:"学生学号"`
	Cookie     string `json:"cookie" jsonschema:"教务系统认证 Cookie"`
}
type ExamArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码"`
	Cookie     string `json:"cookie" jsonschema:"教务系统认证 Cookie"`
	StudentID  string `json:"student_id,omitempty" jsonschema:"学生学号，可选"`
}
type ScoresArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码"`
	StudentID  string `json:"student_id" jsonschema:"学生学号"`
	Semester   string `json:"semester" jsonschema:"学期，如 2024-2025-2"`
	Cookie     string `json:"cookie" jsonschema:"教务系统认证 Cookie"`
}
type SemesterArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码"`
	Cookie     string `json:"cookie" jsonschema:"教务系统认证 Cookie"`
}
type BusArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码"`
	Loc        string `json:"loc,omitempty" jsonschema:"校区，默认 ALL"`
	Time       string `json:"time,omitempty" jsonschema:"日期，格式 YYYY-MM-DD"`
}
type ElectricityArgs struct {
	SchoolCode string `json:"school_code" jsonschema:"学校代码"`
	URL        string `json:"url,omitempty" jsonschema:"充值页面 URL，可选"`
}

type API struct {
	repo    repository.SchoolRepository
	client  *http.Client
	release config.ReleaseConfig
}

func result(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: json.RawMessage(b)}, v, nil
}
func fail(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
}
func (a *API) school(ctx context.Context, code, feature string) (*model.School, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !model.IsValidSchoolCode(code) {
		return nil, errors.New("school_code 无效")
	}
	s, err := a.repo.FindByCode(ctx, code)
	if err != nil || !s.Enabled {
		return nil, errors.New("学校不存在或未启用")
	}
	if feature == "" {
		return s, nil
	}
	for _, f := range s.Features {
		if string(f) == feature {
			return s, nil
		}
	}
	return nil, fmt.Errorf("学校不支持 %s 功能", feature)
}
func base(website string) (string, error) {
	u, err := url.Parse(strings.TrimRight(website, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("学校 website 无效")
	}
	if !strings.HasSuffix(u.Path, "/v1") {
		u.Path = strings.TrimRight(u.Path, "/") + "/v1"
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func (a *API) request(ctx context.Context, s *model.School, path string, q url.Values, cookie string) (any, error) {
	b, err := base(s.Website)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(b + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
		req.Header.Set("xauat", cookie)
	}
	req.Header.Set("x-language", "zh-CN")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("上游请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("上游返回 HTTP %d", resp.StatusCode)
	}
	var env struct {
		Data    json.RawMessage `json:"data"`
		Code    int             `json:"code"`
		Message string          `json:"message"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := dec.Decode(&env); err != nil {
		return nil, errors.New("上游返回无效 JSON")
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("上游业务错误: %s", env.Message)
	}
	var v any
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(env.Data, &v); err != nil {
		return nil, errors.New("上游 data 无效")
	}
	return v, nil
}
func (a *API) querySchool(ctx context.Context, _ *mcp.CallToolRequest, x *QuerySchoolArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "")
	if e != nil {
		return fail(e)
	}
	return result(s)
}
func (a *API) listSchools(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	s, e := a.repo.FindEnabled(ctx)
	if e != nil {
		return fail(e)
	}
	return result(s)
}
func (a *API) appInfo(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	u := a.release.APIURL
	if u == "" {
		u = fmt.Sprintf("https://appapi.xauat.site/api/App/%s/latest?channelId=%s", a.release.AppUUID, a.release.ChannelID)
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return fail(e)
	}
	r, e := a.client.Do(req)
	if e != nil {
		return fail(e)
	}
	defer r.Body.Close()
	var v any
	if r.StatusCode != 200 || json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&v) != nil {
		return fail(errors.New("获取 App 信息失败"))
	}
	return result(v)
}
func (a *API) course(ctx context.Context, _ *mcp.CallToolRequest, x *CourseArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "course_schedule")
	if e != nil {
		return fail(e)
	}
	v, e := a.request(ctx, s, "course", url.Values{"studentId": {x.StudentID}}, x.Cookie)
	if e != nil {
		return fail(e)
	}
	return result(v)
}
func (a *API) exam(ctx context.Context, _ *mcp.CallToolRequest, x *ExamArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "exam_schedule")
	if e != nil {
		return fail(e)
	}
	q := url.Values{}
	if x.StudentID != "" {
		q.Set("studentId", x.StudentID)
	}
	v, e := a.request(ctx, s, "exam", q, x.Cookie)
	if e != nil {
		return fail(e)
	}
	return result(v)
}
func (a *API) scores(ctx context.Context, _ *mcp.CallToolRequest, x *ScoresArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "grade_query")
	if e != nil {
		return fail(e)
	}
	v, e := a.request(ctx, s, "score", url.Values{"studentId": {x.StudentID}, "semester": {x.Semester}}, x.Cookie)
	if e != nil {
		return fail(e)
	}
	return result(v)
}
func (a *API) currentSemester(ctx context.Context, _ *mcp.CallToolRequest, x *SemesterArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "grade_query")
	if e != nil {
		return fail(e)
	}
	v, e := a.request(ctx, s, "score/ThisSemester", nil, x.Cookie)
	if e != nil {
		return fail(e)
	}
	return result(v)
}
func (a *API) bus(ctx context.Context, _ *mcp.CallToolRequest, x *BusArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "bus_schedule")
	if e != nil {
		return fail(e)
	}
	p := "bus/NewData"
	if x.Time != "" {
		p += "/" + url.PathEscape(x.Time)
	}
	q := url.Values{}
	if x.Loc == "" {
		x.Loc = "ALL"
	}
	q.Set("loc", x.Loc)
	v, e := a.request(ctx, s, p, q, "")
	if e != nil {
		return fail(e)
	}
	return result(v)
}
func (a *API) electricity(ctx context.Context, _ *mcp.CallToolRequest, x *ElectricityArgs) (*mcp.CallToolResult, any, error) {
	s, e := a.school(ctx, x.SchoolCode, "electricity")
	if e != nil {
		return fail(e)
	}
	q := url.Values{}
	if x.URL != "" {
		q.Set("url", x.URL)
	}
	b, e := a.request(ctx, s, "electricity", q, "")
	if e != nil {
		return fail(e)
	}
	w, e := a.request(ctx, s, "electricity/WeeklyData", q, "")
	if e != nil {
		return fail(e)
	}
	return result(map[string]any{"balance": b, "weekly": w})
}
