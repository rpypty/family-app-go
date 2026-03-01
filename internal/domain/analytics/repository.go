package analytics

import "context"

type Repository interface {
	Summary(ctx context.Context, familyID string, filter SummaryFilter) (SummaryResult, error)
	Timeseries(ctx context.Context, familyID string, filter TimeseriesFilter) ([]TimeseriesPoint, error)
	ByCategory(ctx context.Context, familyID string, filter ByCategoryFilter) ([]ByCategoryRow, error)
	TopCategories(ctx context.Context, familyID string, filter TopCategoriesFilter) ([]ByCategoryRow, int64, error)
	Monthly(ctx context.Context, familyID string, filter MonthlyFilter) ([]MonthlyRow, error)
}
