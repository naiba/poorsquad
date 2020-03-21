package model

import (
	"fmt"
	"time"

	"github.com/google/go-github/v28/github"
	"github.com/jinzhu/gorm"
	"github.com/naiba/com"
)

// User ...
type User struct {
	Common    `json:"common,omitempty"`
	Login     string `gorm:"UNIQUE_INDEX" json:"login,omitempty"` // 登录名
	AvatarURL string `json:"avatar_url,omitempty"`                // 头像地址
	Name      string `json:"name,omitempty"`                      // 昵称
	Blog      string `json:"blog,omitempty"`                      // 网站链接
	Email     string `json:"email,omitempty"`                     // 邮箱
	Hireable  bool   `json:"hireable,omitempty"`
	Bio       string `json:"bio,omitempty"` // 个人简介

	Token        string    `gorm:"UNIQUE_INDEX" json:"-"`   // 认证 Token
	TokenExpired time.Time `json:"token_expired,omitempty"` // Token 过期时间
	SuperAdmin   bool      `json:"super_admin,omitempty"`   // 超级管理员

	TeamsID []uint64 `gorm:"-"`
}

// NewUserFromGitHub ..
func NewUserFromGitHub(gu *github.User) User {
	var u User
	u.ID = uint64(gu.GetID())
	u.Login = gu.GetLogin()
	u.AvatarURL = gu.GetAvatarURL()
	u.Name = gu.GetName()
	// 昵称为空的情况
	if u.Name == "" {
		u.Name = u.Login
	}
	u.Blog = gu.GetBlog()
	u.Blog = gu.GetBlog()
	u.Email = gu.GetEmail()
	u.Hireable = gu.GetHireable()
	u.Bio = gu.GetBio()
	return u
}

// IssueNewToken ...
func (u *User) IssueNewToken() {
	u.Token = com.MD5(fmt.Sprintf("%d%d%s", time.Now().UnixNano(), u.ID, u.Login))
	u.TokenExpired = time.Now().AddDate(0, 0, 14)
}

// FetchTeams ..
func (u *User) FetchTeams(db *gorm.DB) {
	var uts []UserTeam
	db.Where("user_id = ?", u.ID).Find(&uts)
	for i := 0; i < len(uts); i++ {
		u.TeamsID = append(u.TeamsID, uts[i].TeamID)
	}
}

// InTeams ...
func (u *User) InTeams(db *gorm.DB, teamIDs []uint64) (bool, error) {
	var count int
	err := db.Model(&UserTeam{}).Where("user_id = ? AND team_id IN (?)", u.ID, teamIDs).Count(&count).Error
	return count > 0, err
}
