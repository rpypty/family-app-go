package family

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeFamilyRepo struct {
	families map[string]*Family
	members  map[string]*FamilyMember
	codes    map[string]string
}

func newFakeFamilyRepo() *fakeFamilyRepo {
	return &fakeFamilyRepo{
		families: make(map[string]*Family),
		members:  make(map[string]*FamilyMember),
		codes:    make(map[string]string),
	}
}

func (r *fakeFamilyRepo) Transaction(ctx context.Context, fn func(Repository) error) error {
	return fn(r)
}

func (r *fakeFamilyRepo) GetFamilyByUser(ctx context.Context, userID string) (*Family, error) {
	member, ok := r.members[userID]
	if !ok {
		return nil, ErrFamilyNotFound
	}
	family, ok := r.families[member.FamilyID]
	if !ok {
		return nil, ErrFamilyNotFound
	}
	return family, nil
}

func (r *fakeFamilyRepo) GetFamilyByCode(ctx context.Context, code string) (*Family, error) {
	id, ok := r.codes[code]
	if !ok {
		return nil, ErrFamilyCodeNotFound
	}
	family, ok := r.families[id]
	if !ok {
		return nil, ErrFamilyCodeNotFound
	}
	return family, nil
}

func (r *fakeFamilyRepo) GetMemberByUser(ctx context.Context, userID string) (*FamilyMember, error) {
	member, ok := r.members[userID]
	if !ok {
		return nil, ErrFamilyNotFound
	}
	return member, nil
}

func (r *fakeFamilyRepo) GetMember(ctx context.Context, familyID, userID string) (*FamilyMember, error) {
	member, ok := r.members[userID]
	if !ok || member.FamilyID != familyID {
		return nil, ErrMemberNotFound
	}
	return member, nil
}

func (r *fakeFamilyRepo) ListMembers(ctx context.Context, familyID string) ([]FamilyMember, error) {
	result := make([]FamilyMember, 0)
	for _, member := range r.members {
		if member.FamilyID == familyID {
			result = append(result, *member)
		}
	}
	return result, nil
}

func (r *fakeFamilyRepo) ListMembersWithProfiles(ctx context.Context, familyID string) ([]FamilyMemberProfile, error) {
	members, _ := r.ListMembers(ctx, familyID)
	result := make([]FamilyMemberProfile, 0, len(members))
	for _, member := range members {
		result = append(result, FamilyMemberProfile{
			UserID:   member.UserID,
			Role:     member.Role,
			JoinedAt: member.JoinedAt,
		})
	}
	return result, nil
}

func (r *fakeFamilyRepo) CreateFamily(ctx context.Context, family *Family) error {
	r.families[family.ID] = family
	r.codes[family.Code] = family.ID
	return nil
}

func (r *fakeFamilyRepo) AddMember(ctx context.Context, member *FamilyMember) error {
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now().UTC()
	}
	r.members[member.UserID] = member
	return nil
}

func (r *fakeFamilyRepo) UpdateFamilyName(ctx context.Context, familyID, name string) error {
	family, ok := r.families[familyID]
	if !ok {
		return ErrFamilyNotFound
	}
	family.Name = name
	return nil
}

func (r *fakeFamilyRepo) UpdateFamilyOwner(ctx context.Context, familyID, ownerID string) error {
	family, ok := r.families[familyID]
	if !ok {
		return ErrFamilyNotFound
	}
	family.OwnerID = ownerID
	return nil
}

func (r *fakeFamilyRepo) UpdateMemberRole(ctx context.Context, familyID, userID, role string) error {
	member, ok := r.members[userID]
	if !ok || member.FamilyID != familyID {
		return ErrFamilyNotFound
	}
	member.Role = role
	return nil
}

func (r *fakeFamilyRepo) DeleteFamily(ctx context.Context, familyID string) error {
	family, ok := r.families[familyID]
	if ok {
		delete(r.codes, family.Code)
	}
	delete(r.families, familyID)
	return nil
}

func (r *fakeFamilyRepo) DeleteMember(ctx context.Context, familyID, userID string) error {
	member, ok := r.members[userID]
	if ok && member.FamilyID == familyID {
		delete(r.members, userID)
	}
	return nil
}

func (r *fakeFamilyRepo) DeleteMembersByFamily(ctx context.Context, familyID string) error {
	for userID, member := range r.members {
		if member.FamilyID == familyID {
			delete(r.members, userID)
		}
	}
	return nil
}

func (r *fakeFamilyRepo) CountMembers(ctx context.Context, familyID string) (int64, error) {
	var count int64
	for _, member := range r.members {
		if member.FamilyID == familyID {
			count++
		}
	}
	return count, nil
}

func (r *fakeFamilyRepo) IsUserInFamily(ctx context.Context, userID string) (bool, error) {
	_, ok := r.members[userID]
	return ok, nil
}

func (r *fakeFamilyRepo) IsCodeTaken(ctx context.Context, code string) (bool, error) {
	_, ok := r.codes[code]
	return ok, nil
}

func TestCreateFamilySuccess(t *testing.T) {
	repo := newFakeFamilyRepo()
	svc := NewService(repo)

	result, err := svc.CreateFamily(context.Background(), "user-1", "  My Family  ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Name != "My Family" {
		t.Fatalf("expected name trimmed, got %q", result.Name)
	}
	if result.OwnerID != "user-1" {
		t.Fatalf("expected owner user-1, got %q", result.OwnerID)
	}
	if len(result.Code) != 6 {
		t.Fatalf("expected code length 6, got %q", result.Code)
	}
	member, ok := repo.members["user-1"]
	if !ok {
		t.Fatalf("expected member created")
	}
	if member.Role != RoleOwner {
		t.Fatalf("expected owner role, got %q", member.Role)
	}
	if member.FamilyID != result.ID {
		t.Fatalf("expected member family %s, got %s", result.ID, member.FamilyID)
	}
}

func TestCreateFamilyAlreadyInFamily(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "AAAAAA", OwnerID: "owner"}
	repo.codes["AAAAAA"] = "fam-1"
	repo.members["user-1"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-1", Role: RoleMember}

	svc := NewService(repo)
	_, err := svc.CreateFamily(context.Background(), "user-1", "Name")
	if !errors.Is(err, ErrAlreadyInFamily) {
		t.Fatalf("expected ErrAlreadyInFamily, got %v", err)
	}
}

func TestJoinFamilySuccess(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "owner"}
	repo.codes["ZXCVBN"] = "fam-1"

	svc := NewService(repo)
	result, err := svc.JoinFamily(context.Background(), "user-1", "zxcvbn")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ID != "fam-1" {
		t.Fatalf("expected family fam-1, got %s", result.ID)
	}
	member := repo.members["user-1"]
	if member == nil || member.Role != RoleMember {
		t.Fatalf("expected member role, got %+v", member)
	}
}

func TestJoinFamilyCodeNotFound(t *testing.T) {
	repo := newFakeFamilyRepo()
	svc := NewService(repo)
	_, err := svc.JoinFamily(context.Background(), "user-1", "missing")
	if !errors.Is(err, ErrFamilyCodeNotFound) {
		t.Fatalf("expected ErrFamilyCodeNotFound, got %v", err)
	}
}

func TestLeaveFamilyOwnerTransfers(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "owner"}
	repo.members["owner"] = &FamilyMember{FamilyID: "fam-1", UserID: "owner", Role: RoleOwner}
	repo.members["user-2"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-2", Role: RoleMember}

	svc := NewService(repo)
	if err := svc.LeaveFamily(context.Background(), "owner"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.families["fam-1"] == nil {
		t.Fatalf("family should not be deleted")
	}
	if repo.families["fam-1"].OwnerID != "user-2" {
		t.Fatalf("expected owner reassigned to user-2, got %s", repo.families["fam-1"].OwnerID)
	}
	member := repo.members["user-2"]
	if member == nil || member.Role != RoleOwner {
		t.Fatalf("expected user-2 to be owner, got %+v", member)
	}
	if _, ok := repo.members["owner"]; ok {
		t.Fatalf("expected owner membership deleted")
	}
}

func TestLeaveFamilyOwnerSolo(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "owner"}
	repo.members["owner"] = &FamilyMember{FamilyID: "fam-1", UserID: "owner", Role: RoleOwner}

	svc := NewService(repo)
	if err := svc.LeaveFamily(context.Background(), "owner"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := repo.families["fam-1"]; !ok {
		t.Fatalf("expected family to remain")
	}
	if _, ok := repo.members["owner"]; ok {
		t.Fatalf("expected owner membership deleted")
	}
}

func TestUpdateFamily(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "user-1"}
	repo.members["user-1"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-1", Role: RoleOwner}

	svc := NewService(repo)
	result, err := svc.UpdateFamily(context.Background(), "user-1", "New Name")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Name != "New Name" {
		t.Fatalf("expected updated name, got %q", result.Name)
	}
}

func TestListMembers(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "user-1"}
	repo.members["user-1"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-1", Role: RoleOwner}
	repo.members["user-2"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-2", Role: RoleMember}

	svc := NewService(repo)
	members, err := svc.ListMembers(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}

func TestRemoveMemberNotOwner(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "owner"}
	repo.members["owner"] = &FamilyMember{FamilyID: "fam-1", UserID: "owner", Role: RoleOwner}
	repo.members["user-1"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-1", Role: RoleMember}
	repo.members["user-2"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-2", Role: RoleMember}

	svc := NewService(repo)
	err := svc.RemoveMember(context.Background(), "user-1", "user-2")
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
}

func TestRemoveMemberCannotRemoveOwner(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "owner"}
	repo.members["owner"] = &FamilyMember{FamilyID: "fam-1", UserID: "owner", Role: RoleOwner}
	repo.members["user-1"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-1", Role: RoleMember}

	svc := NewService(repo)
	err := svc.RemoveMember(context.Background(), "owner", "owner")
	if !errors.Is(err, ErrCannotRemoveOwner) {
		t.Fatalf("expected ErrCannotRemoveOwner, got %v", err)
	}
}

func TestRemoveMemberSuccess(t *testing.T) {
	repo := newFakeFamilyRepo()
	repo.families["fam-1"] = &Family{ID: "fam-1", Name: "Fam", Code: "ZXCVBN", OwnerID: "owner"}
	repo.members["owner"] = &FamilyMember{FamilyID: "fam-1", UserID: "owner", Role: RoleOwner}
	repo.members["user-1"] = &FamilyMember{FamilyID: "fam-1", UserID: "user-1", Role: RoleMember}

	svc := NewService(repo)
	if err := svc.RemoveMember(context.Background(), "owner", "user-1"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := repo.members["user-1"]; ok {
		t.Fatalf("expected member removed")
	}
}
