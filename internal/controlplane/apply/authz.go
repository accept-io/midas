package apply

import "context"

// KindAuthorizer reports whether the caller is authorized to write a
// document of the given Kind in a bundle apply. It is the fine-grained
// layer of the two-tier authorization model documented in
// docs/authorization.md and docs/design/040-finer-grained-write-authz.md.
//
// Contract:
//   - allowed=true, missingPermission=""   → document may be planned.
//   - allowed=false, missingPermission=<p> → document is marked invalid
//     with a validation error quoting <p>; the rest of the bundle is not
//     short-circuited so the operator sees every denial at once.
//
// The HTTP layer (see internal/httpapi) constructs a KindAuthorizer from
// the request principal and the internal/authz permission model. The apply
// package does not import either of those; the authorizer is injected via
// context so that the apply.Service itself remains agnostic to the wire
// representation of principals and permissions.
//
// When no KindAuthorizer is present in context, the planner does not
// enforce per-document authorization. This preserves the historical
// behaviour used by tests that call Service methods directly without the
// HTTP middleware chain, and is the behaviour in AuthModeOpen where the
// HTTP layer intentionally skips principal extraction.
type KindAuthorizer func(kind string) (allowed bool, missingPermission string)

// kindAuthorizerCtxKey is the unexported context key used to carry a
// KindAuthorizer through an Apply/Plan call. An unexported struct type
// guarantees no collision with keys from other packages.
type kindAuthorizerCtxKey struct{}

// WithKindAuthorizer returns a new context carrying fn as the per-document
// authorizer for this apply/plan invocation. Passing a nil fn removes any
// previously-set authorizer, which disables per-document enforcement for
// the remainder of the call chain.
func WithKindAuthorizer(ctx context.Context, fn KindAuthorizer) context.Context {
	return context.WithValue(ctx, kindAuthorizerCtxKey{}, fn)
}

// kindAuthorizerFromContext extracts the KindAuthorizer attached to ctx by
// WithKindAuthorizer. Returns nil when no authorizer has been set (tests,
// open auth mode, or direct Service calls that bypass the HTTP layer).
func kindAuthorizerFromContext(ctx context.Context) KindAuthorizer {
	fn, _ := ctx.Value(kindAuthorizerCtxKey{}).(KindAuthorizer)
	return fn
}

// AuthorizerFromContextForTest is an exported accessor that returns the
// KindAuthorizer attached to ctx by WithKindAuthorizer. It exists solely
// to support cross-package end-to-end tests (see internal/httpapi
// bootstrap-admin regression) that need to assert the authorizer round-
// trips through the apply.Service's Apply/Plan entry points.
//
// Production code must not call this function; the planner uses the
// unexported kindAuthorizerFromContext helper instead.
func AuthorizerFromContextForTest(ctx context.Context) KindAuthorizer {
	return kindAuthorizerFromContext(ctx)
}

// authorizeKind is the helper the planner uses to apply per-document
// authorization. It returns (denied, missingPermission):
//
//   - denied=false, missingPermission=""  → proceed with planning
//   - denied=true,  missingPermission=<p> → mark entry invalid with <p>
//
// When no KindAuthorizer is present in ctx, authorization is not enforced
// and the helper returns (false, ""). See KindAuthorizer doc for rationale.
func authorizeKind(ctx context.Context, kind string) (bool, string) {
	fn := kindAuthorizerFromContext(ctx)
	if fn == nil {
		return false, ""
	}
	allowed, missingPerm := fn(kind)
	if allowed {
		return false, ""
	}
	return true, missingPerm
}
