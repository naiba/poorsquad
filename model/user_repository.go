package model

// UserRepository ..
type UserRepository struct {
	UserID       uint64 `gorm:"primary_key;auto_increment:false"`
	RepositoryID uint64 `gorm:"primary_key;auto_increment:false"`
	AccountID    uint64 `gorm:"index;"`
}