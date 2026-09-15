package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"luminous/internal/model"
	"luminous/internal/repository"
	"luminous/internal/response"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	repo repository.SchoolRepository
}

func NewAdminHandler(repo repository.SchoolRepository) *AdminHandler {
	return &AdminHandler{repo: repo}
}

func (h *AdminHandler) AdminListSchools(c *gin.Context) {
	rid := c.GetString("request_id")
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		slog.Warn("invalid page parameter", "request_id", rid, "raw", c.Query("page"))
		response.Error(c, http.StatusBadRequest, "invalid page parameter")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if err != nil {
		slog.Warn("invalid page_size parameter", "request_id", rid, "raw", c.Query("page_size"))
		response.Error(c, http.StatusBadRequest, "invalid page_size parameter")
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	schools, err := h.repo.FindAll(c.Request.Context(), (page-1)*pageSize, pageSize)
	if err != nil {
		slog.Error("failed to list schools", "request_id", rid, "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list schools")
		return
	}

	total, err := h.repo.Count(c.Request.Context())
	if err != nil {
		slog.Error("failed to count schools", "request_id", rid, "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to count schools")
		return
	}

	response.SuccessList(c, http.StatusOK, "success", total, schools)
}

func (h *AdminHandler) CreateSchool(c *gin.Context) {
	rid := c.GetString("request_id")
	if ct := c.ContentType(); !strings.HasPrefix(ct, "application/json") {
		response.Error(c, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	var req model.CreateSchoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if !model.IsValidSchoolCode(req.Code) {
		response.Error(c, http.StatusBadRequest, "invalid school code: must be 1-20 chars, uppercase alphanumeric, hyphens or underscores")
		return
	}
	if !model.IsValidURL(req.Website) {
		response.Error(c, http.StatusBadRequest, "invalid website URL")
		return
	}
	if !model.IsValidEduSystemURL(req.EduSystemURL) {
		response.Error(c, http.StatusBadRequest, "invalid edu system URL")
		return
	}

	for _, f := range req.Features {
		if !model.IsValidFeature(f) {
			slog.Warn("invalid feature in create request", "request_id", rid, "feature", string(f))
			response.Error(c, http.StatusBadRequest, "invalid feature")
			return
		}
	}

	school := &model.School{
		Code:         req.Code,
		Name:         req.Name,
		Website:      req.Website,
		EduSystemURL: req.EduSystemURL,
		Features:     req.Features,
		Enabled:      true,
		WeekStartDay: req.WeekStartDay,
	}

	if err := h.repo.Create(c.Request.Context(), school); err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			response.Error(c, http.StatusConflict, "school already exists")
			return
		}
		slog.Error("failed to create school", "request_id", rid, "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to create school")
		return
	}
	response.Success(c, http.StatusCreated, "school created", school)
}

func (h *AdminHandler) UpdateSchool(c *gin.Context) {
	rid := c.GetString("request_id")
	code := c.Param("code")
	if !model.IsValidSchoolCode(code) {
		response.Error(c, http.StatusBadRequest, "invalid school code")
		return
	}

	if ct := c.ContentType(); !strings.HasPrefix(ct, "application/json") {
		response.Error(c, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	var req model.UpdateSchoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	existing, err := h.repo.FindByCode(c.Request.Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			response.Error(c, http.StatusNotFound, "school not found")
		} else {
			slog.Error("failed to get school for update", "request_id", rid, "code", code, "error", err)
			response.Error(c, http.StatusInternalServerError, "failed to get school")
		}
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Website != nil {
		if !model.IsValidURL(*req.Website) {
			response.Error(c, http.StatusBadRequest, "invalid website URL")
			return
		}
		existing.Website = *req.Website
	}
	if req.EduSystemURL != nil {
		if !model.IsValidEduSystemURL(*req.EduSystemURL) {
			response.Error(c, http.StatusBadRequest, "invalid edu system URL")
			return
		}
		existing.EduSystemURL = *req.EduSystemURL
	}
	if req.Features != nil {
		for _, f := range *req.Features {
			if !model.IsValidFeature(f) {
				slog.Warn("invalid feature in update request", "request_id", rid, "feature", string(f))
				response.Error(c, http.StatusBadRequest, "invalid feature")
				return
			}
		}
		existing.Features = *req.Features
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.WeekStartDay != nil {
		existing.WeekStartDay = *req.WeekStartDay
	}

	if err := h.repo.Update(c.Request.Context(), existing); err != nil {
		slog.Error("failed to update school", "request_id", rid, "code", code, "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to update school")
		return
	}
	response.Success(c, http.StatusOK, "school updated", existing)
}

func (h *AdminHandler) DeleteSchool(c *gin.Context) {
	rid := c.GetString("request_id")
	code := c.Param("code")
	if !model.IsValidSchoolCode(code) {
		response.Error(c, http.StatusBadRequest, "invalid school code")
		return
	}
	if err := h.repo.Delete(c.Request.Context(), code); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			response.Error(c, http.StatusNotFound, "school not found")
		} else {
			slog.Error("failed to delete school", "request_id", rid, "code", code, "error", err)
			response.Error(c, http.StatusInternalServerError, "failed to delete school")
		}
		return
	}
	response.Success(c, http.StatusOK, "school deleted", nil)
}
