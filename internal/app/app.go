package app

import (
	"log"
	"net/http"

	"family-app-go/internal/config"
	"family-app-go/internal/db"
	"family-app-go/internal/transport/httpserver"
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

	log.Printf("app: initializing router")
	router := httpserver.NewRouter(cfg, dbConn)

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
