# Implementation Checklist — Emotional Discovery Portal

## COMPLETED

### Backend (Go)
- [x] Migration `0011_conversation_sessions.sql` — anonymous session table (intent, tradition, region, nakshatra, state, expires_at)
- [x] `internal/ai/client.go` — Hugging Face Inference API client (server-side, HF_TOKEN never exposed)
- [x] `internal/ai/conversation/conversation.go` — conversation state machine, intent handling, empathetic responses
- [x] `/api/v1/conversation` POST route — multi-turn conversation handling
- [x] `/api/v1/conversation` GET route — session state retrieval for results page
- [x] `Discover()` method extracted from `discover()` handler for reuse by conversation layer
- [x] Tests: `go build ./...` and `go test ./...` pass (httpapi + permission)

### Frontend (SvelteKit + Tailwind)
- [x] `apps/web/src/app.css` — nature design tokens (ivory, forest, sage, earth, gold)
- [x] `apps/web/src/routes/+layout.svelte` — shared layout, navigation, canonical link for SEO
- [x] `apps/web/src/routes/+page.svelte` — emotional homepage hero with:
    - Full-viewport hero with floating leaf animations and soft sunlight rays
    - Gentle breathing/pulsing brand mark
    - Responsive grid that scales to device screen
    - Nature-inspired SVG tree illustration
  - 5 emotional pathways (Celebrate, Remember, Explore traditions, Grow, Simply curious)
  - "How Everroot works" 3-step section
  - Trust/values strip
- [x] `apps/web/src/routes/discover/+page.svelte` — conversational discovery flow with typing indicators, suggestion chips, error/loading states
- [x] `apps/web/src/routes/results/[sessionId]/+page.svelte` — results with sources, botanical info, personal reflection
- [x] Vite proxy `/api` → `http://127.0.0.1:8080` verified working
- [x] SEO: meta description, Open Graph (og:title, og:description, og:image, og:type, og:url), Twitter cards, JSON-LD WebPage structured data, canonical link

### Database
- [x] Applied `0011_conversation_sessions.sql` migration
- [x] Session TTL (24h), guest-only, no birth data, minimal fields

### Environment
- [x] `.env` — HF_TOKEN, HF_MODEL_ID (server-side only, gitignored)
- [x] `.env.example` — HF_MODEL_ID placeholder, no secrets

### Verification
- [x] `go build ./...` passes
- [x] `go test ./...` passes
- [x] `svelte-check` passes (0 errors, 0 warnings)
- [x] `vite build` passes
- [x] Services running: API on 127.0.0.1:8080, Frontend on http://127.0.0.1:5173/
- [x] Endpoints respond: /healthz, /readyz, /api/v1/traditions, /api/v1/nakshatras, /api/v1/conversation
- [x] End-to-end conversation flow tested via curl

## REMAINING LIMITATIONS
- OpenAPI contract (`api/openapi.yaml`) not yet updated with conversation endpoint
- No real cultural mappings seeded in the database → discovery returns `no_verified_mapping` (by design per Phase 1 scope)
- Hugging Face inference not tested with real token (network-dependent); unit tests mock the AI client
- robots.txt and sitemap.xml not yet added (could be added to static/)