package httpserver

import (
	"net/http"
	"time"

	"family-app-go/internal/config"
	"family-app-go/internal/transport/httpserver/handler"
	authmw "family-app-go/internal/transport/httpserver/middleware"
	"family-app-go/pkg/logger"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg config.Config, handlers *handler.Handlers, profiles authmw.ProfileSaver, log logger.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(authmw.NewCORS([]string{"http://localhost:5173"}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handlers.Common.Health)

		auth := authmw.NewSupabaseAuth(cfg.Supabase, profiles, log)
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware)

			r.Get("/auth/me", handlers.Common.AuthMe)
			if cfg.OfflineSyncEnabled {
				r.Post("/sync", handlers.Common.SyncBatch)
			}

			r.Get("/analytics/summary", handlers.Expenses.AnalyticsSummary)
			r.Get("/analytics/timeseries", handlers.Expenses.AnalyticsTimeseries)
			r.Get("/analytics/by-category", handlers.Expenses.AnalyticsByCategory)
			r.Get("/top_categories", handlers.Expenses.TopCategories)
			r.Get("/reports/monthly", handlers.Expenses.ReportsMonthly)
			r.Get("/reports/compare", handlers.Expenses.ReportsCompare)

			r.Get("/families/me", handlers.Common.GetFamilyMe)
			r.Post("/families", handlers.Common.CreateFamily)
			r.Post("/families/join", handlers.Common.JoinFamily)
			r.Post("/families/leave", handlers.Common.LeaveFamily)
			r.Patch("/families/me", handlers.Common.UpdateFamily)
			r.Get("/families/me/members", handlers.Common.ListFamilyMembers)
			r.Delete("/families/me/members/{user_id}", handlers.Common.RemoveFamilyMember)

			r.Get("/expenses", handlers.Expenses.ListExpenses)
			r.Post("/expenses", handlers.Expenses.CreateExpense)
			r.Put("/expenses/{id}", handlers.Expenses.UpdateExpense)
			r.Delete("/expenses/{id}", handlers.Expenses.DeleteExpense)

			r.Get("/categories", handlers.Expenses.ListCategories)
			r.Post("/categories", handlers.Expenses.CreateCategory)
			r.Patch("/categories/{id}", handlers.Expenses.UpdateCategory)
			r.Delete("/categories/{id}", handlers.Expenses.DeleteCategory)

			r.Get("/todo-lists", handlers.Todos.ListTodoLists)
			r.Post("/todo-lists", handlers.Todos.CreateTodoList)
			r.Patch("/todo-lists/{list_id}", handlers.Todos.UpdateTodoList)
			r.Delete("/todo-lists/{list_id}", handlers.Todos.DeleteTodoList)
			r.Get("/todo-lists/{list_id}/items", handlers.Todos.ListTodoItems)
			r.Post("/todo-lists/{list_id}/items", handlers.Todos.CreateTodoItem)
			r.Patch("/todo-items/{item_id}", handlers.Todos.UpdateTodoItem)
			r.Delete("/todo-items/{item_id}", handlers.Todos.DeleteTodoItem)

			r.Get("/gym/entries", handlers.Gym.ListGymEntries)
			r.Post("/gym/entries", handlers.Gym.CreateGymEntry)
			r.Put("/gym/entries/{id}", handlers.Gym.UpdateGymEntry)
			r.Delete("/gym/entries/{id}", handlers.Gym.DeleteGymEntry)

			r.Get("/gym/workouts", handlers.Gym.ListWorkouts)
			r.Get("/gym/workouts/{id}", handlers.Gym.GetWorkout)
			r.Post("/gym/workouts", handlers.Gym.CreateWorkout)
			r.Put("/gym/workouts/{id}", handlers.Gym.UpdateWorkout)
			r.Delete("/gym/workouts/{id}", handlers.Gym.DeleteWorkout)

			r.Get("/gym/templates", handlers.Gym.ListTemplates)
			r.Post("/gym/templates", handlers.Gym.CreateTemplate)
			r.Put("/gym/templates/{id}", handlers.Gym.UpdateTemplate)
			r.Delete("/gym/templates/{id}", handlers.Gym.DeleteTemplate)

			r.Get("/gym/exercises", handlers.Gym.ListExercises)
		})
	})

	return r
}
