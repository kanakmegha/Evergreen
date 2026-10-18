# Everroot

Everroot is an evidence-led tree discovery portal. This Phase 1 vertical slice supports anonymous discovery by a user-provided Nakshatra, a source/evidence registry, immutable cultural mapping revisions, independent botanical candidates, and a local-only review workflow. It ships no real cultural mapping or planting data.

## Prerequisites

- Go 1.24 or later
- Node.js 22 and npm
- Docker Desktop with Compose
- PostgreSQL client tools (`psql`)
- `sqlc` 1.29.0 for query generation (or run it through `go run` below)

## Local configuration

Copy `.env.example` to `.env`. It contains local connection and reviewer settings, no credentials. Never put production credentials in source files or commit local credentials.

```sh
cp .env.example .env
docker compose -p everroot-phase1 up -d postgres
```

The disposable local database publishes only on `127.0.0.1:54330` and uses trust authentication for local development, so there is no committed password. Never reuse this configuration outside the machine. To load the local configuration in a shell:

```sh
set -a
. ./.env
set +a
```

`ADMIN_LOCAL_ENABLED=true` enables internal review operations only when the API sees a loopback client and has a configured reviewer identity and role. The API defaults to binding at `127.0.0.1:8080`. Do not expose internal routes to a non-local network; no production admin authentication is implemented in this phase. To disable review routes, set `ADMIN_LOCAL_ENABLED=false`.

## Database migrations

Migrations are ordered SQL files and `schema_migrations` records which files have been applied. The script applies each migration and records it in one PostgreSQL transaction.

```sh
set -a; . ./.env; set +a
./scripts/migrate.sh
```

To confirm a fresh database, use a new local database/container volume, set `DATABASE_URL` to it, and run the same script. Migration `0002` contains only the 27-value Nakshatra input vocabulary; `0004` contains tradition and region selectors. Neither inserts cultural mappings or tree claims.

## Query code generation

SQL migrations are the schema source of truth. Generate the Go query package using the pinned generator:

```sh
GOCACHE=/private/tmp/everroot-go-cache go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
GOCACHE=/private/tmp/everroot-go-cache go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 vet
GOCACHE=/private/tmp/everroot-go-cache go test ./internal/dbgen
```

`sqlc.yaml` generates pgx/v5 types under `internal/dbgen`. Query files are parameterized SQL. API handlers currently use small, explicit pgx queries for their transactional workflow; the generated package is available for typed query reuse as the registry grows.

## Tests

Unit and handler validation tests:

```sh
GOCACHE=/private/tmp/everroot-go-cache go test ./...
```

Database workflow tests run against a dedicated, migrated test database. Create it once and apply migrations before testing. Do not point `TEST_DATABASE_URL` at a database containing user data; integration tests insert synthetic-only records and leave them in place.

```sh
docker compose -p everroot-phase1 exec -T postgres createdb -U everroot everroot_test
DATABASE_URL='postgres://everroot@127.0.0.1:54330/everroot_test?sslmode=disable' ./scripts/migrate.sh
TEST_DATABASE_URL='postgres://everroot@127.0.0.1:54330/everroot_test?sslmode=disable' \
GOCACHE=/private/tmp/everroot-go-cache go test ./internal/httpapi -run TestPostgresDiscoveryAndReviewWorkflow -count=1
```

The integration test covers source verification, evidence registration, the publication gate, loopback authorization, cultural versus botanical reviewer separation, conflicting mappings, anonymous discovery, botanical uncertainty, and revision immutability.

Frontend checks:

```sh
cd apps/web
npm install --ignore-scripts
node node_modules/@sveltejs/kit/svelte-kit.js sync
node node_modules/svelte-check/bin/svelte-check --tsconfig ./tsconfig.json
node node_modules/vite/bin/vite.js build
```

## Run the complete local slice

Terminal 1 — start PostgreSQL and apply migrations:

```sh
docker compose -p everroot-phase1 up -d postgres
set -a; . ./.env; set +a
./scripts/migrate.sh
```

Terminal 2 — start the Go API:

```sh
set -a; . ./.env; set +a
GOCACHE=/private/tmp/everroot-go-cache go run ./cmd/api
```

Terminal 3 — start the SvelteKit interface:

```sh
cd apps/web
npm install --ignore-scripts
node node_modules/@sveltejs/kit/svelte-kit.js sync
node node_modules/vite/bin/vite.js --host 127.0.0.1
```

Open `http://127.0.0.1:5173`. The public page explains that no verified mappings are currently available until reviewed records are entered. The local review interface is at `/review`.

## Local review sequence

1. Register a source with a URL or bibliographic reference, access date, attribution, and reuse status.
2. Add an evidence locator. Leave passage text blank unless reuse rights permit storing it.
3. Use the local source verification action. `citation_only` can be cited with a locator; copied passage text requires `permitted` rights. Revoking a source's verification automatically withdraws currently published mappings that rely on it.
4. Submit a cultural mapping revision with a valid tradition, region if applicable, Nakshatra, original claim, and evidence IDs. Botanical candidates are optional; each candidate requires its own evidence, certainty, and confidence.
5. Review the exact revision. A later submission uses its mapping ID and creates a new revision, leaving earlier content intact.
6. Publish that exact approved revision. PostgreSQL and the API enforce source verification, rights, evidence, and latest review decision.
7. Botanical review is separate. For a local botanical reviewer process, stop the API and restart it with `ADMIN_REVIEWER_ROLE=botanical` and a reviewer ID. Use `admin` only for a trusted local all-role reviewer.

All claims or names entered through this interface are editorial inputs and need qualified human review. Do not use the local UI to create invented content for public release.

## API contract

The OpenAPI contract is in [`api/openapi.yaml`](api/openapi.yaml). Public endpoints provide traditions, the Nakshatra vocabulary, and source-backed discovery. Internal endpoints are local-only and protected by Go middleware. Health and readiness endpoints are `/healthz` and `/readyz`.

### Answering a question

`POST /api/v1/questions` (aliased as `/api/v1/ask`) answers an open question. The request path is:

1. The browser posts a question to the Go API.
2. The API validates the input and builds the prompt (`internal/ai/qa.go`).
3. The API calls the configured Hugging Face model using `HF_TOKEN`, `HF_MODEL_ID` and `HF_BASE_URL`.
4. The API returns the model's answer to the browser.
5. Separately, the interaction is written to PostgreSQL when it is reachable.

PostgreSQL is **not** on the answer path. A database outage never prevents an otherwise successful model call from answering, and a failed write never fails the response. No table is consulted to produce an answer, and there is no fallback to stored content.

The question prompt carries the same rule as the rest of Everroot: the model must never state or imply that a tree is associated with a nakshatra, religion, community, region, deity, festival or scripture. Cultural claims are shown only when a reviewed source backs them.

Feedback is posted to `POST /api/v1/feedback` and is independent of answer generation.

## Deploying

Three values must be supplied by the deployment. Nothing in the repository invents a production host or a secret.

| Variable | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | For persistence only | PostgreSQL connection for interactions and feedback. The API starts, and answers questions, without it. |
| `HF_TOKEN` | To answer questions | Hugging Face credentials. Never commit it, and never expose it to the browser. |
| `PUBLIC_SITE_URL` | Before launch | The public origin, for example `https://example.org`. Used for canonical links, `robots.txt` and `sitemap.xml`. |

### The production domain is not set anywhere

`PUBLIC_SITE_URL` is the only place a public origin is configured. When it is unset, canonical links, `robots.txt` and `sitemap.xml` are generated from the origin each request arrived on, so they always match the domain actually serving the site — a platform subdomain, a preview URL, or a real domain. Set it when the canonical origin must differ from the serving host, for example so a platform subdomain is not indexed as the canonical address.

### Static asset still required

`apps/web/src/lib/site.ts` references `/og-hero.png` for social previews. That image is not in the repository yet; add a 1200×630 image at `apps/web/static/og-hero.png`.

### Health and readiness

`/healthz` reports that the process is up and does not touch the database. `/readyz` reports database readiness and returns 503 when PostgreSQL is unreachable, so a platform health check can tell "the API is running" apart from "persistence is available".

### Cross-origin access

Cross-origin access is opt-in through `WEB_ORIGIN`, and the normal deployment does not need it: the web app calls the API on its own origin, with Vite proxying `/api` in development and `vercel.json` rewriting `/api` in production. Leaving `WEB_ORIGIN` unset grants no cross-origin access.

If the API must be served from a different origin, set `PUBLIC_API_URL` on the web app to that origin **and** `WEB_ORIGIN` on the API to the web app's origin. Both are required: without `WEB_ORIGIN` the browser's cross-origin request is refused.

`PUBLIC_`-prefixed variables are inlined into the browser bundle by design. Never put a secret behind that prefix.

## Current limitations

- No real cultural sources or mappings have been imported; only catalog selectors exist. The public cultural path therefore has no data behind it yet.
- No external identity provider is integrated. The review workflow is for trusted localhost development only.
- No birth-date/time calculations, accounts, save/share, planting partners, analytics, or marketing are included.
- `POST /api/v1/questions` answers from general model knowledge. It is deliberately not treated as evidence, and nothing it returns is stored as a reviewed record.
- Interaction and feedback retention is unmanaged: `user_interactions` and `user_feedback` have no automatic expiry. Add a retention and deletion policy before collecting real user questions.
- Source metadata is entered and reviewed, but external link rot and licence interpretation still require human follow-up.
- API throttling, production secrets, backup policy, and external security review remain future work.
