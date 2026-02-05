package analytics

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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

func (s *Service) ByTag(ctx context.Context, familyID string, filter ByTagFilter) ([]ByTagRow, error) {
	return s.repo.ByTag(ctx, familyID, filter)
}

func (s *Service) Monthly(ctx context.Context, familyID string, filter MonthlyFilter) ([]MonthlyRow, error) {
	return s.repo.Monthly(ctx, familyID, filter)
}

func (s *Service) Compare(ctx context.Context, familyID string, filter CompareFilter) (CompareResult, error) {
	resultA, err := s.repo.Summary(ctx, familyID, SummaryFilter{
		From:     filter.FromA,
		To:       filter.ToA,
		Currency: filter.Currency,
		TagIDs:   filter.TagIDs,
	})
	if err != nil {
		return CompareResult{}, err
	}

	resultB, err := s.repo.Summary(ctx, familyID, SummaryFilter{
		From:     filter.FromB,
		To:       filter.ToB,
		Currency: filter.Currency,
		TagIDs:   filter.TagIDs,
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
