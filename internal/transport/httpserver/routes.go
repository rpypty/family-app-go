package httpserver

import (
	"net/http"
	"time"

	"family-app-go/internal/config"
	"family-app-go/internal/transport/httpserver/handler"
	authmw "family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"
)

func NewRouter(cfg config.Config, db *gorm.DB) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	auth := authmw.NewSupabaseAuth(cfg.Supabase)
	r.Use(auth.Middleware)

	handlers := handler.New(db)

	r.Get("/healthz", handlers.Healthz)
	r.Get("/ping", handlers.Ping)

	return r
}
