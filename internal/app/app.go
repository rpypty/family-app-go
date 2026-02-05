package app

import (
	"log"
	"net/http"

	"family-app-go/internal/config"
	"family-app-go/internal/db"
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	analyticsrepo "family-app-go/internal/repository/analytics"
	expensesrepo "family-app-go/internal/repository/expenses"
	familyrepo "family-app-go/internal/repository/family"
	"family-app-go/internal/transport/httpserver"
	"family-app-go/internal/transport/httpserver/handler"
	"gorm.io/gorm"
)

type App struct {
	cfg        config.Config
	httpServer *http.Server
	db         *gorm.DB
}

func New() (*App, error) {
	log.Printf("app: loading config")
	cfg := config.Load()

	log.Printf("app: initializing database")
	dbConn, err := db.NewPostgres(cfg.DB)
	if err != nil {
		return nil, err
	}

	log.Printf("app: running migrations")
	if err := db.Migrate(dbConn); err != nil {
		return nil, err
	}

	log.Printf("app: initializing services")
	familyRepo := familyrepo.NewPostgres(dbConn)
	familyService := familydomain.NewService(familyRepo)
	expensesRepo := expensesrepo.NewPostgres(dbConn)
	expensesService := expensesdomain.NewService(expensesRepo)
	analyticsRepo := analyticsrepo.NewPostgres(dbConn)
	analyticsService := analyticsdomain.NewService(analyticsRepo)
	handlers := handler.New(analyticsService, familyService, expensesService)

	log.Printf("app: initializing router")
	router := httpserver.NewRouter(cfg, handlers)

	log.Printf("app: initializing http server")
	srv := httpserver.New(cfg, router)

	return &App{
		cfg:        cfg,
		httpServer: srv,
		db:         dbConn,
	}, nil
}

func (a *App) HTTPServer() *http.Server {
	return a.httpServer
}

func (a *App) Close() error {
	if a.db == nil {
		return nil
	}
	sqlDB, err := a.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
