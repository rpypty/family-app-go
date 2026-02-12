package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	familydomain "family-app-go/internal/domain/family"
	gymdomain "family-app-go/internal/domain/gym"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

// GymEntry handlers

type createGymEntryRequest struct {
	Date     string  `json:"date"`
	Exercise string  `json:"exercise"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
}

type updateGymEntryRequest struct {
	Date     string  `json:"date"`
	Exercise string  `json:"exercise"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
}

func (h *Handlers) ListGymEntries(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	query := r.URL.Query()
	from, err := parseDateParam(query.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid from date")
		return
	}
	to, err := parseDateParam(query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid to date")
		return
	}

	limit, err := parseIntParam(query.Get("limit"), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
		return
	}
	offset, err := parseIntParam(query.Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid offset")
		return
	}

	filter := gymdomain.ListFilter{
		From:   from,
		To:     to,
		Limit:  limit,
		Offset: offset,
	}

	items, total, err := h.Gym.ListGymEntries(r.Context(), user.ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]gymEntryResponse, 0, len(items))
	for _, entry := range items {
		response = append(response, toGymEntryResponse(entry))
	}

	writeJSON(w, http.StatusOK, gymEntryListResponse{
		Items: response,
		Total: total,
	})
}

func (h *Handlers) CreateGymEntry(w http.ResponseWriter, r *http.Request) {
	var req createGymEntryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	date, err := parseDateRequired(req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
		return
	}
	if strings.TrimSpace(req.Exercise) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "exercise is required")
		return
	}

	input := gymdomain.CreateGymEntryInput{
		FamilyID: family.ID,
		UserID:   user.ID,
		Date:     date,
		Exercise: req.Exercise,
		WeightKg: req.WeightKg,
		Reps:     req.Reps,
	}

	created, err := h.Gym.CreateGymEntry(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toGymEntryResponse(*created))
}

func (h *Handlers) UpdateGymEntry(w http.ResponseWriter, r *http.Request) {
	var req updateGymEntryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	entryID := strings.TrimSpace(chi.URLParam(r, "id"))
	if entryID == "" {
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
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	date, err := parseDateRequired(req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
		return
	}
	if strings.TrimSpace(req.Exercise) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "exercise is required")
		return
	}

	input := gymdomain.UpdateGymEntryInput{
		ID:       entryID,
		UserID:   user.ID,
		Date:     date,
		Exercise: req.Exercise,
		WeightKg: req.WeightKg,
		Reps:     req.Reps,
	}

	updated, err := h.Gym.UpdateGymEntry(r.Context(), input)
	if err != nil {
		if errors.Is(err, gymdomain.ErrGymEntryNotFound) {
			writeError(w, http.StatusNotFound, "gym_entry_not_found", "gym entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toGymEntryResponse(*updated))
}

func (h *Handlers) DeleteGymEntry(w http.ResponseWriter, r *http.Request) {
	entryID := strings.TrimSpace(chi.URLParam(r, "id"))
	if entryID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	if err := h.Gym.DeleteGymEntry(r.Context(), user.ID, entryID); err != nil {
		if errors.Is(err, gymdomain.ErrGymEntryNotFound) {
			writeError(w, http.StatusNotFound, "gym_entry_not_found", "gym entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Workout handlers

type createWorkoutSetRequest struct {
	Exercise string  `json:"exercise"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
}

type createWorkoutRequest struct {
	Date string                    `json:"date"`
	Name string                    `json:"name"`
	Sets []createWorkoutSetRequest `json:"sets"`
}

type updateWorkoutRequest struct {
	Date string                    `json:"date"`
	Name string                    `json:"name"`
	Sets []createWorkoutSetRequest `json:"sets"`
}

func (h *Handlers) ListWorkouts(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	query := r.URL.Query()
	from, err := parseDateParam(query.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid from date")
		return
	}
	to, err := parseDateParam(query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid to date")
		return
	}

	limit, err := parseIntParam(query.Get("limit"), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
		return
	}
	offset, err := parseIntParam(query.Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid offset")
		return
	}

	filter := gymdomain.ListFilter{
		From:   from,
		To:     to,
		Limit:  limit,
		Offset: offset,
	}

	items, total, err := h.Gym.ListWorkouts(r.Context(), user.ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]workoutResponse, 0, len(items))
	for _, workout := range items {
		response = append(response, toWorkoutResponse(workout))
	}

	writeJSON(w, http.StatusOK, workoutListResponse{
		Items: response,
		Total: total,
	})
}

func (h *Handlers) GetWorkout(w http.ResponseWriter, r *http.Request) {
	workoutID := strings.TrimSpace(chi.URLParam(r, "id"))
	if workoutID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	workout, err := h.Gym.GetWorkoutByID(r.Context(), user.ID, workoutID)
	if err != nil {
		if errors.Is(err, gymdomain.ErrWorkoutNotFound) {
			writeError(w, http.StatusNotFound, "workout_not_found", "workout not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toWorkoutResponse(*workout))
}

func (h *Handlers) CreateWorkout(w http.ResponseWriter, r *http.Request) {
	var req createWorkoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	date, err := parseDateRequired(req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	sets := make([]gymdomain.CreateWorkoutSetInput, 0, len(req.Sets))
	for _, setReq := range req.Sets {
		sets = append(sets, gymdomain.CreateWorkoutSetInput{
			Exercise: setReq.Exercise,
			WeightKg: setReq.WeightKg,
			Reps:     setReq.Reps,
		})
	}

	input := gymdomain.CreateWorkoutInput{
		FamilyID: family.ID,
		UserID:   user.ID,
		Date:     date,
		Name:     req.Name,
		Sets:     sets,
	}

	created, err := h.Gym.CreateWorkout(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toWorkoutResponse(*created))
}

func (h *Handlers) UpdateWorkout(w http.ResponseWriter, r *http.Request) {
	var req updateWorkoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	workoutID := strings.TrimSpace(chi.URLParam(r, "id"))
	if workoutID == "" {
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
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	date, err := parseDateRequired(req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	sets := make([]gymdomain.CreateWorkoutSetInput, 0, len(req.Sets))
	for _, setReq := range req.Sets {
		sets = append(sets, gymdomain.CreateWorkoutSetInput{
			Exercise: setReq.Exercise,
			WeightKg: setReq.WeightKg,
			Reps:     setReq.Reps,
		})
	}

	input := gymdomain.UpdateWorkoutInput{
		ID:     workoutID,
		UserID: user.ID,
		Date:   date,
		Name:   req.Name,
		Sets:   sets,
	}

	updated, err := h.Gym.UpdateWorkout(r.Context(), input)
	if err != nil {
		if errors.Is(err, gymdomain.ErrWorkoutNotFound) {
			writeError(w, http.StatusNotFound, "workout_not_found", "workout not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toWorkoutResponse(*updated))
}

func (h *Handlers) DeleteWorkout(w http.ResponseWriter, r *http.Request) {
	workoutID := strings.TrimSpace(chi.URLParam(r, "id"))
	if workoutID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	if err := h.Gym.DeleteWorkout(r.Context(), user.ID, workoutID); err != nil {
		if errors.Is(err, gymdomain.ErrWorkoutNotFound) {
			writeError(w, http.StatusNotFound, "workout_not_found", "workout not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// WorkoutTemplate handlers

type createTemplateExerciseRequest struct {
	Name string `json:"name"`
	Reps int    `json:"reps"`
	Sets int    `json:"sets"`
}

type createTemplateRequest struct {
	Name      string                          `json:"name"`
	Exercises []createTemplateExerciseRequest `json:"exercises"`
}

type updateTemplateRequest struct {
	Name      string                          `json:"name"`
	Exercises []createTemplateExerciseRequest `json:"exercises"`
}

func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	items, err := h.Gym.ListTemplates(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]templateResponse, 0, len(items))
	for _, template := range items {
		response = append(response, toTemplateResponse(template))
	}

	writeJSON(w, http.StatusOK, templateListResponse{Items: response})
}

func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	exercises := make([]gymdomain.CreateTemplateExerciseInput, 0, len(req.Exercises))
	for _, exReq := range req.Exercises {
		exercises = append(exercises, gymdomain.CreateTemplateExerciseInput{
			Name: exReq.Name,
			Reps: exReq.Reps,
			Sets: exReq.Sets,
		})
	}

	input := gymdomain.CreateTemplateInput{
		FamilyID:  family.ID,
		UserID:    user.ID,
		Name:      req.Name,
		Exercises: exercises,
	}

	created, err := h.Gym.CreateTemplate(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toTemplateResponse(*created))
}

func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	var req updateTemplateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	templateID := strings.TrimSpace(chi.URLParam(r, "id"))
	if templateID == "" {
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
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	exercises := make([]gymdomain.CreateTemplateExerciseInput, 0, len(req.Exercises))
	for _, exReq := range req.Exercises {
		exercises = append(exercises, gymdomain.CreateTemplateExerciseInput{
			Name: exReq.Name,
			Reps: exReq.Reps,
			Sets: exReq.Sets,
		})
	}

	input := gymdomain.UpdateTemplateInput{
		ID:        templateID,
		UserID:    user.ID,
		Name:      req.Name,
		Exercises: exercises,
	}

	updated, err := h.Gym.UpdateTemplate(r.Context(), input)
	if err != nil {
		if errors.Is(err, gymdomain.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, "template_not_found", "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toTemplateResponse(*updated))
}

func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(chi.URLParam(r, "id"))
	if templateID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	if err := h.Gym.DeleteTemplate(r.Context(), user.ID, templateID); err != nil {
		if errors.Is(err, gymdomain.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, "template_not_found", "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Exercise list handler

func (h *Handlers) ListExercises(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	exercises, err := h.Gym.ListExercises(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, exerciseListResponse{Exercises: exercises})
}

// Response types

type gymEntryResponse struct {
	ID        string    `json:"id"`
	FamilyID  string    `json:"family_id"`
	UserID    string    `json:"user_id"`
	Date      string    `json:"date"`
	Exercise  string    `json:"exercise"`
	WeightKg  float64   `json:"weight_kg"`
	Reps      int       `json:"reps"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type gymEntryListResponse struct {
	Items []gymEntryResponse `json:"items"`
	Total int64              `json:"total"`
}

type workoutSetResponse struct {
	ID       string  `json:"id"`
	Exercise string  `json:"exercise"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
}

type workoutResponse struct {
	ID        string               `json:"id"`
	FamilyID  string               `json:"family_id"`
	UserID    string               `json:"user_id"`
	Date      string               `json:"date"`
	Name      string               `json:"name"`
	Sets      []workoutSetResponse `json:"sets"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}

type workoutListResponse struct {
	Items []workoutResponse `json:"items"`
	Total int64             `json:"total"`
}

type templateExerciseResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Reps int    `json:"reps"`
	Sets int    `json:"sets"`
}

type templateResponse struct {
	ID        string                     `json:"id"`
	FamilyID  string                     `json:"family_id"`
	UserID    string                     `json:"user_id"`
	Name      string                     `json:"name"`
	Exercises []templateExerciseResponse `json:"exercises"`
	CreatedAt time.Time                  `json:"created_at"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type templateListResponse struct {
	Items []templateResponse `json:"items"`
}

type exerciseListResponse struct {
	Exercises []string `json:"exercises"`
}

// Response mappers

func toGymEntryResponse(entry gymdomain.GymEntry) gymEntryResponse {
	return gymEntryResponse{
		ID:        entry.ID,
		FamilyID:  entry.FamilyID,
		UserID:    entry.UserID,
		Date:      entry.Date.Format("2006-01-02"),
		Exercise:  entry.Exercise,
		WeightKg:  entry.WeightKg,
		Reps:      entry.Reps,
		CreatedAt: entry.CreatedAt,
		UpdatedAt: entry.UpdatedAt,
	}
}

func toWorkoutResponse(workout gymdomain.WorkoutWithSets) workoutResponse {
	sets := make([]workoutSetResponse, 0, len(workout.Sets))
	for _, set := range workout.Sets {
		sets = append(sets, workoutSetResponse{
			ID:       set.ID,
			Exercise: set.Exercise,
			WeightKg: set.WeightKg,
			Reps:     set.Reps,
		})
	}

	return workoutResponse{
		ID:        workout.ID,
		FamilyID:  workout.FamilyID,
		UserID:    workout.UserID,
		Date:      workout.Date.Format("2006-01-02"),
		Name:      workout.Name,
		Sets:      sets,
		CreatedAt: workout.CreatedAt,
		UpdatedAt: workout.UpdatedAt,
	}
}

func toTemplateResponse(template gymdomain.TemplateWithExercises) templateResponse {
	exercises := make([]templateExerciseResponse, 0, len(template.Exercises))
	for _, ex := range template.Exercises {
		exercises = append(exercises, templateExerciseResponse{
			ID:   ex.ID,
			Name: ex.Name,
			Reps: ex.Reps,
			Sets: ex.Sets,
		})
	}

	return templateResponse{
		ID:        template.ID,
		FamilyID:  template.FamilyID,
		UserID:    template.UserID,
		Name:      template.Name,
		Exercises: exercises,
		CreatedAt: template.CreatedAt,
		UpdatedAt: template.UpdatedAt,
	}
}
