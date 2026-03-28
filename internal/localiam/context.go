package localiam

import "context"

type ctxKey int

const mustChangePasswordKey ctxKey = iota

// WithMustChangePassword stores the must_change_password flag in ctx.
// Called by the session auth middleware after resolving a user.
func WithMustChangePassword(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, mustChangePasswordKey, v)
}

// MustChangePasswordFromContext retrieves the must_change_password flag.
// Returns false when not set (i.e. no session, or session resolved without flag).
func MustChangePasswordFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(mustChangePasswordKey).(bool)
	return v
}
