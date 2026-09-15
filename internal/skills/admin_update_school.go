package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"luminous/internal/httpclient"
	"luminous/internal/skill"
)

// AdminUpdateSchoolSkill implements the admin_update_school MCP tool.
// It calls PUT {API_BASE_URL}/api/v1/admin/schools/:code (requires API_TOKEN).
type AdminUpdateSchoolSkill struct {
	client *httpclient.Client
}

// NewAdminUpdateSchool creates an AdminUpdateSchoolSkill backed by the given HTTP client.
func NewAdminUpdateSchool(client *httpclient.Client) *AdminUpdateSchoolSkill {
	return &AdminUpdateSchoolSkill{client: client}
}

// Definition returns the MCP tool metadata for admin_update_school.
func (s *AdminUpdateSchoolSkill) Definition() skill.ToolDef {
	return skill.ToolDef{
		Name:        "admin_update_school",
		Description: "管理员接口：部分更新一所学校的信息。只需提供要修改的字段，未提供的字段保持不变。支持修改：名称(name)、网站(website)、教务系统地址(edu_system_url)、功能列表(features)、启用状态(enabled)、周开始日(week_start_day)。学校代码不可修改。",
		InputSchema: skill.InputSchema{
			Type: "object",
			Properties: map[string]skill.PropertyDef{
				"code": {
					Type:        "string",
					Description: "要更新的学校代码（必填，用于标识目标学校）",
				},
				"name": {
					Type:        "string",
					Description: "新的学校名称（可选）",
				},
				"website": {
					Type:        "string",
					Description: "新的网站地址（可选）",
				},
				"edu_system_url": {
					Type:        "string",
					Description: "新的教务系统地址（可选），即学生登录教务系统的网址，如 https://jwc.example.edu.cn。传空字符串可清除该地址",
				},
				"features": {
					Type:        "array",
					Description: "新的教务功能列表（可选，会替换全部功能）",
				},
				"enabled": {
					Type:        "boolean",
					Description: "是否启用（可选）。设为 false 可将学校从公开列表中隐藏",
				},
				"week_start_day": {
					Type:        "integer",
					Description: "周开始日 0-6（可选）",
				},
			},
			Required: []string{"code"},
		},
	}
}

// Execute calls the Luminous admin API to partially update a school.
func (s *AdminUpdateSchoolSkill) Execute(ctx context.Context, args map[string]any) (string, error) {
	code, ok := args["code"]
	if !ok || code == nil {
		return "", fmt.Errorf("缺少必填参数 code")
	}
	codeStr, ok := code.(string)
	if !ok || codeStr == "" {
		return "", fmt.Errorf("code 必须是字符串类型")
	}

	reqBody := map[string]any{}

	if v, ok := args["name"]; ok && v != nil {
		if s, ok := v.(string); ok && s != "" {
			reqBody["name"] = s
		}
	}

	if v, ok := args["website"]; ok && v != nil {
		if s, ok := v.(string); ok && s != "" {
			reqBody["website"] = s
		}
	}

	// 与 website 不同，这里接受空字符串：清空教务系统地址是有效操作，
	// 消费方会回退到 website。
	if v, ok := args["edu_system_url"]; ok && v != nil {
		if s, ok := v.(string); ok {
			reqBody["edu_system_url"] = s
		}
	}

	if v, ok := args["features"]; ok && v != nil {
		switch f := v.(type) {
		case []any:
			var arr []string
			for _, e := range f {
				if s, ok := e.(string); ok {
					arr = append(arr, s)
				}
			}
			reqBody["features"] = arr
		case []string:
			reqBody["features"] = f
		case string:
			var arr []string
			if err := json.Unmarshal([]byte(f), &arr); err == nil {
				reqBody["features"] = arr
			}
		}
	}

	if v, ok := args["enabled"]; ok && v != nil {
		switch b := v.(type) {
		case bool:
			reqBody["enabled"] = b
		case string:
			if b == "true" {
				reqBody["enabled"] = true
			} else if b == "false" {
				reqBody["enabled"] = false
			}
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

	respBody, err := s.client.Put(ctx, "api/v1/admin/schools/"+codeStr, jsonBody)
	if err != nil {
		return "", fmt.Errorf("更新学校 %s 失败: %w", codeStr, err)
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
