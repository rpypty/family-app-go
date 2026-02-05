package user

import (
	"context"
	"fmt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) UpsertProfile(ctx context.Context, userID, email, avatarURL string) error {
	if userID == "" {
		return fmt.Errorf("user id is required")
	}

	profile := Profile{UserID: userID}
	if email != "" {
		profile.Email = &email
	}
	if avatarURL != "" {
		profile.AvatarURL = &avatarURL
	}

	return s.repo.UpsertProfile(ctx, &profile)
}
