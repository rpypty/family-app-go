package analytics

import (
	"context"
	"testing"
	"time"
)

type fakeAnalyticsRepo struct {
	summaries map[string]SummaryResult
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
