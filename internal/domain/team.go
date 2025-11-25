package domain

import "time"

type Team struct {
	Name        string    `db:"name" json:"team_name"`
	Description string    `db:"description,omitempty" json:"-"`
	CreatedAt   time.Time `db:"created_at" json:"-"`
}
