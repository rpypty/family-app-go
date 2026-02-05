package analytics

import "context"

type Repository interface {
	Summary(ctx context.Context, familyID string, filter SummaryFilter) (SummaryResult, error)
	Timeseries(ctx context.Context, familyID string, filter TimeseriesFilter) ([]TimeseriesPoint, error)
	ByTag(ctx context.Context, familyID string, filter ByTagFilter) ([]ByTagRow, error)
	Monthly(ctx context.Context, familyID string, filter MonthlyFilter) ([]MonthlyRow, error)
}
