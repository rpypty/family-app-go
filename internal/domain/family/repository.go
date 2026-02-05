package family

import "context"

type Repository interface {
	Transaction(ctx context.Context, fn func(Repository) error) error
	GetFamilyByUser(ctx context.Context, userID string) (*Family, error)
	GetFamilyByCode(ctx context.Context, code string) (*Family, error)
	GetMemberByUser(ctx context.Context, userID string) (*FamilyMember, error)
	ListMembers(ctx context.Context, familyID string) ([]FamilyMember, error)
	CreateFamily(ctx context.Context, family *Family) error
	AddMember(ctx context.Context, member *FamilyMember) error
	UpdateFamilyName(ctx context.Context, familyID, name string) error
	UpdateFamilyOwner(ctx context.Context, familyID, ownerID string) error
	UpdateMemberRole(ctx context.Context, familyID, userID, role string) error
	DeleteFamily(ctx context.Context, familyID string) error
	DeleteMember(ctx context.Context, familyID, userID string) error
	DeleteMembersByFamily(ctx context.Context, familyID string) error
	CountMembers(ctx context.Context, familyID string) (int64, error)
	IsUserInFamily(ctx context.Context, userID string) (bool, error)
	IsCodeTaken(ctx context.Context, code string) (bool, error)
}
