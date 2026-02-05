package httpserver

import (
	"net/http"
	"time"

	"family-app-go/internal/config"
	"family-app-go/internal/transport/httpserver/handler"
	authmw "family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg config.Config, handlers *handler.Handlers, profiles authmw.ProfileSaver) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(authmw.NewCORS([]string{"http://localhost:5173"}))

	r.Get("/health", handlers.Health)

	auth := authmw.NewSupabaseAuth(cfg.Supabase, profiles)
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)

		r.Get("/auth/me", handlers.AuthMe)

		r.Get("/analytics/summary", handlers.AnalyticsSummary)
		r.Get("/analytics/timeseries", handlers.AnalyticsTimeseries)
		r.Get("/analytics/by-tag", handlers.AnalyticsByTag)
		r.Get("/reports/monthly", handlers.ReportsMonthly)
		r.Get("/reports/compare", handlers.ReportsCompare)

		r.Get("/families/me", handlers.GetFamilyMe)
		r.Post("/families", handlers.CreateFamily)
		r.Post("/families/join", handlers.JoinFamily)
		r.Post("/families/leave", handlers.LeaveFamily)
		r.Patch("/families/me", handlers.UpdateFamily)
		r.Get("/families/me/members", handlers.ListFamilyMembers)
		r.Delete("/families/me/members/{user_id}", handlers.RemoveFamilyMember)

		r.Get("/expenses", handlers.ListExpenses)
		r.Post("/expenses", handlers.CreateExpense)
		r.Put("/expenses/{id}", handlers.UpdateExpense)
		r.Delete("/expenses/{id}", handlers.DeleteExpense)

		r.Get("/tags", handlers.ListTags)
		r.Post("/tags", handlers.CreateTag)
		r.Delete("/tags/{id}", handlers.DeleteTag)
	})

	return r
}
