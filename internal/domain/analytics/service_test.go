package analytics

import (
	"context"
	"testing"
	"time"
)

type fakeAnalyticsRepo struct {
	summaries                map[string]SummaryResult
	topCategoriesRows        []ByCategoryRow
	topCategoriesRecordsRead int64
	topCategoriesCalls       int
}

func (f *fakeAnalyticsRepo) Summary(ctx context.Context, familyID string, filter SummaryFilter) (SummaryResult, error) {
	key := filter.From.Format("2006-01-02") + "_" + filter.To.Format("2006-01-02")
	if result, ok := f.summaries[key]; ok {
		return result, nil
	}
	return SummaryResult{}, nil
}

func (f *fakeAnalyticsRepo) Timeseries(ctx context.Context, familyID string, filter TimeseriesFilter) ([]TimeseriesPoint, error) {
	return nil, nil
}

func (f *fakeAnalyticsRepo) ByCategory(ctx context.Context, familyID string, filter ByCategoryFilter) ([]ByCategoryRow, error) {
	return nil, nil
}

func (f *fakeAnalyticsRepo) TopCategories(ctx context.Context, familyID string, filter TopCategoriesFilter) ([]ByCategoryRow, int64, error) {
	f.topCategoriesCalls++
	rows := make([]ByCategoryRow, len(f.topCategoriesRows))
	copy(rows, f.topCategoriesRows)
	return rows, f.topCategoriesRecordsRead, nil
}

func (f *fakeAnalyticsRepo) Monthly(ctx context.Context, familyID string, filter MonthlyFilter) ([]MonthlyRow, error) {
	return nil, nil
}

func TestSummaryAvgPerDay(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		summaries: map[string]SummaryResult{
			"2026-01-01_2026-01-03": {TotalAmount: 300, Count: 3},
		},
	}
	svc := NewService(repo)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	result, err := svc.Summary(context.Background(), "fam-1", SummaryFilter{From: from, To: to})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AvgPerDay != 100 {
		t.Fatalf("expected avg 100, got %v", result.AvgPerDay)
	}
}

func TestCompareDelta(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		summaries: map[string]SummaryResult{
			"2026-01-01_2026-01-31": {TotalAmount: 2800, Count: 75},
			"2025-12-01_2025-12-31": {TotalAmount: 3200, Count: 84},
		},
	}
	svc := NewService(repo)

	fromA := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	toA := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	fromB := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	toB := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

	result, err := svc.Compare(context.Background(), "fam-1", CompareFilter{
		FromA: fromA,
		ToA:   toA,
		FromB: fromB,
		ToB:   toB,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Delta.Amount != -400 {
		t.Fatalf("expected delta -400, got %v", result.Delta.Amount)
	}
	if result.Delta.Percent != -12.5 {
		t.Fatalf("expected percent -12.5, got %v", result.Delta.Percent)
	}
}

func TestTopCategoriesUsesCacheWithinTTL(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		topCategoriesRows: []ByCategoryRow{
			{CategoryID: "cat-1", CategoryName: "Food", Count: 2, Total: 25},
		},
		topCategoriesRecordsRead: 12,
	}

	svc := NewServiceWithTopCategoriesConfig(repo, TopCategoriesConfig{
		Enabled:       true,
		LookbackDays:  30,
		DBReadLimit:   1000,
		MinRecords:    10,
		ResponseCount: 5,
		CacheTTL:      time.Minute,
	})
	currentTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return currentTime }

	first, err := svc.TopCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if first.Status != TopCategoriesStatusOK {
		t.Fatalf("expected status OK, got %s", first.Status)
	}
	if repo.topCategoriesCalls != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.topCategoriesCalls)
	}
	if len(first.Items) != 1 || first.Items[0].CategoryID != "cat-1" {
		t.Fatalf("unexpected rows: %+v", first.Items)
	}

	repo.topCategoriesRows = []ByCategoryRow{
		{CategoryID: "cat-2", CategoryName: "Transport", Count: 7, Total: 70},
	}

	second, err := svc.TopCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.topCategoriesCalls != 1 {
		t.Fatalf("expected cache hit without extra repo call, got %d", repo.topCategoriesCalls)
	}
	if second.Status != TopCategoriesStatusOK {
		t.Fatalf("expected status OK, got %s", second.Status)
	}
	if len(second.Items) != 1 || second.Items[0].CategoryID != "cat-1" {
		t.Fatalf("expected cached rows, got %+v", second.Items)
	}
}

func TestTopCategoriesCacheExpires(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		topCategoriesRows: []ByCategoryRow{
			{CategoryID: "cat-1", CategoryName: "Food", Count: 2, Total: 25},
		},
		topCategoriesRecordsRead: 12,
	}

	svc := NewServiceWithTopCategoriesConfig(repo, TopCategoriesConfig{
		Enabled:       true,
		LookbackDays:  30,
		DBReadLimit:   1000,
		MinRecords:    10,
		ResponseCount: 5,
		CacheTTL:      time.Minute,
	})
	currentTime := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return currentTime }

	_, err := svc.TopCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.topCategoriesCalls != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.topCategoriesCalls)
	}

	currentTime = currentTime.Add(2 * time.Minute)
	repo.topCategoriesRows = []ByCategoryRow{
		{CategoryID: "cat-2", CategoryName: "Transport", Count: 3, Total: 90},
	}

	rows, err := svc.TopCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.topCategoriesCalls != 2 {
		t.Fatalf("expected cache miss after TTL expiration, got %d repo calls", repo.topCategoriesCalls)
	}
	if rows.Status != TopCategoriesStatusOK {
		t.Fatalf("expected status OK, got %s", rows.Status)
	}
	if len(rows.Items) != 1 || rows.Items[0].CategoryID != "cat-2" {
		t.Fatalf("expected fresh rows after cache expiration, got %+v", rows.Items)
	}
}

func TestTopCategoriesDisabled(t *testing.T) {
	repo := &fakeAnalyticsRepo{}
	svc := NewServiceWithTopCategoriesConfig(repo, TopCategoriesConfig{
		Enabled:       false,
		LookbackDays:  30,
		DBReadLimit:   1000,
		MinRecords:    10,
		ResponseCount: 5,
		CacheTTL:      time.Minute,
	})

	result, err := svc.TopCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != TopCategoriesStatusDisabled {
		t.Fatalf("expected status TOP_CATEGORIES_DISABLED, got %s", result.Status)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected empty items, got %+v", result.Items)
	}
	if repo.topCategoriesCalls != 0 {
		t.Fatalf("expected no repo calls, got %d", repo.topCategoriesCalls)
	}
}

func TestTopCategoriesNeedMoreData(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		topCategoriesRows: []ByCategoryRow{
			{CategoryID: "cat-1", CategoryName: "Food", Count: 2, Total: 25},
		},
		topCategoriesRecordsRead: 5,
	}
	svc := NewServiceWithTopCategoriesConfig(repo, TopCategoriesConfig{
		Enabled:       true,
		LookbackDays:  30,
		DBReadLimit:   1000,
		MinRecords:    10,
		ResponseCount: 5,
		CacheTTL:      0,
	})

	result, err := svc.TopCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != TopCategoriesStatusNeedMoreData {
		t.Fatalf("expected status NEED_MORE_DATA, got %s", result.Status)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected empty items, got %+v", result.Items)
	}
}

func TestTopCategoriesCacheIsPerFamily(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		topCategoriesRows: []ByCategoryRow{
			{CategoryID: "cat-1", CategoryName: "Food", Count: 2, Total: 25},
		},
		topCategoriesRecordsRead: 12,
	}
	svc := NewServiceWithTopCategoriesConfig(repo, TopCategoriesConfig{
		Enabled:       true,
		LookbackDays:  30,
		DBReadLimit:   1000,
		MinRecords:    10,
		ResponseCount: 5,
		CacheTTL:      time.Minute,
	})

	if _, err := svc.TopCategories(context.Background(), "fam-1"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if _, err := svc.TopCategories(context.Background(), "fam-2"); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if repo.topCategoriesCalls != 2 {
		t.Fatalf("expected separate cache entries per family, got %d repo calls", repo.topCategoriesCalls)
	}
}
