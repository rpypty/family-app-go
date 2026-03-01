package analytics

import (
	"context"
	"sync"
	"time"
)

type Service struct {
	repo                Repository
	topCategoriesConfig TopCategoriesConfig
	topCategoriesCache  topCategoriesCache
	now                 func() time.Time
}

func NewService(repo Repository) *Service {
	return NewServiceWithTopCategoriesConfig(repo, TopCategoriesConfig{
		Enabled:       true,
		LookbackDays:  defaultTopCategoriesLookbackDays,
		DBReadLimit:   defaultTopCategoriesDBReadLimit,
		MinRecords:    defaultTopCategoriesMinRecords,
		ResponseCount: defaultTopCategoriesResponseCount,
		CacheTTL:      defaultTopCategoriesCacheTTL,
	})
}

func NewServiceWithTopCategoriesConfig(repo Repository, cfg TopCategoriesConfig) *Service {
	cfg = normalizeTopCategoriesConfig(cfg)

	return &Service{
		repo:                repo,
		topCategoriesConfig: cfg,
		topCategoriesCache: topCategoriesCache{
			items: make(map[string]topCategoriesCacheItem),
		},
		now: time.Now,
	}
}

func (s *Service) Summary(ctx context.Context, familyID string, filter SummaryFilter) (SummaryResult, error) {
	result, err := s.repo.Summary(ctx, familyID, filter)
	if err != nil {
		return SummaryResult{}, err
	}

	days := daysBetweenInclusive(filter.From, filter.To)
	if days > 0 {
		result.AvgPerDay = result.TotalAmount / float64(days)
	}

	return result, nil
}

func (s *Service) Timeseries(ctx context.Context, familyID string, filter TimeseriesFilter) ([]TimeseriesPoint, error) {
	return s.repo.Timeseries(ctx, familyID, filter)
}

func (s *Service) ByCategory(ctx context.Context, familyID string, filter ByCategoryFilter) ([]ByCategoryRow, error) {
	return s.repo.ByCategory(ctx, familyID, filter)
}

func (s *Service) TopCategories(ctx context.Context, familyID string) (TopCategoriesResult, error) {
	if !s.topCategoriesConfig.Enabled {
		return TopCategoriesResult{
			Status: TopCategoriesStatusDisabled,
			Items:  []ByCategoryRow{},
		}, nil
	}

	filter := s.topCategoriesFilter()
	if s.topCategoriesConfig.CacheTTL <= 0 {
		rows, recordsRead, err := s.repo.TopCategories(ctx, familyID, filter)
		if err != nil {
			return TopCategoriesResult{}, err
		}
		return s.buildTopCategoriesResult(rows, recordsRead), nil
	}

	now := s.now()
	cacheKey := topCategoriesCacheKey(familyID)
	if result, ok := s.topCategoriesCache.Get(cacheKey, now); ok {
		return result, nil
	}

	rows, recordsRead, err := s.repo.TopCategories(ctx, familyID, filter)
	if err != nil {
		return TopCategoriesResult{}, err
	}

	result := s.buildTopCategoriesResult(rows, recordsRead)
	s.topCategoriesCache.Set(cacheKey, result, now.Add(s.topCategoriesConfig.CacheTTL))
	return result, nil
}

func (s *Service) Monthly(ctx context.Context, familyID string, filter MonthlyFilter) ([]MonthlyRow, error) {
	return s.repo.Monthly(ctx, familyID, filter)
}

func (s *Service) Compare(ctx context.Context, familyID string, filter CompareFilter) (CompareResult, error) {
	resultA, err := s.repo.Summary(ctx, familyID, SummaryFilter{
		From:        filter.FromA,
		To:          filter.ToA,
		Currency:    filter.Currency,
		CategoryIDs: filter.CategoryIDs,
	})
	if err != nil {
		return CompareResult{}, err
	}

	resultB, err := s.repo.Summary(ctx, familyID, SummaryFilter{
		From:        filter.FromB,
		To:          filter.ToB,
		Currency:    filter.Currency,
		CategoryIDs: filter.CategoryIDs,
	})
	if err != nil {
		return CompareResult{}, err
	}

	deltaAmount := resultA.TotalAmount - resultB.TotalAmount
	deltaPercent := 0.0
	if resultB.TotalAmount != 0 {
		deltaPercent = (deltaAmount / resultB.TotalAmount) * 100
	}

	return CompareResult{
		PeriodA: PeriodSummary{
			From:  filter.FromA.Format("2006-01-02"),
			To:    filter.ToA.Format("2006-01-02"),
			Total: resultA.TotalAmount,
			Count: resultA.Count,
		},
		PeriodB: PeriodSummary{
			From:  filter.FromB.Format("2006-01-02"),
			To:    filter.ToB.Format("2006-01-02"),
			Total: resultB.TotalAmount,
			Count: resultB.Count,
		},
		Delta: DeltaResult{
			Amount:  deltaAmount,
			Percent: deltaPercent,
		},
	}, nil
}

func daysBetweenInclusive(from, to time.Time) int {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if to.Before(from) {
		return 0
	}
	return int(to.Sub(from).Hours()/24) + 1
}

const (
	defaultTopCategoriesLookbackDays  = 30
	defaultTopCategoriesDBReadLimit   = 1000
	defaultTopCategoriesMinRecords    = 10
	defaultTopCategoriesResponseCount = 5
	defaultTopCategoriesCacheTTL      = time.Minute
)

func normalizeTopCategoriesConfig(cfg TopCategoriesConfig) TopCategoriesConfig {
	if cfg.LookbackDays <= 0 {
		cfg.LookbackDays = defaultTopCategoriesLookbackDays
	}
	if cfg.DBReadLimit <= 0 {
		cfg.DBReadLimit = defaultTopCategoriesDBReadLimit
	}
	if cfg.MinRecords < 0 {
		cfg.MinRecords = defaultTopCategoriesMinRecords
	}
	if cfg.ResponseCount <= 0 {
		cfg.ResponseCount = defaultTopCategoriesResponseCount
	}
	if cfg.CacheTTL < 0 {
		cfg.CacheTTL = 0
	}
	return cfg
}

func (s *Service) topCategoriesFilter() TopCategoriesFilter {
	current := s.now().UTC()
	to := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -(s.topCategoriesConfig.LookbackDays - 1))

	return TopCategoriesFilter{
		From:          from,
		To:            to,
		DBReadLimit:   s.topCategoriesConfig.DBReadLimit,
		ResponseCount: s.topCategoriesConfig.ResponseCount,
	}
}

func (s *Service) buildTopCategoriesResult(rows []ByCategoryRow, recordsRead int64) TopCategoriesResult {
	if recordsRead < int64(s.topCategoriesConfig.MinRecords) || len(rows) == 0 {
		return TopCategoriesResult{
			Status: TopCategoriesStatusNeedMoreData,
			Items:  []ByCategoryRow{},
		}
	}

	if len(rows) > s.topCategoriesConfig.ResponseCount {
		rows = rows[:s.topCategoriesConfig.ResponseCount]
	}

	return TopCategoriesResult{
		Status: TopCategoriesStatusOK,
		Items:  cloneByCategoryRows(rows),
	}
}

func topCategoriesCacheKey(familyID string) string {
	return familyID
}

type topCategoriesCache struct {
	mu    sync.RWMutex
	items map[string]topCategoriesCacheItem
}

type topCategoriesCacheItem struct {
	result    TopCategoriesResult
	expiresAt time.Time
}

func (c *topCategoriesCache) Get(key string, now time.Time) (TopCategoriesResult, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return TopCategoriesResult{}, false
	}

	if !item.expiresAt.After(now) {
		c.mu.Lock()
		item, ok = c.items[key]
		if ok && !item.expiresAt.After(now) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return TopCategoriesResult{}, false
	}

	return cloneTopCategoriesResult(item.result), true
}

func (c *topCategoriesCache) Set(key string, result TopCategoriesResult, expiresAt time.Time) {
	c.mu.Lock()
	c.items[key] = topCategoriesCacheItem{
		result:    cloneTopCategoriesResult(result),
		expiresAt: expiresAt,
	}
	c.mu.Unlock()
}

func cloneTopCategoriesResult(result TopCategoriesResult) TopCategoriesResult {
	return TopCategoriesResult{
		Status: result.Status,
		Items:  cloneByCategoryRows(result.Items),
	}
}

func cloneByCategoryRows(rows []ByCategoryRow) []ByCategoryRow {
	if rows == nil {
		return nil
	}
	cloned := make([]ByCategoryRow, len(rows))
	copy(cloned, rows)
	return cloned
}
