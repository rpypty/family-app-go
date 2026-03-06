package rates

import (
	"context"
	"time"

	ratesdomain "family-app-go/internal/domain/rates"
	"gorm.io/gorm"
)

type RateProvider interface {
	GetBYNRate(ctx context.Context, currency string, onDate time.Time) (ratesdomain.BYNRate, error)
}

type PostgresProvider struct {
	db           *gorm.DB
	rateProvider RateProvider
}

func NewPostgresProvider(db *gorm.DB, rateProvider RateProvider) *PostgresProvider {
	return &PostgresProvider{
		db:           db,
		rateProvider: rateProvider,
	}
}

func (p *PostgresProvider) ListCurrencies(ctx context.Context) ([]ratesdomain.Currency, error) {
	var rows []struct {
		Code string `gorm:"column:code"`
		Name string `gorm:"column:name"`
		Icon string `gorm:"column:icon"`
	}

	if err := p.db.WithContext(ctx).
		Table("currencies").
		Select("code, name, icon").
		Where("is_active = ?", true).
		Order("sort_order asc, code asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]ratesdomain.Currency, 0, len(rows))
	for _, row := range rows {
		result = append(result, ratesdomain.Currency{
			Code: row.Code,
			Name: row.Name,
			Icon: row.Icon,
		})
	}

	return result, nil
}

func (p *PostgresProvider) GetBYNRate(ctx context.Context, currency string, onDate time.Time) (ratesdomain.BYNRate, error) {
	return p.rateProvider.GetBYNRate(ctx, currency, onDate)
}
