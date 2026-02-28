package analytics

import "time"

type SummaryFilter struct {
	From        time.Time
	To          time.Time
	Currency    string
	CategoryIDs []string
}

type SummaryResult struct {
	TotalAmount float64
	Count       int64
	AvgPerDay   float64
}

type TimeseriesFilter struct {
	From        time.Time
	To          time.Time
	GroupBy     string
	Currency    string
	CategoryIDs []string
	Timezone    string
}

type TimeseriesPoint struct {
	Period string  `json:"period"`
	Total  float64 `json:"total"`
	Count  int64   `json:"count"`
}

type ByCategoryFilter struct {
	From        time.Time
	To          time.Time
	Currency    string
	CategoryIDs []string
	Limit       int
}

type ByCategoryRow struct {
	CategoryID   string  `json:"category_id"`
	CategoryName string  `json:"category_name"`
	Total        float64 `json:"total"`
	Count        int64   `json:"count"`
}

type MonthlyFilter struct {
	From        time.Time
	To          time.Time
	Currency    string
	CategoryIDs []string
}

type MonthlyRow struct {
	Month string  `json:"month"`
	Total float64 `json:"total"`
	Count int64   `json:"count"`
}

type CompareFilter struct {
	FromA       time.Time
	ToA         time.Time
	FromB       time.Time
	ToB         time.Time
	Currency    string
	CategoryIDs []string
}

type PeriodSummary struct {
	From  string  `json:"from"`
	To    string  `json:"to"`
	Total float64 `json:"total"`
	Count int64   `json:"count"`
}

type CompareResult struct {
	PeriodA PeriodSummary `json:"period_a"`
	PeriodB PeriodSummary `json:"period_b"`
	Delta   DeltaResult   `json:"delta"`
}

type DeltaResult struct {
	Amount  float64 `json:"amount"`
	Percent float64 `json:"percent"`
}
