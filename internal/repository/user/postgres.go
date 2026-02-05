package user

import (
	"context"
	"time"

	domain "family-app-go/internal/domain/user"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) UpsertProfile(ctx context.Context, profile *domain.Profile) error {
	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}
	if profile.Email != nil {
		updates["email"] = profile.Email
	}
	if profile.AvatarURL != nil {
		updates["avatar_url"] = profile.AvatarURL
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(updates),
		}).
		Create(profile).Error
}
