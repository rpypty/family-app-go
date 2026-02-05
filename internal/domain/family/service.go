package family

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const (
	familyCodeLength   = 6
	familyCodeAttempts = 10
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetFamilyByUser(ctx context.Context, userID string) (*Family, error) {
	return s.repo.GetFamilyByUser(ctx, userID)
}

func (s *Service) CreateFamily(ctx context.Context, userID, name string) (*Family, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	var result Family
	err := s.repo.Transaction(ctx, func(tx Repository) error {
		inFamily, err := tx.IsUserInFamily(ctx, userID)
		if err != nil {
			return err
		}
		if inFamily {
			return ErrAlreadyInFamily
		}

		id, err := newUUID()
		if err != nil {
			return err
		}

		code, err := generateUniqueCode(ctx, tx)
		if err != nil {
			return err
		}

		family := Family{
			ID:      id,
			Name:    name,
			Code:    code,
			OwnerID: userID,
		}
		if err := tx.CreateFamily(ctx, &family); err != nil {
			return err
		}

		member := FamilyMember{
			FamilyID: family.ID,
			UserID:   userID,
			Role:     RoleOwner,
		}
		if err := tx.AddMember(ctx, &member); err != nil {
			return err
		}

		result = family
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *Service) JoinFamily(ctx context.Context, userID, code string) (*Family, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	var result Family
	err := s.repo.Transaction(ctx, func(tx Repository) error {
		inFamily, err := tx.IsUserInFamily(ctx, userID)
		if err != nil {
			return err
		}
		if inFamily {
			return ErrAlreadyInFamily
		}

		family, err := tx.GetFamilyByCode(ctx, code)
		if err != nil {
			return err
		}

		member := FamilyMember{
			FamilyID: family.ID,
			UserID:   userID,
			Role:     RoleMember,
		}
		if err := tx.AddMember(ctx, &member); err != nil {
			return err
		}

		result = *family
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *Service) LeaveFamily(ctx context.Context, userID string) error {
	return s.repo.Transaction(ctx, func(tx Repository) error {
		member, err := tx.GetMemberByUser(ctx, userID)
		if err != nil {
			return err
		}

		if member.Role == RoleOwner {
			count, err := tx.CountMembers(ctx, member.FamilyID)
			if err != nil {
				return err
			}
			if count > 1 {
				return ErrOwnerMustTransfer
			}

			if err := tx.DeleteMembersByFamily(ctx, member.FamilyID); err != nil {
				return err
			}
			return tx.DeleteFamily(ctx, member.FamilyID)
		}

		return tx.DeleteMember(ctx, member.FamilyID, userID)
	})
}

func (s *Service) UpdateFamily(ctx context.Context, userID, name string) (*Family, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	family, err := s.repo.GetFamilyByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpdateFamilyName(ctx, family.ID, name); err != nil {
		return nil, err
	}

	family.Name = name
	return family, nil
}

func (s *Service) ListMembers(ctx context.Context, userID string) ([]FamilyMember, error) {
	family, err := s.repo.GetFamilyByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListMembers(ctx, family.ID)
}

func generateUniqueCode(ctx context.Context, repo Repository) (string, error) {
	for i := 0; i < familyCodeAttempts; i++ {
		code, err := generateCode(familyCodeLength)
		if err != nil {
			return "", err
		}
		taken, err := repo.IsCodeTaken(ctx, code)
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
	}
	return "", ErrCodeGenerationFailed
}

func generateCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	max := big.NewInt(int64(len(alphabet)))

	var builder strings.Builder
	builder.Grow(length)

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(alphabet[n.Int64()])
	}

	return builder.String(), nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
