package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	familydomain "family-app-go/internal/domain/family"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

type createFamilyRequest struct {
	Name string `json:"name"`
}

type joinFamilyRequest struct {
	Code string `json:"code"`
}

type updateFamilyRequest struct {
	Name string `json:"name"`
}

func (h *Handlers) GetFamilyMe(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	result, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("families.get_me: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("families.get_me: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toFamilyResponse(result))
}

func (h *Handlers) CreateFamily(w http.ResponseWriter, r *http.Request) {
	var req createFamilyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	result, err := h.Families.CreateFamily(r.Context(), user.ID, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, familydomain.ErrAlreadyInFamily):
			h.log.BusinessError("families.create: user already in family", err, "user_id", user.ID)
			writeError(w, http.StatusConflict, "already_in_family", "already in family")
		default:
			h.log.InternalError("families.create: create family failed", err, "user_id", user.ID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, toFamilyResponse(result))
}

func (h *Handlers) JoinFamily(w http.ResponseWriter, r *http.Request) {
	var req joinFamilyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "code is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	result, err := h.Families.JoinFamily(r.Context(), user.ID, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, familydomain.ErrFamilyCodeNotFound):
			h.log.BusinessError("families.join: family code not found", err, "user_id", user.ID, "code", req.Code)
			writeError(w, http.StatusNotFound, "family_code_not_found", "family code not found")
		case errors.Is(err, familydomain.ErrAlreadyInFamily):
			h.log.BusinessError("families.join: user already in family", err, "user_id", user.ID)
			writeError(w, http.StatusConflict, "already_in_family", "already in family")
		default:
			h.log.InternalError("families.join: join family failed", err, "user_id", user.ID, "code", req.Code)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, toFamilyResponse(result))
}

func (h *Handlers) LeaveFamily(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	if err := h.Families.LeaveFamily(r.Context(), user.ID); err != nil {
		switch {
		case errors.Is(err, familydomain.ErrFamilyNotFound):
			h.log.BusinessError("families.leave: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
		default:
			h.log.InternalError("families.leave: leave family failed", err, "user_id", user.ID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) UpdateFamily(w http.ResponseWriter, r *http.Request) {
	var req updateFamilyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	result, err := h.Families.UpdateFamily(r.Context(), user.ID, req.Name)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("families.update: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("families.update: update family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toFamilyResponse(result))
}

func (h *Handlers) ListFamilyMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	members, err := h.Families.ListMembersWithProfiles(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("families.list_members: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("families.list_members: list members failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]familyMemberResponse, 0, len(members))
	for _, member := range members {
		response = append(response, familyMemberResponse{
			UserID:    member.UserID,
			Role:      member.Role,
			JoinedAt:  member.JoinedAt,
			Email:     member.Email,
			AvatarURL: member.AvatarURL,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) RemoveFamilyMember(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	memberID := strings.TrimSpace(chi.URLParam(r, "user_id"))
	if memberID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}

	if err := h.Families.RemoveMember(r.Context(), user.ID, memberID); err != nil {
		switch {
		case errors.Is(err, familydomain.ErrFamilyNotFound):
			h.log.BusinessError("families.remove_member: family not found", err, "actor_id", user.ID, "member_id", memberID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
		case errors.Is(err, familydomain.ErrMemberNotFound):
			h.log.BusinessError("families.remove_member: member not found", err, "actor_id", user.ID, "member_id", memberID)
			writeError(w, http.StatusNotFound, "member_not_found", "member not found")
		case errors.Is(err, familydomain.ErrNotOwner):
			h.log.BusinessError("families.remove_member: actor is not owner", err, "actor_id", user.ID, "member_id", memberID)
			writeError(w, http.StatusForbidden, "not_owner", "only owner can remove members")
		case errors.Is(err, familydomain.ErrCannotRemoveOwner):
			h.log.BusinessError("families.remove_member: cannot remove owner", err, "actor_id", user.ID, "member_id", memberID)
			writeError(w, http.StatusConflict, "cannot_remove_owner", "cannot remove owner")
		default:
			h.log.InternalError("families.remove_member: remove member failed", err, "actor_id", user.ID, "member_id", memberID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func notImplemented(w http.ResponseWriter) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "not implemented")
}

type familyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}

type familyMemberResponse struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
	Email     *string   `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
}

func toFamilyResponse(familyModel *familydomain.Family) familyResponse {
	return familyResponse{
		ID:        familyModel.ID,
		Name:      familyModel.Name,
		Code:      familyModel.Code,
		OwnerID:   familyModel.OwnerID,
		CreatedAt: familyModel.CreatedAt,
	}
}
