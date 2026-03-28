package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// LocalUserRepo implements localiam.UserRepository against Postgres.
type LocalUserRepo struct {
	db sqltx.DBTX
}

func NewLocalUserRepo(db sqltx.DBTX) (*LocalUserRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &LocalUserRepo{db: db}, nil
}

func (r *LocalUserRepo) FindByID(ctx context.Context, id string) (*localiam.User, error) {
	const q = `
		SELECT id, username, password_hash, roles, enabled,
		       must_change_password, created_at, updated_at, last_login_at
		FROM platform_users WHERE id = $1`
	return r.scanUser(r.db.QueryRowContext(ctx, q, id))
}

func (r *LocalUserRepo) FindByUsername(ctx context.Context, username string) (*localiam.User, error) {
	const q = `
		SELECT id, username, password_hash, roles, enabled,
		       must_change_password, created_at, updated_at, last_login_at
		FROM platform_users WHERE username = $1`
	return r.scanUser(r.db.QueryRowContext(ctx, q, username))
}

func (r *LocalUserRepo) Create(ctx context.Context, u *localiam.User) error {
	const q = `
		INSERT INTO platform_users
		  (id, username, password_hash, roles, enabled,
		   must_change_password, created_at, updated_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, q,
		u.ID, u.Username, u.PasswordHash, encodeRoles(u.Roles), u.Enabled,
		u.MustChangePassword, u.CreatedAt, u.UpdatedAt, u.LastLoginAt,
	)
	return err
}

func (r *LocalUserRepo) Update(ctx context.Context, u *localiam.User) error {
	const q = `
		UPDATE platform_users SET
		  username = $2, password_hash = $3, roles = $4, enabled = $5,
		  must_change_password = $6, updated_at = $7, last_login_at = $8
		WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q,
		u.ID, u.Username, u.PasswordHash, encodeRoles(u.Roles), u.Enabled,
		u.MustChangePassword, u.UpdatedAt, u.LastLoginAt,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("localiam: user %q not found", u.ID)
	}
	return nil
}

func (r *LocalUserRepo) Count(ctx context.Context) (int, error) {
	const q = `SELECT COUNT(*) FROM platform_users`
	var n int
	if err := r.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *LocalUserRepo) scanUser(row *sql.Row) (*localiam.User, error) {
	var u localiam.User
	var rolesStr string
	var lastLoginAt sql.NullTime
	err := row.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &rolesStr, &u.Enabled,
		&u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt, &lastLoginAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.Roles = decodeRoles(rolesStr)
	if lastLoginAt.Valid {
		t := lastLoginAt.Time
		u.LastLoginAt = &t
	}
	return &u, nil
}

// LocalSessionRepo implements localiam.SessionRepository against Postgres.
type LocalSessionRepo struct {
	db sqltx.DBTX
}

func NewLocalSessionRepo(db sqltx.DBTX) (*LocalSessionRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &LocalSessionRepo{db: db}, nil
}

func (r *LocalSessionRepo) Create(ctx context.Context, s *localiam.Session) error {
	const q = `
		INSERT INTO platform_sessions (id, user_id, created_at, expires_at, principal_json)
		VALUES ($1, $2, $3, $4, $5)`
	userID := sql.NullString{String: s.UserID, Valid: s.UserID != ""}
	principalJSON := sql.NullString{String: s.PrincipalJSON, Valid: s.PrincipalJSON != ""}
	_, err := r.db.ExecContext(ctx, q, s.ID, userID, s.CreatedAt, s.ExpiresAt, principalJSON)
	return err
}

func (r *LocalSessionRepo) FindByID(ctx context.Context, id string) (*localiam.Session, error) {
	const q = `
		SELECT id, user_id, created_at, expires_at, principal_json
		FROM platform_sessions WHERE id = $1`
	var s localiam.Session
	var userID sql.NullString
	var principalJSON sql.NullString
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&s.ID, &userID, &s.CreatedAt, &s.ExpiresAt, &principalJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.UserID = userID.String
	s.PrincipalJSON = principalJSON.String
	return &s, nil
}

func (r *LocalSessionRepo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM platform_sessions WHERE id = $1`
	_, err := r.db.ExecContext(ctx, q, id)
	return err
}

func (r *LocalSessionRepo) DeleteExpired(ctx context.Context) error {
	const q = `DELETE FROM platform_sessions WHERE expires_at < $1`
	_, err := r.db.ExecContext(ctx, q, time.Now().UTC())
	return err
}

// encodeRoles serialises a role slice as a comma-separated string for storage.
func encodeRoles(roles []string) string {
	return strings.Join(roles, ",")
}

// decodeRoles parses a comma-separated role string back to a slice.
func decodeRoles(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
