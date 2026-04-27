# team-app

Standalone Go + Next.js application served at `team.multica.uittai.com`. This directory is **independent** of the Multica monorepo at the language and dependency level — separate Go module, separate Postgres database (AR4), separate Next.js app — and the two only coexist in the same git repo for development convenience.

Cross-imports between `team-app/` and `server/` / `apps/` / `packages/` are forbidden in both directions.

## Local development

### Full stack via Docker Compose

```sh
cp .env.example .env                       # then fill in real values
cp frontend/.env.local.example frontend/.env.local
docker compose up                          # api :8080, frontend :3000, nginx :80, db :5432 (internal)
```

After the stack is up:

- `curl http://localhost:8080/healthz` → `200 OK` (Go API).
- `curl http://localhost:3000` → the default Next.js page.
- `curl -H 'Host: team.multica.uittai.com' http://localhost/api/v1/...` → routed through nginx to the API.

The Postgres service uses `pgvector/pgvector:pg17` and creates a database named **`team_app`** (distinct from Multica's database — AR4). The api container reads `DATABASE_URL` from `.env`.

### Backend only (Go)

```sh
cd team-app
go mod tidy
go test -race ./...
go run ./cmd/server     # requires .env exported into the shell
```

Boot validates nine environment variables (see [.env.example](./.env.example)). Any missing or unparseable value logs `missing_env_var=<NAME>` and exits non-zero — no fallback (AC5, AR8). AR8 nominally lists "eight" required vars; this implementation also validates `TEAM_APP_SYSTEM_USER_ID` (presence only — Story 1.9 will additionally assert identity against `GET /api/me`), so the count is **nine**.

### Frontend only (Next.js)

```sh
cd team-app/frontend
pnpm install
pnpm dev                  # http://localhost:3000
pnpm lint                 # enforces the package-boundary rules below
pnpm build
```

`team-app/frontend` has its own `pnpm install` lifecycle and is **not** part of the root `pnpm-workspace.yaml`. The local `.npmrc` (`ignore-workspace=true`) ensures `pnpm install` here does not touch the root lockfile.

## Project layout

```
team-app/
├── cmd/server/                # main.go (boot + env validation), router.go, *_test.go
├── internal/
│   ├── auth/                  # placeholder — populated in Story 1.5
│   ├── events/                # placeholder — populated in Stories 1.7+
│   ├── handler/               # AR13 thin handlers (parse → service → respond)
│   │   └── workspace_work_week.go   # SOLE WRITER for mirror_workspace.work_week (AR14)
│   ├── middleware/            # AR16 middleware chain (Story 1.5+)
│   ├── multica/               # SOLE WRITER for mirror_* tables (AR14)
│   └── service/
│       ├── anomaly/           # NFR42 90% target later
│       ├── approval/          # SOLE WRITER for time_confirm.status, time_confirm_history (AR14)
│       ├── autofill/          # SOLE WRITER for auto time_entry rows via UpsertAutoIssueEntry (AR14)
│       ├── capacity/          # NFR39 100% target later
│       ├── org/
│       └── rrule/             # NFR40 95% target later
├── pkg/db/{queries,generated} # sqlc input/output (Story 1.6 lands queries)
├── migrations/                # 001_init.up.sql lands in Story 1.6
├── scripts/team-app-cli/      # operator CLI — `org bootstrap` lands in Story 8.9
├── frontend/
│   ├── app/                   # Next.js App Router — only place allowed to use next/*
│   ├── packages/
│   │   ├── core/team/         # Headless logic — boundary enforced by ESLint (AR22)
│   │   └── views/team/        # Cross-platform views — boundary enforced by ESLint (AR22)
│   ├── eslint.config.mjs      # no-restricted-imports rules per AR22
│   └── .npmrc                 # ignore-workspace=true (own pnpm lifecycle)
├── nginx/team.conf            # /api/, /gates/, /api/v1/orgs/.+/events SSE, catch-all → frontend
├── docker-compose.yml         # db / api / frontend / nginx
├── Dockerfile                 # Go server image (golang:1.26-alpine → distroless)
├── sqlc.yaml                  # mirrors server/sqlc.yaml (pgx/v5)
└── tools.go                   # //go:build tools — pins Architecture libs in go.mod
```

## Architectural rules (apply to all future code)

1. **Layering (AR13).** Handlers are thin: parse → call service → respond. Handlers MUST NOT import `pkg/db/queries` directly.
2. **Sole writers (AR14).** The `// SOLE WRITER:` comments mark the only places that write the named table or column. Don't add a second writer.
3. **Frontend boundaries (AR22).** Enforced by `frontend/eslint.config.mjs` via `no-restricted-imports`:
   - `packages/core/team/**` — no `next/*`, `react-router-dom`, `react-dom`, UI libraries, `process.env`, `window`, `localStorage`.
   - `packages/views/team/**` — no `next/*`, `react-router-dom`, no direct Zustand store imports.
   - `app/**` — the only place allowed to import `next/navigation`, `next/headers`, server actions, route handlers.
4. **Direct `time.Now()`** per Architecture Enforcement Guidelines #9 — no `clock.Clock` interface; inject `time.Now` via a struct field for tests.
5. **Naming.** DB tables singular `snake_case`. JSON fields `snake_case` everywhere (matches Multica). URL paths plural nouns; path params `camelCase` (`{orgID}`, `{userID}`, `{weekStart}`). Forbidden URL params: `{memberId}`, `{empId}` — always `{userID}`.

## Companion Multica fork PRs

Stories 1.2 / 1.3 / 1.4 land three Multica-side PRs (M-PR#1/#2/#3). They WILL touch `server/migrations/`. **Whoever picks up those stories must determine the next migration prefix dynamically** — at PR-open time, run:

```sh
git log --oneline -- server/migrations/ | head
ls server/migrations/ | sort -n | tail
```

against `origin/main`. **Never assume `057`.** The `feature/issue-tags` branch has already reserved migration prefix `101`, and other in-flight branches may reserve more. The current head migration is whatever the diff against `origin/main` says — pick the next integer above it.

## What this story did NOT do

This is the foundation PR (S-PR#1, Story 1.1). The following are deferred to later stories — do **not** pull them in:

- `001_init.up.sql` — Story 1.6.
- Auth bridge / PAT validation cache / `/api/auth/bridge` — Story 1.5.
- WS subscriber + reconciler — Stories 1.7 / 1.8.
- `GET /api/me` system-PAT identity-mismatch boot validation — Story 1.9.
- Migration runner that applies `*.up.sql` on boot — Story 1.6.
- Any business handler, service, or schema beyond package skeletons + sole-writer comments.
- Multica fork PRs (M-PR#1/#2/#3) — Stories 1.2 / 1.3 / 1.4.
- Production TLS / Let's Encrypt — placeholder `# TODO` in `nginx/team.conf` (AR7).

## CI

The CI workflow is checked in as a **template** at [`ci/team-app-ci.yml.template`](./ci/team-app-ci.yml.template). Both the architect's and the dev's OAuth tokens lack GitHub `workflow` scope, so neither can push under `.github/workflows/`. After this PR merges, a human (or any actor with workflow scope) must copy the template into place:

```sh
cp team-app/ci/team-app-ci.yml.template .github/workflows/team-app-ci.yml
git add .github/workflows/team-app-ci.yml
git commit -m "ci(team-app): add team-app-ci workflow per Story 1.1"
```

Once that lands, three jobs gate every PR that touches `team-app/**`:

- `team-app-backend` — Go 1.26.1 + `pgvector/pgvector:pg17` service → `cd team-app && go test -race ./...`.
- `team-app-frontend` — Node 22 → `pnpm install --frozen-lockfile && pnpm lint && pnpm build`.
- `team-app-migrate-test` — placeholder no-op until Story 1.6 lands the migration runner.
