package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"luminous/internal/httpclient"
	"luminous/internal/skill"
)

// AdminCreateSchoolSkill implements the admin_create_school MCP tool.
// It calls POST {API_BASE_URL}/api/v1/admin/schools (requires API_TOKEN).
type AdminCreateSchoolSkill struct {
	client *httpclient.Client
}

// NewAdminCreateSchool creates an AdminCreateSchoolSkill backed by the given HTTP client.
func NewAdminCreateSchool(client *httpclient.Client) *AdminCreateSchoolSkill {
	return &AdminCreateSchoolSkill{client: client}
}

// Definition returns the MCP tool metadata for admin_create_school.
func (s *AdminCreateSchoolSkill) Definition() skill.ToolDef {
	return skill.ToolDef{
		Name:        "admin_create_school",
		Description: "管理员接口：新增一所学校。需要提供学校代码、名称、网站和教务功能列表。学校代码为 1-20 位大写字母/数字/连字符/下划线。功能列表可从以下值中选择：login, timetable, grade_query, gpa_calculation, exam_schedule, course_schedule, bus_schedule, program, study_progress, electricity, payment, map。",
		InputSchema: skill.InputSchema{
			Type: "object",
			Properties: map[string]skill.PropertyDef{
				"code": {
					Type:        "string",
					Description: "学校代码，1-20 位大写字母/数字/连字符/下划线，如 XAUAT",
				},
				"name": {
					Type:        "string",
					Description: "学校名称，如「西安建筑科技大学」",
				},
				"website": {
					Type:        "string",
					Description: "学校后端 API 网站地址，必须以 http:// 或 https:// 开头，如 https://xauatapi.xauat.site",
				},
				"edu_system_url": {
					Type:        "string",
					Description: "教务系统地址（可选），即学生登录教务系统的网址，必须以 http:// 或 https:// 开头，如 https://jwc.example.edu.cn。留空时客户端会回退到 website",
				},
				"features": {
					Type:        "array",
					Description: "教务功能列表（字符串数组）。可选值：login, timetable, grade_query, gpa_calculation, exam_schedule, course_schedule, bus_schedule, program, study_progress, electricity, payment, map",
				},
				"week_start_day": {
					Type:        "integer",
					Description: "学校周开始日，0=周日, 1=周一, ..., 6=周六，默认 0",
				},
			},
			Required: []string{"code", "name", "website", "features"},
		},
	}
}

// Execute calls the Luminous admin API to create a school.
func (s *AdminCreateSchoolSkill) Execute(ctx context.Context, args map[string]any) (string, error) {
	// --- validate required fields ---
	code, ok := args["code"]
	if !ok || code == nil {
		return "", fmt.Errorf("缺少必填参数 code")
	}
	codeStr, ok := code.(string)
	if !ok || codeStr == "" {
		return "", fmt.Errorf("code 必须是字符串类型")
	}

	name, ok := args["name"]
	if !ok || name == nil {
		return "", fmt.Errorf("缺少必填参数 name")
	}
	nameStr, ok := name.(string)
	if !ok || nameStr == "" {
		return "", fmt.Errorf("name 必须是字符串类型")
	}

	website, ok := args["website"]
	if !ok || website == nil {
		return "", fmt.Errorf("缺少必填参数 website")
	}
	websiteStr, ok := website.(string)
	if !ok || websiteStr == "" {
		return "", fmt.Errorf("website 必须是字符串类型")
	}

	features, ok := args["features"]
	if !ok || features == nil {
		return "", fmt.Errorf("缺少必填参数 features")
	}

	// features can be []any or stringified JSON array
	var featuresArr []string
	switch f := features.(type) {
	case []any:
		for _, v := range f {
			s, ok := v.(string)
			if !ok {
				return "", fmt.Errorf("features 数组中的每个元素必须是字符串")
			}
			featuresArr = append(featuresArr, s)
		}
	case []string:
		featuresArr = f
	case string:
		if err := json.Unmarshal([]byte(f), &featuresArr); err != nil {
			return "", fmt.Errorf("features 必须是字符串数组")
		}
	default:
		return "", fmt.Errorf("features 必须是字符串数组")
	}

	reqBody := map[string]any{
		"code":     codeStr,
		"name":     nameStr,
		"website":  websiteStr,
		"features": featuresArr,
	}

	if v, ok := args["edu_system_url"]; ok && v != nil {
		if s, ok := v.(string); ok && s != "" {
			reqBody["edu_system_url"] = s
		}
	}

	if v, ok := args["week_start_day"]; ok && v != nil {
		switch n := v.(type) {
		case float64:
			reqBody["week_start_day"] = int(n)
		case int:
			reqBody["week_start_day"] = n
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}

	respBody, err := s.client.Post(ctx, "api/v1/admin/schools", jsonBody)
	if err != nil {
		return "", fmt.Errorf("创建学校 %s 失败: %w", codeStr, err)
	}

	var parsed any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("后端返回了无效的 JSON: %w", err)
	}

	result, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("序列化响应失败: %w", err)
	}

	return string(result), nil
}
