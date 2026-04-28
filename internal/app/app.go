package app

import (
	"fmt"
	"net/http"

	"family-app-go/internal/config"
	"family-app-go/internal/db"
	"family-app-go/internal/devseed"
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	gymdomain "family-app-go/internal/domain/gym"
	ratesdomain "family-app-go/internal/domain/rates"
	receiptsdomain "family-app-go/internal/domain/receipts"
	syncdomain "family-app-go/internal/domain/sync"
	todosdomain "family-app-go/internal/domain/todos"
	userdomain "family-app-go/internal/domain/user"
	httpratesrepo "family-app-go/internal/repository/http/rates"
	inmemoryrepo "family-app-go/internal/repository/inmemory"
	analyticsrepo "family-app-go/internal/repository/postgres/analytics"
	expensesrepo "family-app-go/internal/repository/postgres/expenses"
	familyrepo "family-app-go/internal/repository/postgres/family"
	gymrepo "family-app-go/internal/repository/postgres/gym"
	postgresratesrepo "family-app-go/internal/repository/postgres/rates"
	receiptsrepo "family-app-go/internal/repository/postgres/receipts"
	syncrepo "family-app-go/internal/repository/postgres/sync"
	todosrepo "family-app-go/internal/repository/postgres/todos"
	userrepo "family-app-go/internal/repository/postgres/user"
	"family-app-go/internal/transport/httpserver"
	"family-app-go/internal/transport/httpserver/handler"
	commonhandler "family-app-go/internal/transport/httpserver/handler/common"
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
	familyCache := inmemoryrepo.NewInMemoryFamilyCache()
	familyService := familydomain.NewServiceWithCache(familyRepo, familyCache)
	expensesRepo := expensesrepo.NewPostgres(dbConn)
	categoriesCache := inmemoryrepo.NewInMemoryCategoriesCache()
	nbrbProvider, err := httpratesrepo.NewNBRBClient(cfg.Rates.NBRBBaseURL, cfg.Rates.HTTPTimeout)
	if err != nil {
		return nil, fmt.Errorf("initialize rates provider: %w", err)
	}
	ratesProvider := postgresratesrepo.NewPostgresProvider(dbConn, nbrbProvider)
	ratesService := ratesdomain.NewService(ratesProvider, ratesdomain.Config{
		RateCacheTTL:       cfg.Rates.RateCacheTTL,
		CurrenciesCacheTTL: cfg.Rates.CurrenciesCacheTTL,
		FallbackDays:       cfg.Rates.FallbackDays,
	})
	expensesService := expensesdomain.NewServiceWithDependencies(expensesRepo, categoriesCache, ratesService)
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
	receiptRepo := receiptsrepo.NewPostgres(dbConn)
	receiptParser, err := buildReceiptParser(cfg.ReceiptParser, log)
	if err != nil {
		return nil, fmt.Errorf("initialize receipt parser: %w", err)
	}
	receiptHintNormalizer, err := buildReceiptHintNormalizer(cfg.ReceiptParser, log)
	if err != nil {
		return nil, fmt.Errorf("initialize receipt hint normalizer: %w", err)
	}
	receiptService := receiptsdomain.NewServiceWithOptions(receiptRepo, receiptParser, expensesService, expensesService, receiptsdomain.ServiceOptions{
		FileStore:      receiptsdomain.NewLocalFileStore(cfg.ReceiptParser.FileStorageDir),
		HintNormalizer: receiptHintNormalizer,
		WorkerEnabled:  true,
	})

	var mockDataSeeder commonhandler.FamilySeeder
	if cfg.MockDataSeed.Enabled {
		log.Info("app: mock data seed enabled")
		mockDataSeeder = devseed.NewExpenseSeeder(expensesService, devseed.Config{
			Enabled:          cfg.MockDataSeed.Enabled,
			LookbackMonths:   cfg.MockDataSeed.LookbackMonths,
			MinCategories:    cfg.MockDataSeed.MinCategories,
			MaxCategories:    cfg.MockDataSeed.MaxCategories,
			MaxDailyExpenses: cfg.MockDataSeed.MaxDailyExpenses,
			Currency:         cfg.MockDataSeed.Currency,
		})
	}
	handlers := handler.New(analyticsService, familyService, expensesService, ratesService, todosService, syncService, gymService, receiptService, log, mockDataSeeder)

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
