# Authentication

MIDAS has two distinct authentication surfaces that operate independently:

- **Explorer / platform UI** — browser-based access to the Explorer sandbox and admin console. Handled by Local IAM (username/password + session cookies) or OIDC/SSO.
- **`/v1/*` API routes** — machine-to-machine access for governance evaluation and control-plane operations. Handled by static bearer token authentication.

These two surfaces are deliberately separate. Configuring OIDC for the Explorer does not affect how `/v1/*` clients authenticate, and vice versa.

---

## Local IAM

Local IAM provides username/password authentication with session cookies for the Explorer. It is the default authentication mechanism for local development.

**When it is used:**
- When `local_iam.enabled: true` (default) and `server.headless: false` (default).
- Provides login at `/auth/login`, logout at `/auth/logout`, and session management via the `midas_session` cookie.

**Bootstrap credentials:**
- On first run, MIDAS creates an `admin` account with password `admin`.
- Login forces an immediate password change before further access is granted.
- Do not use the bootstrap password in any environment accessible to other people.

**Configuration:**

```yaml
local_iam:
  enabled: true
  session_ttl: 8h          # how long a session remains valid
  secure_cookies: false    # set true in production (requires HTTPS)
```

| Config field | Default | Env var |
|---|---|---|
| `local_iam.enabled` | `true` | `MIDAS_LOCAL_IAM_ENABLED` |
| `local_iam.session_ttl` | `8h` | — |
| `local_iam.secure_cookies` | `false` | — |

**Security note:** Set `secure_cookies: true` in any environment served over HTTPS.

---

## OIDC / SSO

MIDAS supports OIDC-based authentication for the Explorer and platform console. The implementation is provider-agnostic — it works with any OIDC-compliant identity provider. Configuration examples are provided for Microsoft Entra ID and Google Workspace.

**When it is used:**
- When `platform_oidc.enabled: true`. Requires `local_iam.enabled: true` (Local IAM manages the session after OIDC authentication).
- Registers `/auth/oidc/login` and `/auth/oidc/callback`.
- On successful authentication, MIDAS maps the provider's group claims to internal roles using `role_mappings`.

**How providers are named:**
`provider_name` is a display label only (e.g. `entra`, `google`). It has no effect on authentication logic. All OIDC providers are handled by the same implementation.

**Configuration:**

```yaml
platform_oidc:
  enabled: true
  provider_name: entra          # display label only

  issuer_url: ""                # OIDC discovery endpoint
  client_id: ""                 # use MIDAS_OIDC_CLIENT_ID env var
  client_secret: ""             # use MIDAS_OIDC_CLIENT_SECRET env var
  redirect_url: ""              # must match your IdP's registered callback URI

  scopes:
    - openid
    - profile
    - email

  subject_claim: sub
  username_claim: preferred_username
  groups_claim: groups

  role_mappings:
    - external: platform-admins
      internal: platform.admin
    - external: governance-team
      internal: governance.approver

  deny_if_no_roles: true        # reject login when no roles are mapped
  use_pkce: true                # recommended; required by Google
```

**Provider-specific issuer URLs:**

| Provider | `issuer_url` |
|---|---|
| Microsoft Entra ID | `https://login.microsoftonline.com/<tenant-id>/v2.0` |
| Google Workspace | `https://accounts.google.com` |

**Provider-specific claim names:**

| Claim | Microsoft Entra ID | Google Workspace |
|---|---|---|
| `username_claim` | `preferred_username` | `email` |
| `groups_claim` | `groups` | `hd` (hosted domain) |

**Optional fields:**

- `auth_url` — overrides the discovered authorization endpoint.
- `token_url` — overrides the discovered token endpoint.
- `domain_hint` — login hint passed to the provider (Entra uses this to pre-fill the tenant).
- `allowed_groups` — restricts login to members of at least one listed group; empty means any authenticated user.

| Config field | Default | Env var |
|---|---|---|
| `platform_oidc.enabled` | `false` | `MIDAS_OIDC_ENABLED` |
| `platform_oidc.client_id` | — | `MIDAS_OIDC_CLIENT_ID` |
| `platform_oidc.client_secret` | — | `MIDAS_OIDC_CLIENT_SECRET` |
| All other fields | — | — (midas.yaml only) |

**Role mapping:** The `role_mappings` list maps external group identifiers (as returned by the identity provider) to MIDAS internal roles. If `deny_if_no_roles: true` (default), users with no matching group are rejected at login.

Available internal roles: `platform.admin`, `platform.operator`, `platform.viewer`, `governance.approver`, `governance.reviewer`. See [docs/authorization.md](../authorization.md) for the permission bundle each role expands to on control-plane write operations.

---

## API authentication (`/v1/*`)

All `/v1/*` governance routes are protected independently of Local IAM and OIDC. Bearer token authentication is used for machine-to-machine access.

**Auth modes:**

| `auth.mode` | Behaviour |
|---|---|
| `open` | No authentication required. Logs `UNSAFE FOR PRODUCTION` at startup. |
| `required` | Valid bearer token mandatory on all governed routes. |

`open` is the default. It is appropriate for local development. Do not use it in production.

**Configuring static bearer tokens:**

```yaml
auth:
  mode: required
  tokens:
    - token: "${MY_SECRET_TOKEN}"      # supports ${VAR} expansion
      principal: svc:payments
      roles: platform.operator
    - token: "${ADMIN_TOKEN}"
      principal: user:alice
      roles: platform.admin,governance.approver
```

Or via environment variable:

```bash
export MIDAS_AUTH_MODE=required
export MIDAS_AUTH_TOKENS="<token>|svc:payments|platform.operator;<token2>|user:alice|platform.admin,governance.approver"
```

Token format: `token|principal-id|role1,role2`. Entries are semicolon-separated.

Generate tokens with: `openssl rand -base64 32`

| Config field | Default | Env var |
|---|---|---|
| `auth.mode` | `open` | `MIDAS_AUTH_MODE` |
| `auth.tokens` | `[]` | `MIDAS_AUTH_TOKENS` |

---

## Default behaviour

With no configuration file and no environment variables, MIDAS starts with these defaults:

- `auth.mode: open` — no bearer token required on `/v1/*`
- `local_iam.enabled: true` — Explorer login via username/password; bootstrap `admin`/`admin`
- `platform_oidc.enabled: false` — no SSO
- `server.explorer_enabled: true` — Explorer available at `/explorer`
- `server.headless: false` — all browser-facing routes active

This default is appropriate for local development and `make dev`. It is **not appropriate for production**.

---

## Deployment patterns

### Local development

```yaml
# defaults — no config file needed
# auth.mode: open
# local_iam.enabled: true
# platform_oidc.enabled: false
```

Run `go run ./cmd/midas` or `make dev`. Explorer is available immediately. Login with `admin` / `admin` and change the password on first use.

### Production with SSO (Explorer-enabled)

```yaml
auth:
  mode: required
  tokens:
    - token: "${API_TOKEN}"
      principal: svc:evaluator
      roles: platform.operator

local_iam:
  enabled: true
  secure_cookies: true

platform_oidc:
  enabled: true
  issuer_url: "https://login.microsoftonline.com/<tenant>/v2.0"
  client_id: "${MIDAS_OIDC_CLIENT_ID}"
  client_secret: "${MIDAS_OIDC_CLIENT_SECRET}"
  redirect_url: "https://midas.example.com/auth/oidc/callback"
  role_mappings:
    - external: platform-admins
      internal: platform.admin
  deny_if_no_roles: true
  use_pkce: true
```

### Headless API-only mode

```yaml
server:
  headless: true      # disables Explorer, /auth/*, local IAM, and OIDC

auth:
  mode: required
  tokens:
    - token: "${API_TOKEN}"
      principal: svc:evaluator
      roles: platform.operator
```

Use `headless: true` when MIDAS is deployed as a pure API backend with no browser access. All `/v1/*` and health routes remain active. `server.explorer_enabled` and `local_iam.enabled` must not be set to `true` when headless is `true`.

---

## Security notes

- **Do not expose `auth.mode: open` in production.** MIDAS logs a warning at startup, but it is the operator's responsibility to ensure this mode is not reachable outside a trusted network.
- **Use OIDC for production Explorer access.** Local IAM is intended for development and initial bootstrap only. In production, disable direct password access by configuring `platform_oidc.enabled: true` and removing or rotating the bootstrap password.
- **Set `secure_cookies: true` in any HTTPS deployment.** Without it, session cookies are sent over HTTP and are vulnerable to interception.
- **Rotate bearer tokens regularly.** Static bearer tokens do not expire automatically. Use `${VAR}` expansion in `midas.yaml` to manage secrets via environment variables rather than embedding them in config files.
- **`deny_if_no_roles: true` is the default** for OIDC. Do not set it to `false` unless you intend to permit any authenticated user regardless of group membership.
