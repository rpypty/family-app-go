package family

import (
	"context"
	"errors"
	"time"

	familydomain "family-app-go/internal/domain/family"
	"gorm.io/gorm"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Transaction(ctx context.Context, fn func(familydomain.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PostgresRepository{db: tx})
	})
}

func (r *PostgresRepository) GetFamilyByUser(ctx context.Context, userID string) (*familydomain.Family, error) {
	var family familydomain.Family
	err := r.db.WithContext(ctx).
		Table("families").
		Joins("join family_members on family_members.family_id = families.id").
		Where("family_members.user_id = ?", userID).
		Limit(1).
		First(&family).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, familydomain.ErrFamilyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &family, nil
}

func (r *PostgresRepository) GetFamilyByCode(ctx context.Context, code string) (*familydomain.Family, error) {
	var family familydomain.Family
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&family).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, familydomain.ErrFamilyCodeNotFound
		}
		return nil, err
	}
	return &family, nil
}

func (r *PostgresRepository) GetMemberByUser(ctx context.Context, userID string) (*familydomain.FamilyMember, error) {
	var member familydomain.FamilyMember
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, familydomain.ErrFamilyNotFound
		}
		return nil, err
	}
	return &member, nil
}

func (r *PostgresRepository) GetMember(ctx context.Context, familyID, userID string) (*familydomain.FamilyMember, error) {
	var member familydomain.FamilyMember
	if err := r.db.WithContext(ctx).Where("family_id = ? AND user_id = ?", familyID, userID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, familydomain.ErrMemberNotFound
		}
		return nil, err
	}
	return &member, nil
}

func (r *PostgresRepository) ListMembers(ctx context.Context, familyID string) ([]familydomain.FamilyMember, error) {
	var members []familydomain.FamilyMember
	if err := r.db.WithContext(ctx).
		Where("family_id = ?", familyID).
		Order("joined_at asc").
		Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func (r *PostgresRepository) ListMembersWithProfiles(ctx context.Context, familyID string) ([]familydomain.FamilyMemberProfile, error) {
	type memberRow struct {
		UserID    string    `gorm:"column:user_id"`
		Role      string    `gorm:"column:role"`
		JoinedAt  time.Time `gorm:"column:joined_at"`
		Email     *string   `gorm:"column:email"`
		AvatarURL *string   `gorm:"column:avatar_url"`
	}

	var rows []memberRow
	if err := r.db.WithContext(ctx).
		Table("family_members").
		Select("family_members.user_id, family_members.role, family_members.joined_at, user_profiles.email, user_profiles.avatar_url").
		Joins("left join user_profiles on user_profiles.user_id = family_members.user_id").
		Where("family_members.family_id = ?", familyID).
		Order("family_members.joined_at asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	members := make([]familydomain.FamilyMemberProfile, 0, len(rows))
	for _, row := range rows {
		members = append(members, familydomain.FamilyMemberProfile{
			UserID:    row.UserID,
			Role:      row.Role,
			JoinedAt:  row.JoinedAt,
			Email:     row.Email,
			AvatarURL: row.AvatarURL,
		})
	}
	return members, nil
}

func (r *PostgresRepository) CreateFamily(ctx context.Context, family *familydomain.Family) error {
	return r.db.WithContext(ctx).Create(family).Error
}

func (r *PostgresRepository) AddMember(ctx context.Context, member *familydomain.FamilyMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

func (r *PostgresRepository) UpdateFamilyName(ctx context.Context, familyID, name string) error {
	return r.db.WithContext(ctx).Model(&familydomain.Family{}).Where("id = ?", familyID).Update("name", name).Error
}

func (r *PostgresRepository) UpdateFamilyOwner(ctx context.Context, familyID, ownerID string) error {
	return r.db.WithContext(ctx).Model(&familydomain.Family{}).Where("id = ?", familyID).Update("owner_id", ownerID).Error
}

func (r *PostgresRepository) UpdateMemberRole(ctx context.Context, familyID, userID, role string) error {
	return r.db.WithContext(ctx).Model(&familydomain.FamilyMember{}).
		Where("family_id = ? AND user_id = ?", familyID, userID).
		Update("role", role).Error
}

func (r *PostgresRepository) DeleteFamily(ctx context.Context, familyID string) error {
	return r.db.WithContext(ctx).Delete(&familydomain.Family{}, "id = ?", familyID).Error
}

func (r *PostgresRepository) DeleteMember(ctx context.Context, familyID, userID string) error {
	return r.db.WithContext(ctx).Delete(&familydomain.FamilyMember{}, "family_id = ? AND user_id = ?", familyID, userID).Error
}

func (r *PostgresRepository) DeleteMembersByFamily(ctx context.Context, familyID string) error {
	return r.db.WithContext(ctx).Where("family_id = ?", familyID).Delete(&familydomain.FamilyMember{}).Error
}

func (r *PostgresRepository) CountMembers(ctx context.Context, familyID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&familydomain.FamilyMember{}).Where("family_id = ?", familyID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) IsUserInFamily(ctx context.Context, userID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&familydomain.FamilyMember{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresRepository) IsCodeTaken(ctx context.Context, code string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&familydomain.Family{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
