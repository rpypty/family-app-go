package user

import "context"

type Repository interface {
	UpsertProfile(ctx context.Context, profile *Profile) error
}
