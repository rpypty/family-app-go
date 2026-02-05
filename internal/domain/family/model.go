package family

import "time"

const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Family struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	Name      string    `gorm:"not null"`
	Code      string    `gorm:"size:6;not null;uniqueIndex"`
	OwnerID   string    `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type FamilyMember struct {
	FamilyID string    `gorm:"type:uuid;primaryKey"`
	UserID   string    `gorm:"primaryKey;uniqueIndex"`
	Role     string    `gorm:"type:varchar(16);not null"`
	JoinedAt time.Time `gorm:"autoCreateTime"`

	Family Family `gorm:"foreignKey:FamilyID;references:ID;constraint:OnDelete:CASCADE"`
}
