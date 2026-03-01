package expenses

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

type createCategoryRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
	Emoji *string `json:"emoji"`
}

type updateCategoryRequest struct {
	Name  string                 `json:"name"`
	Color optionalNullableString `json:"color"`
	Emoji optionalNullableString `json:"emoji"`
}

type optionalNullableString struct {
	Set   bool
	Value *string
}

func (o *optionalNullableString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	o.Value = &value
	return nil
}

func (h *Handlers) ListCategories(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("categories.list: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("categories.list: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	categories, err := h.Expenses.ListCategories(r.Context(), family.ID)
	if err != nil {
		h.log.InternalError("categories.list: list categories failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		response = append(response, categoryResponse{
			ID:        category.ID,
			Name:      category.Name,
			Color:     category.Color,
			Emoji:     category.Emoji,
			CreatedAt: category.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req createCategoryRequest
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
			h.log.BusinessError("categories.create: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("categories.create: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	created, err := h.Expenses.CreateCategory(r.Context(), expensesdomain.CreateCategoryInput{
		FamilyID: family.ID,
		Name:     req.Name,
		Color:    req.Color,
		Emoji:    req.Emoji,
	})
	if err != nil {
		if writeCategoryValidationError(w, err) {
			h.log.BusinessError("categories.create: validation failed", err, "user_id", user.ID, "family_id", family.ID)
			return
		}
		h.log.InternalError("categories.create: create category failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, categoryResponse{
		ID:        created.ID,
		Name:      created.Name,
		Color:     created.Color,
		Emoji:     created.Emoji,
		CreatedAt: created.CreatedAt,
	})
}

func (h *Handlers) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	categoryID := strings.TrimSpace(chi.URLParam(r, "id"))
	if categoryID == "" {
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
			h.log.BusinessError("categories.delete: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("categories.delete: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if err := h.Expenses.DeleteCategory(r.Context(), family.ID, categoryID); err != nil {
		if errors.Is(err, expensesdomain.ErrCategoryNotFound) {
			h.log.BusinessError("categories.delete: category not found", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
			writeError(w, http.StatusNotFound, "category_not_found", "category not found")
			return
		}
		if errors.Is(err, expensesdomain.ErrCategoryInUse) {
			h.log.BusinessError("categories.delete: category is in use", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
			writeError(w, http.StatusConflict, "category_in_use", "Category is used by expenses")
			return
		}
		h.log.InternalError("categories.delete: delete category failed", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	categoryID := strings.TrimSpace(chi.URLParam(r, "id"))
	if categoryID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	var req updateCategoryRequest
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
			h.log.BusinessError("categories.update: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("categories.update: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	updated, err := h.Expenses.UpdateCategory(r.Context(), expensesdomain.UpdateCategoryInput{
		FamilyID:   family.ID,
		CategoryID: categoryID,
		Name:       req.Name,
		Color: expensesdomain.OptionalNullableString{
			Set:   req.Color.Set,
			Value: req.Color.Value,
		},
		Emoji: expensesdomain.OptionalNullableString{
			Set:   req.Emoji.Set,
			Value: req.Emoji.Value,
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, expensesdomain.ErrCategoryNotFound):
			h.log.BusinessError("categories.update: category not found", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
			writeError(w, http.StatusNotFound, "category_not_found", "category not found")
		case errors.Is(err, expensesdomain.ErrCategoryNameTaken):
			h.log.BusinessError("categories.update: category name already exists", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
			writeError(w, http.StatusConflict, "category_name_taken", "Category name already exists")
		case writeCategoryValidationError(w, err):
			h.log.BusinessError("categories.update: validation failed", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
		default:
			h.log.InternalError("categories.update: update category failed", err, "user_id", user.ID, "family_id", family.ID, "category_id", categoryID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, categoryResponse{
		ID:        updated.ID,
		Name:      updated.Name,
		Color:     updated.Color,
		Emoji:     updated.Emoji,
		CreatedAt: updated.CreatedAt,
	})
}

type categoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	Emoji     *string   `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

func writeCategoryValidationError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, expensesdomain.ErrInvalidCategoryColor):
		writeError(w, http.StatusBadRequest, "invalid_request", "color must be null or #RRGGBB")
		return true
	case errors.Is(err, expensesdomain.ErrInvalidCategoryEmoji):
		writeError(w, http.StatusBadRequest, "invalid_request", "emoji must be a single emoji grapheme")
		return true
	default:
		return false
	}
}
