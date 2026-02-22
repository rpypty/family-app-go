package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

type createTagRequest struct {
	Name string `json:"name"`
}

type updateTagRequest struct {
	Name string `json:"name"`
}

func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("tags.list: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("tags.list: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	tags, err := h.Expenses.ListTags(r.Context(), family.ID)
	if err != nil {
		h.log.InternalError("tags.list: list tags failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]tagResponse, 0, len(tags))
	for _, tag := range tags {
		response = append(response, tagResponse{
			ID:        tag.ID,
			Name:      tag.Name,
			CreatedAt: tag.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	var req createTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if len([]rune(strings.TrimSpace(req.Name))) > 50 {
		writeError(w, http.StatusBadRequest, "invalid_request", "name must be at most 50 characters")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("tags.create: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("tags.create: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	created, err := h.Expenses.CreateTag(r.Context(), family.ID, req.Name)
	if err != nil {
		h.log.InternalError("tags.create: create tag failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, tagResponse{
		ID:        created.ID,
		Name:      created.Name,
		CreatedAt: created.CreatedAt,
	})
}

func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	tagID := strings.TrimSpace(chi.URLParam(r, "id"))
	if tagID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("tags.delete: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("tags.delete: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if err := h.Expenses.DeleteTag(r.Context(), family.ID, tagID); err != nil {
		if errors.Is(err, expensesdomain.ErrTagNotFound) {
			h.log.BusinessError("tags.delete: tag not found", err, "user_id", user.ID, "family_id", family.ID, "tag_id", tagID)
			writeError(w, http.StatusNotFound, "tag_not_found", "tag not found")
			return
		}
		if errors.Is(err, expensesdomain.ErrTagInUse) {
			h.log.BusinessError("tags.delete: tag is in use", err, "user_id", user.ID, "family_id", family.ID, "tag_id", tagID)
			writeError(w, http.StatusConflict, "tag_in_use", "Tag is used by expenses")
			return
		}
		h.log.InternalError("tags.delete: delete tag failed", err, "user_id", user.ID, "family_id", family.ID, "tag_id", tagID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	tagID := strings.TrimSpace(chi.URLParam(r, "id"))
	if tagID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	var req updateTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if len([]rune(strings.TrimSpace(req.Name))) > 50 {
		writeError(w, http.StatusBadRequest, "invalid_request", "name must be at most 50 characters")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("tags.update: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("tags.update: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	updated, err := h.Expenses.UpdateTag(r.Context(), family.ID, tagID, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, expensesdomain.ErrTagNotFound):
			h.log.BusinessError("tags.update: tag not found", err, "user_id", user.ID, "family_id", family.ID, "tag_id", tagID)
			writeError(w, http.StatusNotFound, "tag_not_found", "tag not found")
		case errors.Is(err, expensesdomain.ErrTagNameTaken):
			h.log.BusinessError("tags.update: tag name already exists", err, "user_id", user.ID, "family_id", family.ID, "tag_id", tagID)
			writeError(w, http.StatusConflict, "tag_name_taken", "Tag name already exists")
		default:
			h.log.InternalError("tags.update: update tag failed", err, "user_id", user.ID, "family_id", family.ID, "tag_id", tagID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, tagResponse{
		ID:        updated.ID,
		Name:      updated.Name,
		CreatedAt: updated.CreatedAt,
	})
}

type tagResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
