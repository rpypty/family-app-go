package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	familydomain "family-app-go/internal/domain/family"
	todosdomain "family-app-go/internal/domain/todos"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

type todoListSettingsRequest struct {
	ArchiveCompleted *bool `json:"archive_completed"`
}

type createTodoListRequest struct {
	Title    string                   `json:"title"`
	Settings *todoListSettingsRequest `json:"settings"`
	Order    *int                     `json:"order"`
}

type updateTodoListRequest struct {
	Title       *string                  `json:"title"`
	Settings    *todoListSettingsRequest `json:"settings"`
	IsCollapsed *bool                    `json:"is_collapsed"`
	Order       *int                     `json:"order"`
}

type createTodoItemRequest struct {
	Title string `json:"title"`
}

type updateTodoItemRequest struct {
	Title       *string `json:"title"`
	IsCompleted *bool   `json:"is_completed"`
}

type todoListSettingsResponse struct {
	ArchiveCompleted bool `json:"archive_completed"`
}

type todoListResponse struct {
	ID             string                   `json:"id"`
	FamilyID       string                   `json:"family_id"`
	Title          string                   `json:"title"`
	IsCollapsed    bool                     `json:"is_collapsed"`
	Order          int                      `json:"order"`
	CreatedAt      time.Time                `json:"created_at"`
	Settings       todoListSettingsResponse `json:"settings"`
	ItemsTotal     int64                    `json:"items_total"`
	ItemsCompleted int64                    `json:"items_completed"`
	ItemsArchived  int64                    `json:"items_archived"`
	Items          *[]todoItemResponse      `json:"items,omitempty"`
}

type todoListListResponse struct {
	Items []todoListResponse `json:"items"`
	Total int64              `json:"total"`
}

type todoItemResponse struct {
	ID          string                   `json:"id"`
	ListID      string                   `json:"list_id"`
	Title       string                   `json:"title"`
	IsCompleted bool                     `json:"is_completed"`
	IsArchived  bool                     `json:"is_archived"`
	CreatedAt   time.Time                `json:"created_at"`
	CompletedAt *time.Time               `json:"completed_at"`
	CompletedBy *todoCompletedByResponse `json:"completed_by"`
}

type todoCompletedByResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	AvatarURL *string `json:"avatar_url"`
}

type todoItemListResponse struct {
	Items []todoItemResponse `json:"items"`
	Total int64              `json:"total"`
}

func (h *Handlers) ListTodoLists(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("todos.list_lists: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.list_lists: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	limit, err := parseIntParam(query.Get("limit"), 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
		return
	}
	offset, err := parseIntParam(query.Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid offset")
		return
	}
	includeItems, err := parseBoolParam(query.Get("include_items"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid include_items")
		return
	}

	itemsArchived, err := parseArchivedFilter(query.Get("items_archived"), todosdomain.ArchivedExclude)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid items_archived")
		return
	}

	filter := todosdomain.ListFilter{
		Query:  strings.TrimSpace(query.Get("q")),
		Limit:  limit,
		Offset: offset,
	}

	items, total, err := h.Todos.ListTodoLists(r.Context(), family.ID, filter, includeItems, itemsArchived)
	if err != nil {
		h.log.InternalError("todos.list_lists: list todo lists failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]todoListResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toTodoListResponse(item, includeItems))
	}

	writeJSON(w, http.StatusOK, todoListListResponse{
		Items: response,
		Total: total,
	})
}

func (h *Handlers) CreateTodoList(w http.ResponseWriter, r *http.Request) {
	var req createTodoListRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	if req.Order != nil && *req.Order < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "order must be non-negative")
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
			h.log.BusinessError("todos.create_list: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.create_list: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	archiveCompleted := false
	if req.Settings != nil && req.Settings.ArchiveCompleted != nil {
		archiveCompleted = *req.Settings.ArchiveCompleted
	}

	list, err := h.Todos.CreateTodoList(r.Context(), todosdomain.CreateTodoListInput{
		FamilyID:         family.ID,
		Title:            req.Title,
		ArchiveCompleted: archiveCompleted,
		Order:            req.Order,
	})
	if err != nil {
		h.log.InternalError("todos.create_list: create todo list failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	counts, err := h.Todos.CountItemsByListID(r.Context(), list.ID)
	if err != nil {
		h.log.InternalError("todos.create_list: count items failed", err, "user_id", user.ID, "family_id", family.ID, "list_id", list.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, todoListResponse{
		ID:             list.ID,
		FamilyID:       list.FamilyID,
		Title:          list.Title,
		IsCollapsed:    list.IsCollapsed,
		Order:          list.Order,
		CreatedAt:      list.CreatedAt,
		Settings:       todoListSettingsResponse{ArchiveCompleted: list.ArchiveCompleted},
		ItemsTotal:     counts.ItemsTotal,
		ItemsCompleted: counts.ItemsCompleted,
		ItemsArchived:  counts.ItemsArchived,
	})
}

func (h *Handlers) UpdateTodoList(w http.ResponseWriter, r *http.Request) {
	var req updateTodoListRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	listID := strings.TrimSpace(chi.URLParam(r, "list_id"))
	if listID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "list_id is required")
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
			h.log.BusinessError("todos.update_list: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.update_list: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	var archiveCompleted *bool
	if req.Settings != nil {
		archiveCompleted = req.Settings.ArchiveCompleted
	}
	if req.Title == nil && archiveCompleted == nil && req.IsCollapsed == nil && req.Order == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "no fields to update")
		return
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	if req.Order != nil && *req.Order < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "order must be non-negative")
		return
	}

	list, err := h.Todos.UpdateTodoList(r.Context(), todosdomain.UpdateTodoListInput{
		ID:               listID,
		FamilyID:         family.ID,
		Title:            req.Title,
		ArchiveCompleted: archiveCompleted,
		IsCollapsed:      req.IsCollapsed,
		Order:            req.Order,
	})
	if err != nil {
		switch {
		case errors.Is(err, todosdomain.ErrTodoListNotFound):
			h.log.BusinessError("todos.update_list: todo list not found", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
			writeError(w, http.StatusNotFound, "todo_list_not_found", "todo list not found")
		default:
			h.log.InternalError("todos.update_list: update todo list failed", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	counts, err := h.Todos.CountItemsByListID(r.Context(), list.ID)
	if err != nil {
		h.log.InternalError("todos.update_list: count items failed", err, "user_id", user.ID, "family_id", family.ID, "list_id", list.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, todoListResponse{
		ID:             list.ID,
		FamilyID:       list.FamilyID,
		Title:          list.Title,
		IsCollapsed:    list.IsCollapsed,
		Order:          list.Order,
		CreatedAt:      list.CreatedAt,
		Settings:       todoListSettingsResponse{ArchiveCompleted: list.ArchiveCompleted},
		ItemsTotal:     counts.ItemsTotal,
		ItemsCompleted: counts.ItemsCompleted,
		ItemsArchived:  counts.ItemsArchived,
	})
}

func (h *Handlers) DeleteTodoList(w http.ResponseWriter, r *http.Request) {
	listID := strings.TrimSpace(chi.URLParam(r, "list_id"))
	if listID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "list_id is required")
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
			h.log.BusinessError("todos.delete_list: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.delete_list: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if err := h.Todos.DeleteTodoList(r.Context(), family.ID, listID); err != nil {
		if errors.Is(err, todosdomain.ErrTodoListNotFound) {
			h.log.BusinessError("todos.delete_list: todo list not found", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
			writeError(w, http.StatusNotFound, "todo_list_not_found", "todo list not found")
			return
		}
		h.log.InternalError("todos.delete_list: delete todo list failed", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListTodoItems(w http.ResponseWriter, r *http.Request) {
	listID := strings.TrimSpace(chi.URLParam(r, "list_id"))
	if listID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "list_id is required")
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
			h.log.BusinessError("todos.list_items: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.list_items: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	archived, err := parseArchivedFilter(r.URL.Query().Get("archived"), todosdomain.ArchivedExclude)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid archived")
		return
	}

	items, total, err := h.Todos.ListTodoItems(r.Context(), family.ID, listID, archived)
	if err != nil {
		if errors.Is(err, todosdomain.ErrTodoListNotFound) {
			h.log.BusinessError("todos.list_items: todo list not found", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
			writeError(w, http.StatusNotFound, "todo_list_not_found", "todo list not found")
			return
		}
		h.log.InternalError("todos.list_items: list todo items failed", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]todoItemResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toTodoItemResponse(item))
	}

	writeJSON(w, http.StatusOK, todoItemListResponse{
		Items: response,
		Total: total,
	})
}

func (h *Handlers) CreateTodoItem(w http.ResponseWriter, r *http.Request) {
	var req createTodoItemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	listID := strings.TrimSpace(chi.URLParam(r, "list_id"))
	if listID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "list_id is required")
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
			h.log.BusinessError("todos.create_item: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.create_item: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	item, err := h.Todos.CreateTodoItem(r.Context(), family.ID, todosdomain.CreateTodoItemInput{
		ListID: listID,
		Title:  req.Title,
	})
	if err != nil {
		if errors.Is(err, todosdomain.ErrTodoListNotFound) {
			h.log.BusinessError("todos.create_item: todo list not found", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
			writeError(w, http.StatusNotFound, "todo_list_not_found", "todo list not found")
			return
		}
		h.log.InternalError("todos.create_item: create todo item failed", err, "user_id", user.ID, "family_id", family.ID, "list_id", listID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toTodoItemResponse(*item))
}

func (h *Handlers) UpdateTodoItem(w http.ResponseWriter, r *http.Request) {
	var req updateTodoItemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	itemID := strings.TrimSpace(chi.URLParam(r, "item_id"))
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "item_id is required")
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
			h.log.BusinessError("todos.update_item: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.update_item: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if req.Title == nil && req.IsCompleted == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "no fields to update")
		return
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	var completedBy *todosdomain.UserSnapshot
	if req.IsCompleted != nil && *req.IsCompleted {
		completedBy = &todosdomain.UserSnapshot{
			ID:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			AvatarURL: user.AvatarURL,
		}
	}

	item, err := h.Todos.UpdateTodoItem(r.Context(), todosdomain.UpdateTodoItemInput{
		ID:          itemID,
		FamilyID:    family.ID,
		Title:       req.Title,
		IsCompleted: req.IsCompleted,
		CompletedBy: completedBy,
	})
	if err != nil {
		switch {
		case errors.Is(err, todosdomain.ErrTodoItemNotFound):
			h.log.BusinessError("todos.update_item: todo item not found", err, "user_id", user.ID, "family_id", family.ID, "item_id", itemID)
			writeError(w, http.StatusNotFound, "todo_item_not_found", "todo item not found")
		default:
			h.log.InternalError("todos.update_item: update todo item failed", err, "user_id", user.ID, "family_id", family.ID, "item_id", itemID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, toTodoItemResponse(*item))
}

func (h *Handlers) DeleteTodoItem(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimSpace(chi.URLParam(r, "item_id"))
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "item_id is required")
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
			h.log.BusinessError("todos.delete_item: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("todos.delete_item: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if err := h.Todos.DeleteTodoItem(r.Context(), family.ID, itemID); err != nil {
		if errors.Is(err, todosdomain.ErrTodoItemNotFound) {
			h.log.BusinessError("todos.delete_item: todo item not found", err, "user_id", user.ID, "family_id", family.ID, "item_id", itemID)
			writeError(w, http.StatusNotFound, "todo_item_not_found", "todo item not found")
			return
		}
		h.log.InternalError("todos.delete_item: delete todo item failed", err, "user_id", user.ID, "family_id", family.ID, "item_id", itemID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseArchivedFilter(value string, fallback todosdomain.ArchivedFilter) (todosdomain.ArchivedFilter, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case string(todosdomain.ArchivedExclude):
		return todosdomain.ArchivedExclude, nil
	case string(todosdomain.ArchivedOnly):
		return todosdomain.ArchivedOnly, nil
	case string(todosdomain.ArchivedAll):
		return todosdomain.ArchivedAll, nil
	default:
		return "", errors.New("invalid archived")
	}
}

func parseBoolParam(value string, fallback bool) (bool, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, errors.New("invalid bool")
	}
}

func toTodoListResponse(item todosdomain.ListWithItems, includeItems bool) todoListResponse {
	response := todoListResponse{
		ID:             item.List.ID,
		FamilyID:       item.List.FamilyID,
		Title:          item.List.Title,
		IsCollapsed:    item.List.IsCollapsed,
		Order:          item.List.Order,
		CreatedAt:      item.List.CreatedAt,
		Settings:       todoListSettingsResponse{ArchiveCompleted: item.List.ArchiveCompleted},
		ItemsTotal:     item.Counts.ItemsTotal,
		ItemsCompleted: item.Counts.ItemsCompleted,
		ItemsArchived:  item.Counts.ItemsArchived,
	}

	if includeItems {
		items := make([]todoItemResponse, 0, len(item.Items))
		for _, todo := range item.Items {
			items = append(items, toTodoItemResponse(todo))
		}
		response.Items = &items
	}

	return response
}

func toTodoItemResponse(item todosdomain.TodoItem) todoItemResponse {
	var completedBy *todoCompletedByResponse
	if item.CompletedByID != nil && strings.TrimSpace(*item.CompletedByID) != "" {
		completedBy = &todoCompletedByResponse{
			ID:        *item.CompletedByID,
			Name:      valueOrEmpty(item.CompletedByName),
			Email:     valueOrEmpty(item.CompletedByEmail),
			AvatarURL: item.CompletedByAvatarURL,
		}
	}

	return todoItemResponse{
		ID:          item.ID,
		ListID:      item.ListID,
		Title:       item.Title,
		IsCompleted: item.IsCompleted,
		IsArchived:  item.IsArchived,
		CreatedAt:   item.CreatedAt,
		CompletedAt: item.CompletedAt,
		CompletedBy: completedBy,
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
