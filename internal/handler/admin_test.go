package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"luminous/internal/repository"
	"luminous/internal/response"

	"github.com/gin-gonic/gin"
)

func setupAdminTest(t *testing.T) (*gin.Engine, *AdminHandler, repository.SchoolRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo, err := repository.NewJSONSchoolRepository(t.TempDir() + "/schools.json")
	if err != nil {
		t.Fatal(err)
	}

	h := NewAdminHandler(repo)
	r := gin.New()
	r.POST("/api/v1/admin/schools", h.CreateSchool)
	r.PUT("/api/v1/admin/schools/:code", h.UpdateSchool)
	return r, h, repo
}

func postSchool(t *testing.T, r *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/admin/schools", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func putSchool(t *testing.T, r *gin.Engine, code string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", "/api/v1/admin/schools/"+code, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func baseSchoolBody() map[string]any {
	return map[string]any{
		"code":     "TEST",
		"name":     "Test U",
		"website":  "https://api.test.edu",
		"features": []string{"login"},
	}
}

func TestCreateSchoolWithEduSystemURL(t *testing.T) {
	r, _, _ := setupAdminTest(t)

	body := baseSchoolBody()
	body["edu_system_url"] = "https://jwc.test.edu.cn"
	w := postSchool(t, r, body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	if data["edu_system_url"] != "https://jwc.test.edu.cn" {
		t.Fatalf("expected edu_system_url in response, got %v", data["edu_system_url"])
	}
}

func TestCreateSchoolEduSystemURLOptional(t *testing.T) {
	r, _, _ := setupAdminTest(t)

	// Omitted entirely — must still be accepted, since the field is optional.
	w := postSchool(t, r, baseSchoolBody())
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 when field omitted, got %d: %s", w.Code, w.Body.String())
	}

	body := baseSchoolBody()
	body["code"] = "EMPTY"
	body["edu_system_url"] = ""
	w = postSchool(t, r, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 when field empty, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateSchoolRejectsInvalidEduSystemURL(t *testing.T) {
	r, _, _ := setupAdminTest(t)

	for _, bad := range []string{"ftp://jwc.test.edu.cn", "javascript:alert(1)", "not-a-url"} {
		body := baseSchoolBody()
		body["edu_system_url"] = bad
		w := postSchool(t, r, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for %q, got %d: %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestUpdateSchoolSetsEduSystemURL(t *testing.T) {
	r, _, repo := setupAdminTest(t)

	if w := postSchool(t, r, baseSchoolBody()); w.Code != http.StatusCreated {
		t.Fatalf("setup create failed: %d", w.Code)
	}

	w := putSchool(t, r, "TEST", map[string]any{"edu_system_url": "https://jwc.test.edu.cn"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got, err := repo.FindByCode(context.Background(), "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if got.EduSystemURL != "https://jwc.test.edu.cn" {
		t.Fatalf("expected persisted edu system url, got %q", got.EduSystemURL)
	}

	// Clearing it is valid and must not be confused with "field absent".
	if w := putSchool(t, r, "TEST", map[string]any{"edu_system_url": ""}); w.Code != http.StatusOK {
		t.Fatalf("expected 200 when clearing, got %d: %s", w.Code, w.Body.String())
	}
	got, err = repo.FindByCode(context.Background(), "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if got.EduSystemURL != "" {
		t.Fatalf("expected cleared edu system url, got %q", got.EduSystemURL)
	}
}

func TestUpdateSchoolRejectsInvalidEduSystemURL(t *testing.T) {
	r, _, repo := setupAdminTest(t)

	if w := postSchool(t, r, baseSchoolBody()); w.Code != http.StatusCreated {
		t.Fatalf("setup create failed: %d", w.Code)
	}

	w := putSchool(t, r, "TEST", map[string]any{"edu_system_url": "ftp://jwc.test.edu.cn"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	got, err := repo.FindByCode(context.Background(), "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if got.EduSystemURL != "" {
		t.Fatalf("rejected update must not be applied, got %q", got.EduSystemURL)
	}
}

// Keeps the existing partial-update contract honest: a PUT that omits the
// field must leave it untouched.
func TestUpdateSchoolLeavesEduSystemURLAbsent(t *testing.T) {
	r, _, repo := setupAdminTest(t)

	body := baseSchoolBody()
	body["edu_system_url"] = "https://jwc.test.edu.cn"
	if w := postSchool(t, r, body); w.Code != http.StatusCreated {
		t.Fatalf("setup create failed: %d", w.Code)
	}

	if w := putSchool(t, r, "TEST", map[string]any{"name": "Renamed"}); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got, err := repo.FindByCode(context.Background(), "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if got.EduSystemURL != "https://jwc.test.edu.cn" {
		t.Fatalf("omitted field must be preserved, got %q", got.EduSystemURL)
	}
}
