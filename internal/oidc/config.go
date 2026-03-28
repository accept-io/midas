package oidc

// Provider is the identity.Principal.Provider value for OIDC-authenticated principals.
const Provider = "oidc"

// Config holds runtime OIDC configuration passed to NewService.
// This mirrors config.PlatformOIDCConfig; main.go converts between them.
type Config struct {
	ProviderName string

	IssuerURL string
	AuthURL   string
	TokenURL  string

	ClientID     string
	ClientSecret string
	RedirectURL  string

	Scopes []string

	SubjectClaim  string
	UsernameClaim string
	GroupsClaim   string

	DomainHint    string
	AllowedGroups []string

	RoleMappings []RoleMapping

	DenyIfNoRoles bool
	UsePKCE       bool
}

// RoleMapping maps a single external group identifier to a MIDAS canonical role.
type RoleMapping struct {
	External string
	Internal string
}
