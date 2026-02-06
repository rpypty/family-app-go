package todos

import (
	"time"

	"gorm.io/gorm"
)

type TodoList struct {
	ID               string         `gorm:"type:uuid;primaryKey"`
	FamilyID         string         `gorm:"type:uuid;index;not null"`
	Title            string         `gorm:"not null"`
	ArchiveCompleted bool           `gorm:"not null;default:false;column:archive_completed"`
	CreatedAt        time.Time      `gorm:"autoCreateTime"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

type TodoItem struct {
	ID                   string    `gorm:"type:uuid;primaryKey"`
	ListID               string    `gorm:"type:uuid;index;not null"`
	Title                string    `gorm:"not null"`
	IsCompleted          bool      `gorm:"not null;default:false"`
	IsArchived           bool      `gorm:"not null;default:false"`
	CreatedAt            time.Time `gorm:"autoCreateTime"`
	CompletedAt          *time.Time
	CompletedByID        *string        `gorm:"column:completed_by_id"`
	CompletedByName      *string        `gorm:"column:completed_by_name"`
	CompletedByEmail     *string        `gorm:"column:completed_by_email"`
	CompletedByAvatarURL *string        `gorm:"column:completed_by_avatar_url"`
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}

type UserSnapshot struct {
	ID        string
	Name      string
	Email     string
	AvatarURL string
}

type ListFilter struct {
	Query  string
	Limit  int
	Offset int
}

type ArchivedFilter string

const (
	ArchivedExclude ArchivedFilter = "exclude"
	ArchivedOnly    ArchivedFilter = "only"
	ArchivedAll     ArchivedFilter = "all"
)

type ListItemCounts struct {
	ItemsTotal     int64
	ItemsCompleted int64
	ItemsArchived  int64
}

type ListWithItems struct {
	List   TodoList
	Counts ListItemCounts
	Items  []TodoItem
}

type CreateTodoListInput struct {
	FamilyID         string
	Title            string
	ArchiveCompleted bool
}

type UpdateTodoListInput struct {
	ID               string
	FamilyID         string
	Title            *string
	ArchiveCompleted *bool
}

type CreateTodoItemInput struct {
	ListID string
	Title  string
}

type UpdateTodoItemInput struct {
	ID          string
	FamilyID    string
	Title       *string
	IsCompleted *bool
	CompletedBy *UserSnapshot
}
