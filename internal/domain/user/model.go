package user

import "time"

type Profile struct {
	UserID    string     `gorm:"type:uuid;primaryKey"`
	Email     *string    `gorm:"type:text"`
	AvatarURL *string    `gorm:"type:text"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime"`
}
