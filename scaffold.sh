#!/usr/bin/env bash
# Accept MIDAS (Community) — Repository Scaffold
# Run from the root of your cloned midas repo:
#   chmod +x scaffold.sh && ./scaffold.sh

set -euo pipefail

echo "Scaffolding Accept MIDAS (Community)..."

# ─────────────────────────────────────────────────────────────
# cmd
# ─────────────────────────────────────────────────────────────
mkdir -p cmd/midas

cat > cmd/midas/main.go <<'GO'
package main

import (
	"log"

	"github.com/accept-io/midas/internal/httpapi"
)

func main() {
	srv := httpapi.NewServer()

	log.Println("MIDAS listening on :8080")
	log.Fatal(srv.ListenAndServe(":8080"))
}
GO

# ─────────────────────────────────────────────────────────────
# internal domain packages
# ─────────────────────────────────────────────────────────────
for pkg in surface agent authority envelope decision policy escalation review audit metrics events; do
  mkdir -p "internal/${pkg}"
  cat > "internal/${pkg}/${pkg}.go" <<GO
package ${pkg}
GO
done

# httpapi
mkdir -p internal/httpapi

cat > internal/httpapi/server.go <<'GO'
package httpapi

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	mux *http.ServeMux
}

func NewServer() *Server {
	mux := http.NewServeMux()

	s := &Server{mux: mux}
	s.routes()

	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReady)
	s.mux.HandleFunc("/v1/evaluate", s.handleEvaluate)
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "midas",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ready",
		"service": "midas",
	})
}

type evaluateRequest struct {
	SurfaceID   string      `json:"surface_id"`
	AgentID     string      `json:"agent_id"`
	Confidence  float64     `json:"confidence"`
	Consequence interface{} `json:"consequence,omitempty"`
}

type evaluateResponse struct {
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason"`
	EnvelopeID string `json:"envelope_id,omitempty"`
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var req evaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON payload",
		})
		return
	}

	if req.SurfaceID == "" || req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "surface_id and agent_id are required",
		})
		return
	}

	resp := evaluateResponse{
		Outcome:    "RequestClarification",
		Reason:     "evaluation pipeline not yet implemented",
		EnvelopeID: "env_placeholder",
	}

	writeJSON(w, http.StatusOK, resp)
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error": "method not allowed",
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
GO

# store
mkdir -p internal/store/postgres

cat > internal/store/postgres/agent_repo.go <<'GO'
package postgres
GO

cat > internal/store/postgres/surface_repo.go <<'GO'
package postgres
GO

cat > internal/store/postgres/authority_repo.go <<'GO'
package postgres
GO

cat > internal/store/postgres/envelope_repo.go <<'GO'
package postgres
GO

mkdir -p internal/store/migrations
touch internal/store/migrations/.gitkeep

# ─────────────────────────────────────────────────────────────
# policies
# ─────────────────────────────────────────────────────────────
mkdir -p policies/lending policies/refunds policies/fraud
touch policies/lending/.gitkeep
touch policies/refunds/.gitkeep
touch policies/fraud/.gitkeep

# ─────────────────────────────────────────────────────────────
# public API spec
# ─────────────────────────────────────────────────────────────
mkdir -p api/openapi

cat > api/openapi/v1.yaml <<'YAML'
openapi: "3.1.0"
info:
  title: Accept MIDAS API
  version: 0.1.0
  description: Decision authority engine for governing autonomous actors.
paths:
  /healthz:
    get:
      summary: Health check
      responses:
        "200":
          description: OK
  /readyz:
    get:
      summary: Readiness check
      responses:
        "200":
          description: Ready
  /v1/evaluate:
    post:
      summary: Evaluate a decision request
      responses:
        "200":
          description: Evaluation response
YAML

# ─────────────────────────────────────────────────────────────
# deploy
# ─────────────────────────────────────────────────────────────
mkdir -p deploy/docker deploy/compose

cat > deploy/docker/Dockerfile <<'DOCKER'
FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod tidy || true

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /midas ./cmd/midas

FROM alpine:3.20
COPY --from=build /midas /usr/local/bin/midas
EXPOSE 8080
ENTRYPOINT ["midas"]
DOCKER

cat > deploy/compose/docker-compose.yml <<'YAML'
services:
  midas:
    build:
      context: ../..
      dockerfile: deploy/docker/Dockerfile
    ports:
      - "8080:8080"
    depends_on:
      - postgres
    environment:
      DATABASE_URL: postgres://midas:midas@postgres:5432/midas?sslmode=disable

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: midas
      POSTGRES_PASSWORD: midas
      POSTGRES_DB: midas
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
YAML

# ─────────────────────────────────────────────────────────────
# scripts
# ─────────────────────────────────────────────────────────────
mkdir -p scripts

cat > scripts/migrate.sh <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
echo "Running migrations..."
BASH
chmod +x scripts/migrate.sh

cat > scripts/seed.sh <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
echo "Seeding example data..."
BASH
chmod +x scripts/seed.sh

# ─────────────────────────────────────────────────────────────
# docs
# ─────────────────────────────────────────────────────────────
mkdir -p docs/architecture docs/concepts docs/guides

cat > docs/architecture/evaluation-flow.md <<'MD'
# Evaluation Flow

The MIDAS orchestrator evaluates a request through a deterministic sequence of resolution, validation, threshold checks, policy evaluation, and outcome recording.

Detailed flow documentation to follow.
MD

cat > docs/guides/getting-started.md <<'MD'
# Getting Started

## Prerequisites

- Go 1.23+
- Docker (optional, for local compose-based development)

## Run locally

```bash
go run ./cmd/midas
```

## Health check

```bash
curl http://localhost:8080/healthz
```
MD

cat > docs/concepts/decision-surfaces.md <<'MD'
# Decision Surfaces

A decision surface is a bounded point at which an actor may be authorised to make or execute a decision.

Detailed modelling to follow.
MD

cat > docs/concepts/authority-model.md <<'MD'
# Authority Model

MIDAS evaluates whether a given actor is authorised to act on a given decision surface within a defined operational envelope.

Detailed authority rules to follow.
MD

cat > docs/concepts/operational-envelope.md <<'MD'
# Operational Envelope

Every MIDAS evaluation creates an operational envelope that tracks lifecycle state, evidence references, and the final authority outcome.

Detailed lifecycle modelling to follow.
MD

cat > docs/guides/rego-policies.md <<'MD'
# Writing Rego Policies

MIDAS is intended to support embedded OPA / Rego policy evaluation.

Policy examples will be added as the runtime matures.
MD

# ─────────────────────────────────────────────────────────────
# GitHub
# ─────────────────────────────────────────────────────────────
mkdir -p .github/workflows .github/ISSUE_TEMPLATE

cat > .github/workflows/ci.yml <<'YAML'
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - run: go test ./...
      - run: go vet ./...
YAML

cat > .github/ISSUE_TEMPLATE/bug_report.md <<'MD'
---
name: Bug Report
about: Report a bug in Accept MIDAS
labels: bug
---

**Describe the bug**

**To reproduce**

**Expected behaviour**

**MIDAS version**
MD

cat > .github/ISSUE_TEMPLATE/feature_request.md <<'MD'
---
name: Feature Request
about: Suggest a feature for Accept MIDAS
labels: enhancement
---

**Problem**

**Proposed solution**
MD

cat > .github/ISSUE_TEMPLATE/config.yml <<'YAML'
blank_issues_enabled: true
contact_links:
  - name: Community Discussions
    url: https://github.com/accept-io/community/discussions
    about: Ask questions and share ideas
YAML

cat > .github/PULL_REQUEST_TEMPLATE.md <<'MD'
## What

## Why

## How

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] `go vet` passes
- [ ] `go test ./...` passes
MD

# Only add CODEOWNERS if the team/user exists.
# touch .github/CODEOWNERS

# ─────────────────────────────────────────────────────────────
# go.mod
# ─────────────────────────────────────────────────────────────
cat > go.mod <<'MOD'
module github.com/accept-io/midas

go 1.23
MOD

# Create empty go.sum so Docker COPY won't fail before first tidy.
touch go.sum

# ─────────────────────────────────────────────────────────────
# Makefile
# ─────────────────────────────────────────────────────────────
cat > Makefile <<'MAKE'
.PHONY: build test lint docker migrate seed dev run tidy

build:
	mkdir -p bin
	go build -o bin/midas ./cmd/midas

test:
	go test ./...

lint:
	go vet ./...

docker:
	docker build -f deploy/docker/Dockerfile -t accept-midas:latest .

migrate:
	./scripts/migrate.sh

seed:
	./scripts/seed.sh

dev:
	docker compose -f deploy/compose/docker-compose.yml up --build

run:
	go run ./cmd/midas

tidy:
	go mod tidy
MAKE

# ─────────────────────────────────────────────────────────────
# root files
# ─────────────────────────────────────────────────────────────
cat > README.md <<'MD'
# Accept MIDAS

Open-source decision governance platform for governing AI agents, automated models and human actors.

## Status

This repository is in an early scaffold phase. The initial HTTP runtime is in place, with core domain modules, persistence, and policy integration to follow.

## What is MIDAS?

MIDAS is a centralised authority orchestration platform that evaluates whether autonomous actors are authorised to execute decisions. Instead of embedding authority logic in every service, callers submit a decision request and receive an authority outcome such as:

- Execute
- Escalate
- Reject
- Request Clarification

## Quick Start

```bash
git clone https://github.com/accept-io/midas.git
cd midas
make run
```

In a second terminal:

```bash
curl http://localhost:8080/healthz
```

Example evaluation request:

```bash
curl -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "loan-approval",
    "agent_id": "lending-model-v3",
    "confidence": 0.87,
    "consequence": { "currency": "GBP", "amount": 4500 }
  }'
```

## Build

```bash
make build
make test
make docker
make dev
```

## Documentation

- [Platform Architecture](docs/architecture/platform-architecture.md)
- [Evaluation Flow](docs/architecture/evaluation-flow.md)
- [Getting Started](docs/guides/getting-started.md)
- [Decision Surfaces](docs/concepts/decision-surfaces.md)
- [Authority Model](docs/concepts/authority-model.md)
- [Operational Envelope](docs/concepts/operational-envelope.md)
- [Writing Rego Policies](docs/guides/rego-policies.md)

## Enterprise

MIDAS Enterprise adds time-bounded authority, threshold governance, escalation SLA management, RBAC, drift detection, and OpenTelemetry integration.

## Community

- Discussions: https://github.com/accept-io/community/discussions
- [Contributing](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)

## License

Apache License 2.0. See [LICENSE](LICENSE).
MD

cat > CONTRIBUTING.md <<'MD'
# Contributing to Accept MIDAS

Thank you for your interest in contributing to Accept MIDAS.

## Getting Started

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Run `make test`
5. Open a pull request

## Code Style

- Follow standard Go conventions
- Run `go vet` before submitting
- Add tests for new functionality
MD

cat > CHANGELOG.md <<'MD'
# Changelog

All notable changes to Accept MIDAS will be documented in this file.

## [Unreleased]

- Initial repository scaffold
MD

cat > SECURITY.md <<'MD'
# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Accept MIDAS, please report it responsibly by emailing **security@accept.io**.

Do not open a public GitHub issue for security vulnerabilities.
MD

cat > .gitignore <<'TXT'
# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test / coverage
*.test
*.out
coverage.out
coverage.html

# Go
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Environment
.env
.env.local
TXT

cat > LICENSE <<'TXT'
Apache License
Version 2.0, January 2004
http://www.apache.org/licenses/

TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

1. Definitions.

"License" shall mean the terms and conditions for use, reproduction, and
distribution as defined by Sections 1 through 9 of this document.

"Licensor" shall mean the copyright owner or entity authorized by the copyright
owner that is granting the License.

"Legal Entity" shall mean the union of the acting entity and all other entities
that control, are controlled by, or are under common control with that entity.
For the purposes of this definition, "control" means (i) the power, direct or
indirect, to cause the direction or management of such entity, whether by
contract or otherwise, or (ii) ownership of fifty percent (50%) or more of the
outstanding shares, or (iii) beneficial ownership of such entity.

"You" (or "Your") shall mean an individual or Legal Entity exercising
permissions granted by this License.

"Source" form shall mean the preferred form for making modifications, including
but not limited to software source code, documentation source, and configuration
files.

"Object" form shall mean any form resulting from mechanical transformation or
translation of a Source form, including but not limited to compiled object code,
generated documentation, and conversions to other media types.

"Work" shall mean the work of authorship, whether in Source or Object form,
made available under the License, as indicated by a copyright notice that is
included in or attached to the work (an example is provided in the Appendix
below).

"Derivative Works" shall mean any work, whether in Source or Object form, that
is based on (or derived from) the Work and for which the editorial revisions,
annotations, elaborations, or other modifications represent, as a whole, an
original work of authorship. For the purposes of this License, Derivative Works
shall not include works that remain separable from, or merely link (or bind by
name) to the interfaces of, the Work and Derivative Works thereof.

"Contribution" shall mean any work of authorship, including the original version
of the Work and any modifications or additions to that Work or Derivative Works
thereof, that is intentionally submitted to Licensor for inclusion in the Work
by the copyright owner or by an individual or Legal Entity authorized to submit
on behalf of the copyright owner. For the purposes of this definition,
"submitted" means any form of electronic, verbal, or written communication sent
to the Licensor or its representatives, including but not limited to
communication on electronic mailing lists, source code control systems, and
issue tracking systems that are managed by, or on behalf of, the Licensor for
the purpose of discussing and improving the Work, but excluding communication
that is conspicuously marked or otherwise designated in writing by the copyright
owner as "Not a Contribution."

"Contributor" shall mean Licensor and any individual or Legal Entity on behalf
of whom a Contribution has been received by Licensor and subsequently
incorporated within the Work.

2. Grant of Copyright License. Subject to the terms and conditions of this
License, each Contributor hereby grants to You a perpetual, worldwide,
non-exclusive, no-charge, royalty-free, irrevocable copyright license to
reproduce, prepare Derivative Works of, publicly display, publicly perform,
sublicense, and distribute the Work and such Derivative Works in Source or
Object form.

3. Grant of Patent License. Subject to the terms and conditions of this License,
each Contributor hereby grants to You a perpetual, worldwide, non-exclusive,
no-charge, royalty-free, irrevocable (except as stated in this section) patent
license to make, have made, use, offer to sell, sell, import, and otherwise
transfer the Work, where such license applies only to those patent claims
licensable by such Contributor that are necessarily infringed by their
Contribution(s) alone or by combination of their Contribution(s) with the Work
to which such Contribution(s) was submitted. If You institute patent litigation
against any entity (including a cross-claim or counterclaim in a lawsuit)
alleging that the Work or a Contribution incorporated within the Work
constitutes direct or contributory patent infringement, then any patent licenses
granted to You under this License for that Work shall terminate as of the date
such litigation is filed.

4. Redistribution. You may reproduce and distribute copies of the Work or
Derivative Works thereof in any medium, with or without modifications, and in
Source or Object form, provided that You meet the following conditions:

(a) You must give any other recipients of the Work or Derivative Works a copy of
this License; and

(b) You must cause any modified files to carry prominent notices stating that
You changed the files; and

(c) You must retain, in the Source form of any Derivative Works that You
distribute, all copyright, patent, trademark, and attribution notices from the
Source form of the Work, excluding those notices that do not pertain to any part
of the Derivative Works; and

(d) If the Work includes a "NOTICE" text file as part of its distribution, then
any Derivative Works that You distribute must include a readable copy of the
attribution notices contained within such NOTICE file, excluding those notices
that do not pertain to any part of the Derivative Works, in at least one of the
following places: within a NOTICE text file distributed as part of the
Derivative Works; within the Source form or documentation, if provided along
with the Derivative Works; or, within a display generated by the Derivative
Works, if and wherever such third-party notices normally appear. The contents of
the NOTICE file are for informational purposes only and do not modify the
License. You may add Your own attribution notices within Derivative Works that
You distribute, alongside or as an addendum to the NOTICE text from the Work,
provided that such additional attribution notices cannot be construed as
modifying the License.

You may add Your own copyright statement to Your modifications and may provide
additional or different license terms and conditions for use, reproduction, or
distribution of Your modifications, or for any such Derivative Works as a whole,
provided Your use, reproduction, and distribution of the Work otherwise complies
with the conditions stated in this License.

5. Submission of Contributions. Unless You explicitly state otherwise, any
Contribution intentionally submitted for inclusion in the Work by You to the
Licensor shall be under the terms and conditions of this License, without any
additional terms or conditions. Notwithstanding the above, nothing herein shall
supersede or modify the terms of any separate license agreement you may have
executed with Licensor regarding such Contributions.

6. Trademarks. This License does not grant permission to use the trade names,
trademarks, service marks, or product names of the Licensor, except as required
for reasonable and customary use in describing the origin of the Work and
reproducing the content of the NOTICE file.

7. Disclaimer of Warranty. Unless required by applicable law or agreed to in
writing, Licensor provides the Work (and each Contributor provides its
Contributions) on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied, including, without limitation, any warranties
or conditions of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
PARTICULAR PURPOSE. You are solely responsible for determining the
appropriateness of using or redistributing the Work and assume any risks
associated with Your exercise of permissions under this License.

8. Limitation of Liability. In no event and under no legal theory, whether in
tort (including negligence), contract, or otherwise, unless required by
applicable law (such as deliberate and grossly negligent acts) or agreed to in
writing, shall any Contributor be liable to You for damages, including any
direct, indirect, special, incidental, or consequential damages of any
character arising as a result of this License or out of the use or inability to
use the Work (including but not limited to damages for loss of goodwill, work
stoppage, computer failure or malfunction, or any and all other commercial
damages or losses), even if such Contributor has been advised of the
possibility of such damages.

9. Accepting Warranty or Additional Liability. While redistributing the Work or
Derivative Works thereof, You may choose to offer, and charge a fee for,
acceptance of support, warranty, indemnity, or other liability obligations
and/or rights consistent with this License. However, in accepting such
obligations, You may act only on Your own behalf and on Your sole
responsibility, not on behalf of any other Contributor, and only if You agree to
indemnify, defend, and hold each Contributor harmless for any liability incurred
by, or claims asserted against, such Contributor by reason of your accepting any
such warranty or additional liability.

END OF TERMS AND CONDITIONS
TXT

cat > NOTICE <<'TXT'
Accept MIDAS
Copyright 2026 Accept.io

This product includes software developed by Accept.io and contributors.
TXT

echo ""
echo "Running go mod tidy..."
go mod tidy

echo ""
echo "Done. Accept MIDAS (Community) scaffolded."
echo ""
echo "Try:"
echo "  make run"
echo "  curl http://localhost:8080/healthz"
echo "  curl -X POST http://localhost:8080/v1/evaluate -H 'Content-Type: application/json' -d '{\"surface_id\":\"loan-approval\",\"agent_id\":\"lending-model-v3\",\"confidence\":0.87}'"
echo ""
echo "Next steps:"
echo "  git add -A"
echo "  git commit -m 'Initial repository scaffold'"
echo "  git push origin main"