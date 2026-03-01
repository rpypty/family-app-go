package app

import (
	"fmt"
	"net/http"

	"family-app-go/internal/config"
	"family-app-go/internal/db"
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	gymdomain "family-app-go/internal/domain/gym"
	syncdomain "family-app-go/internal/domain/sync"
	todosdomain "family-app-go/internal/domain/todos"
	userdomain "family-app-go/internal/domain/user"
	analyticsrepo "family-app-go/internal/repository/analytics"
	expensesrepo "family-app-go/internal/repository/expenses"
	familyrepo "family-app-go/internal/repository/family"
	gymrepo "family-app-go/internal/repository/gym"
	syncrepo "family-app-go/internal/repository/sync"
	todosrepo "family-app-go/internal/repository/todos"
	userrepo "family-app-go/internal/repository/user"
	"family-app-go/internal/transport/httpserver"
	"family-app-go/internal/transport/httpserver/handler"
	"family-app-go/pkg/logger"
	"gorm.io/gorm"
)

type App struct {
	cfg        config.Config
	httpServer *http.Server
	db         *gorm.DB
}

func New(log logger.Logger) (*App, error) {
	log.Info("app: loading config")
	cfg, err := config.Load(log)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	log.Info("app: initializing database")
	dbConn, err := db.NewPostgres(log, cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	log.Info("app: running migrations")
	if err := db.Migrate(dbConn); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	log.Info("app: initializing services")
	familyRepo := familyrepo.NewPostgres(dbConn)
	familyService := familydomain.NewService(familyRepo)
	expensesRepo := expensesrepo.NewPostgres(dbConn)
	expensesService := expensesdomain.NewService(expensesRepo)
	analyticsRepo := analyticsrepo.NewPostgres(dbConn)
	analyticsService := analyticsdomain.NewServiceWithTopCategoriesConfig(analyticsRepo, analyticsdomain.TopCategoriesConfig{
		Enabled:       cfg.TopCategories.Enabled,
		LookbackDays:  cfg.TopCategories.LookbackDays,
		DBReadLimit:   cfg.TopCategories.DBReadLimit,
		MinRecords:    cfg.TopCategories.MinRecords,
		ResponseCount: cfg.TopCategories.ResponseCount,
		CacheTTL:      cfg.TopCategories.CacheTTL,
	})
	userRepo := userrepo.NewPostgres(dbConn)
	userService := userdomain.NewService(userRepo)
	todosRepo := todosrepo.NewPostgres(dbConn)
	todosService := todosdomain.NewService(todosRepo)
	syncRepo := syncrepo.NewPostgres(dbConn)
	syncService := syncdomain.NewService(syncRepo, expensesService, todosService)
	gymRepo := gymrepo.NewPostgres(dbConn)
	gymService := gymdomain.NewService(gymRepo)
	handlers := handler.New(analyticsService, familyService, expensesService, todosService, syncService, gymService, log)

	log.Info("app: initializing router")
	router := httpserver.NewRouter(cfg, handlers, userService, log)

	log.Info("app: initializing http server")
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
