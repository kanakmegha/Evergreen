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

### Suggesting trees

`POST /api/v1/recommendations` is the AI-first half of discovery: it draws tree suggestions for one visitor's answers, whatever intention they chose. The request path is the same shape as the question path — the browser posts the selections and the words the visitor wrote, the API validates them, builds the prompt (`internal/ai/recommend.go`), calls the configured model, then validates the reply before returning it. A candidate without a name or without a reason is dropped rather than repaired, and a reply Everroot cannot read is reported as **502**, never as an empty list.

Two properties are deliberate:

- **It consults neither the database nor the botanical catalogue.** A suggestion arrives even when the catalogue is 503 or empty, which is what makes discovery independent of a migration. The only part that touches reviewed data is `evidence.culturalAssociation`, and it goes through the same publication gate as `/api/v1/discoveries` and `/api/v1/explain`.
- **It is separate from `POST /api/v1/discoveries`.** That endpoint answers "is there a reviewed cultural association for this selection?" and can only ever return what a reviewer approved. This one answers "what else might fit?", and is labelled as machine-written wherever it appears. Neither can borrow the other's authority.

Failures are reported as failures: **503** when the AI layer is not configured or unreachable, **429** (with `Retry-After`) when the model is busy, **502** when the reply cannot be read. A 200 with an empty list means one thing only — the model found nothing it would stand behind — and the interface says exactly that instead of showing a blank shortlist.

The visitor's own words do travel in this request, because a suggestion cannot be drawn from closed vocabularies alone. They are used to build one prompt and are never written to PostgreSQL or to the log; the follow-up step says so at the field where the note is written.

Three optional things may come back with the suggestions, and each one is bounded by a rule rather than by trust:

- **`matureSize`** is the typical mature height the model offers alongside its growing requirements and limitations. Where it is not sure, it writes "not sure" rather than inventing a figure.
- **`followUpQuestion`** is present only when one missing fact would materially change the recommendations. It is omitted when absent, and it never stands in for an answer: the suggestions arrive with it, in the same reply, so asking costs nothing extra and delays nothing.
- **`culturalNote`** is the one place the model may mention cultural associations at all, and it may only restate a verdict it did not choose. The server computes `evidence.culturalAssociation` from the same publication gate as every other endpoint and hands the model exactly one value of it — `no_reviewed_match`, the case where a visitor asked about a tradition and Nakshatra and Everroot has no reviewed record to answer with. In that case the model writes one plain sentence saying so; otherwise it is told to say nothing about cultural associations, not even to deny one. The note describes the record, never the tree: no tree is or is not associated with a nakshatra because a suggestion was drawn. A verified match, and a selection nobody made, are stated by the server and never by the model.

The results page renders the follow-up question as a question, not as a placeholder for a second visit: "Change my answers" is how a visitor answers it, and there is no second model call.

The results page asks for suggestions first and falls back to the explanation call only when none arrive, so a results view costs at most one model call either way.

## Deploying

The project deploys as two Vercel projects from this one repository:

| Project | Root directory | What it runs |
| --- | --- | --- |
| Backend | repository root | the Go API, as a Vercel **Service** (`framework: "go"`) running the long-running server in `cmd/api` |
| Frontend | `apps/web` | the SvelteKit web app |

The backend is a listening HTTP server, not a serverless function. It reads the port to bind from `PORT`, which the platform assigns and then probes; if nothing listens there in time the deployment is declared failed. `API_ADDR` is a local-development convenience and is ignored whenever `PORT` is set. The server binds its listener before it touches the database, so a slow or unreachable PostgreSQL can never stop it from starting.

### Environment variables

| Variable | Project | Required | Purpose |
| --- | --- | --- | --- |
| `DATABASE_URL` | Backend | Yes, for the reviewed catalogue | PostgreSQL connection (Neon: use the **pooled** endpoint with `sslmode=require`). The nakshatra, tradition, region and tree catalogue and interaction persistence all read it. **The AI answer path does not**: `/api/v1/questions` answers from the model alone. |
| `DIRECT_URL` | — | No | The **direct** (unpooled) endpoint. Used only by `cmd/migrate`, never by the server, because the migration runner holds a session-level advisory lock. |
| `DATABASE_CONNECT_TIMEOUT` | Backend | No | Bound on one connection attempt (default `10s`). Raise it if Neon cold starts exceed the default. |
| `HF_TOKEN` | Backend | To answer questions | Hugging Face credentials. Never commit it, and never expose it to the browser. |
| `HF_MODEL_ID`, `HF_BASE_URL` | Backend | No | Optional provider overrides. Leave `HF_MODEL_ID` unset to use the default, un-suffixed identifier; a provider or policy suffix such as `:cheapest` pins one provider and gives up the router's automatic failover. |
| `WEB_ORIGIN` | Backend | Only for a split origin | Comma-separated list of web origins allowed to call this API. See below. |
| `PUBLIC_API_URL` | Frontend | For a split origin | The backend origin, for example `https://everroot-api.example`. Read through SvelteKit's `$env/static/public` and inlined at **build** time. A production build without it (and without the opt-in below) refuses to send catalogue requests and reports a configuration error. |
| `PUBLIC_API_SAME_ORIGIN` | Frontend | Only for a single-origin deployment | Set to `true` when the API really is served from the same origin as the web app. This is automatic under `vite dev`; a production build needs this opt-in before it will send same-origin requests with no `PUBLIC_API_URL`. |
| `PUBLIC_SITE_URL` | Frontend | Before launch | The public origin used for canonical links, `robots.txt` and `sitemap.xml`. |
| `API_ADDR` | — | No | Local development only. Do not set it on a platform that provides `PORT`. |

### If the catalogue returns 404 in the browser

A **404** and a **503** are different failures, and the interface says which one happened.

A 404 on `/api/v1/…` means the request went to the *web app's* own origin — for example `https://<web-project>.vercel.app/api/v1/nakshatras` — and was answered by SvelteKit's catch-all HTML page. That origin serves no API: the frontend and the backend are two Vercel projects here, and the frontend project's root is `apps/web`, so it never sees the root `vercel.json` rewrites.

`PUBLIC_API_URL` is what prevents that. It is read in `apps/web/src/lib/apiEnv.ts` through SvelteKit's `$env/static/public`, and inlined into the client bundle at build time. Two consequences matter:

- **The variable has to exist when the frontend is built.** Changing it later, or setting it on the wrong project or environment, does not change a bundle that is already deployed; the frontend must be redeployed.
- **A production build with no address configured no longer sends a same-origin request.** `getJSON()` in `apps/web/src/lib/discovery.ts` refuses before calling `fetch`, and the interface names `PUBLIC_API_URL`. Set it to the backend origin, or set `PUBLIC_API_SAME_ORIGIN=true` if the API truly shares this origin.

Earlier revisions read `import.meta.env.PUBLIC_API_URL` instead. That resolved only while `vite.config.ts` carried a hand-written `envPrefix: ['VITE_', 'PUBLIC_']`; SvelteKit's own types note that Vite's `envPrefix` must be set separately for Vite's environment handling and that using that feature is generally unnecessary. Remove or reorder the option and the origin silently became `undefined`, the app fell back to same-origin requests, and the problem showed up only as HTML 404s in a browser. The `$env/static/public` read does not depend on `envPrefix`, and the option has been removed.

To verify a deployed build without changing anything:

```sh
node -e '(async () => {
  const base = process.argv[1];
  const seen = new Set(), queue = [];
  const add = (ref, from) => { try { const u = new URL(ref, from); if (u.origin === base) queue.push(u.href); } catch {} };
  const html = await (await fetch(base + "/discover")).text();
  for (const m of html.matchAll(/\/_app\/immutable\/[A-Za-z0-9._\/-]+\.js/g)) add(m[0], base + "/");
  while (queue.length && seen.size < 80) {
    const url = queue.shift();
    if (seen.has(url)) continue;
    seen.add(url);
    const text = await (await fetch(url)).text();
    if (text.includes("evergreen-backend-eight")) return console.log("origin present in", url);
    for (const m of text.matchAll(/["\x27`(]([A-Za-z0-9._\/-]+\.js)["\x27`)]/g)) add(m[1], url);
  }
  console.log("ORIGIN ABSENT from the client bundle");
})()' https://<web-project>.vercel.app
```

Then in DevTools → Network, reload `/discover` and walk to the tradition step: the three catalogue requests must show the backend hostname. If they do, the routing is correct and the only remaining failure is the 503 below — the API is reached, but the reviewed records cannot be served. That is a database state, not a routing one.

### If the reviewed catalogue returns 503

`/api/v1/nakshatras`, `/api/v1/traditions`, `/api/v1/traditions/{tradition}/regions` and `/api/v1/trees` describe records a person checked, so when the database cannot be reached they report **503 `the reviewed catalogue is unavailable right now`**. They deliberately do not answer from a built-in list: a hardcoded reply is indistinguishable from a reviewed record and would imply Everroot had checked something it had not.

A 503 is a database problem, not a code problem, but "the database" covers several failures that need different fixes. Each failure is logged with a `category` and, when PostgreSQL itself answered, a `sqlstate`. The driver's own error string is never logged: for a connection failure it embeds the host and user from `DATABASE_URL`.

| `category` | Meaning | Fix |
| --- | --- | --- |
| `not_configured` | `DATABASE_URL` is unset | Set it on the backend project |
| `malformed_dsn` | The connection string cannot be parsed | Re-copy it from the Neon dashboard |
| `dns` | The hostname does not resolve | The endpoint is wrong, or the project or branch was deleted |
| `connection_refused` | The host exists but is not accepting connections | Check the port; the Neon compute may be suspended or disabled |
| `timeout` | The attempt ran out of time (often a Neon cold start) | Retry; raise `DATABASE_CONNECT_TIMEOUT` if it recurs |
| `tls` | TLS negotiation failed | Neon requires TLS: keep `sslmode=require` on `DATABASE_URL` |
| `authentication` (`28…`) | The role or password was rejected | Re-copy the connection string; the password may have been rotated |
| `unknown_database` (`3D000`) | The named database does not exist on this branch | Re-copy the connection string |
| `permission_denied` (`42501`) | The role lacks a privilege | Check the role in Neon |
| `schema_missing` (`42P01`, or a reachable database with no tables) | The connection works but migrations were never applied | Run the migrations, below |
| `server_error` | PostgreSQL rejected the connection | The `sqlstate` in the log identifies the cause |

Connection and schema are different failures. `Ping` succeeds against a reachable, authenticated database even when it is empty, so a missing schema never appears as "the database is unreachable". The backend logs the target (host, port, database, user, and whether a password is set — never the password itself) and the classification.

`/healthz` returns 200 whenever the process is up; `/readyz` returns 503 when PostgreSQL cannot be reached at all. That separates "the API is running" from "the database answers".

### If an AI question returns 429 or 503

`/api/v1/questions` and `/api/v1/explain` answer from the model alone: they never read PostgreSQL, so a database outage never turns an AI answer into a failure, and a failed interaction save is logged and ignored.

A provider failure is classified like a database failure, and only the classification is logged — never the token, the prompt, or the provider's response body:

| `category` | HTTP | Meaning | Fix |
| --- | --- | --- | --- |
| `not_configured` | 503 | `HF_TOKEN` is unset | Set it on the backend project |
| `auth` | 503 | The provider rejected the credentials | Re-issue `HF_TOKEN` with the *Inference Providers* permission |
| `credits` | 503 | The account cannot pay for the selected provider | Enable billing, or pick a different provider |
| `rate_limited` | 429 | The provider is busy or throttling (for example `provider_code=engine_overloaded`) | Nothing: the request is retried, and a `Retry-After` header is returned |
| `provider_error` | 503 | A 5xx from the provider, or a transport failure | Retry; the log records the status |
| `timeout` | 503 | No answer within the per-attempt deadline | Retry |
| `no_completion`, `bad_response` | 503 | The provider replied without any usable text | Retry; if it recurs the model may not be a chat model |

Transient failures (an overloaded model, a 5xx, a timeout) are retried up to three times. Each retry drops any routing suffix from the model so the router can fail over to a different provider, which is why a pinned `:cheapest` value still recovers. Authentication and billing failures are never retried: waiting cannot fix them.

A model identifier carrying a provider or policy suffix (`openai/gpt-oss-120b:cheapest`) pins inference to a single provider and gives up the router's automatic failover, so that provider's overload becomes a 429 for every visitor. The default, un-suffixed identifier is the more reliable choice.

The server binds its listener and starts answering **before** it tests the database, and it keeps the pool open when that first test fails. pgxpool connects lazily, so a Neon compute that is still waking serves the next request normally; discarding the pool on the first failed ping would turn a temporary outage into a permanent one.

### The bounded connection test

`cmd/dbcheck` runs a read-only, bounded connection test against the same variable the server reads, and prints the same classification. It writes nothing and never prints the connection string:

```sh
go run ./cmd/dbcheck              # tests DATABASE_URL, the application target
go run ./cmd/dbcheck DIRECT_URL   # tests the unpooled migration target
```

Run it in the same environment the deployment uses. It reports two things separately:

- `reachable=false` — a connection problem; `category` and `next step` say which.
- `reachable=true schema_present=false` — authentication works, but the database has never been migrated.

### Applying migrations

The schema is not created automatically. Applying migrations is a deliberate, manual step against the target database and is never run by the API on start-up. Migrations run over the **direct** endpoint, because the pooled endpoint cannot hold the session-level advisory lock the runner takes:

```sh
DIRECT_URL='postgres://…direct…' go run ./cmd/migrate
```

Use the pooled endpoint for `DATABASE_URL` (the server) and the direct endpoint for `DIRECT_URL` (migrations). Both need `sslmode=require` for Neon.

### Split origins (two projects, two domains)

When the frontend and backend are on different domains, set `PUBLIC_API_URL` on the frontend to the backend origin **and** `WEB_ORIGIN` on the backend to the frontend origin. Both are required: without `WEB_ORIGIN` the browser's cross-origin request is refused.

`WEB_ORIGIN` is an exact, comma-separated allow-list. Cross-origin access is opt-in and there is no wildcard — this API spends a paid model call on `/api/v1/questions`, so reflecting an arbitrary `Origin` would let any site spend someone else's quota. A missing `WEB_ORIGIN` grants nothing, and preflight requests are answered only for listed origins.

`PUBLIC_API_URL` is read through SvelteKit's `$env/static/public` and inlined into the browser bundle at build time, so changing it requires a new frontend deployment; it cannot be changed at runtime. Set it for every environment that should work: a Preview build without it reports a configuration error rather than calling the preview origin.

### Cross-origin access on one origin

If both are served from one domain, set `PUBLIC_API_SAME_ORIGIN=true` on the frontend and leave `WEB_ORIGIN` unset. Requests stay relative: Vite proxies `/api` during development (where the opt-in is automatic), and on Vercel the root `vercel.json` routes `/api/(.*)`, `/internal/(.*)`, `/healthz` and `/readyz` to the API service. The opt-in is explicit because a missing `PUBLIC_API_URL` used to fall back to same-origin requests silently, which is indistinguishable, in the browser, from a correct deployment.

`PUBLIC_`-prefixed variables are inlined into the browser bundle by design. Never put a secret behind that prefix.

### Deployment order

1. Apply migrations to the database (manual, once per schema change). Verify with `go run ./cmd/dbcheck DIRECT_URL` — it must report `reachable=true schema_present=true`.
2. Deploy the backend, with `DATABASE_URL`, `HF_TOKEN` and (for split origins) `WEB_ORIGIN` set. Remove any `API_ADDR` left over from local development.
3. Verify the backend directly: `/healthz` should be 200, `/readyz` 200, and `/api/v1/nakshatras` should be 200 with 27 entries (not 503, not 500). If a catalogue endpoint returns 503, the log line names the `category`; see *If the reviewed catalogue returns 503* above.
4. Set `PUBLIC_API_URL` on the frontend if the origins differ, or `PUBLIC_API_SAME_ORIGIN=true` for a production single-origin deployment.
5. Deploy the frontend last, so the build picks up `PUBLIC_API_URL`.


## Current limitations

- No real cultural sources or mappings have been imported; only catalog selectors exist. The public cultural path therefore has no data behind it yet.
- No external identity provider is integrated. The review workflow is for trusted localhost development only.
- No birth-date/time calculations, accounts, save/share, planting partners, analytics, or marketing are included.
- `POST /api/v1/questions` answers from general model knowledge, and `POST /api/v1/recommendations` draws its suggestions the same way. Neither is treated as evidence: nothing either returns is stored as a reviewed record, and a suggestion never states a cultural association. Suggestions are also not checked against the authority catalogue, so a suggested name is the model's own until someone opens the tree reference.
- Interaction and feedback retention is unmanaged: `user_interactions` and `user_feedback` have no automatic expiry. Add a retention and deletion policy before collecting real user questions.
- Source metadata is entered and reviewed, but external link rot and licence interpretation still require human follow-up.
- API throttling, production secrets, backup policy, and external security review remain future work.
