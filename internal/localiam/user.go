package localiam

import (
	"context"
	"time"
)

// User is a local platform IAM user. Users represent human operators and
// administrators of the MIDAS control plane only — they are entirely separate
// from runtime authority (agents, surfaces, grants).
type User struct {
	ID                 string
	Username           string
	PasswordHash       string // bcrypt hash; never store or log plaintext
	Roles              []string
	Enabled            bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        *time.Time // nil until first successful login
}

// UserRepository defines persistence for local IAM users.
type UserRepository interface {
	// FindByID returns nil, nil when not found.
	FindByID(ctx context.Context, id string) (*User, error)
	// FindByUsername returns nil, nil when not found.
	FindByUsername(ctx context.Context, username string) (*User, error)
	// Create inserts a new user. Returns an error if the username is already taken.
	Create(ctx context.Context, u *User) error
	// Update overwrites all mutable fields for an existing user.
	Update(ctx context.Context, u *User) error
	// Count returns the total number of users in the store.
	Count(ctx context.Context) (int, error)
}
