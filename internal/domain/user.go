package domain

import (
	"errors"
	"time"
)

type UserID string
type TeamID string

var (
	ErrInvalidUserID   = errors.New("invalid user ID")
	ErrInvalidUsername = errors.New("invalid username")
	ErrInvalidTeamID   = errors.New("invalid team ID")
)

const (
	MinUsernameLength = 1
	MaxUsernameLength = 50
)

type User struct {
	ID        UserID    `db:"user_id" json:"user_id"`
	Username  string    `db:"username" json:"username"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	TeamName  TeamID    `db:"team_name" json:"team_name"`
	CreatedAt time.Time `db:"created_at" json:"-"`
	UpdatedAt time.Time `db:"updated_at" json:"-"`
}

func NewUser(id, username string, team TeamID, isActive bool) *User {
	now := time.Now()
	return &User{
		ID:        UserID(id),
		Username:  username,
		TeamName:  team,
		IsActive:  isActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (u *User) Activate() {
	u.IsActive = true
	u.UpdatedAt = time.Now()
}

func (u *User) Deactivate() {
	u.IsActive = false
	u.UpdatedAt = time.Now()
}
