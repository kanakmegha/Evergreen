# Everroot Phase 0: Repository Review and Architecture

Date: 2026-10-15  
Status: Phase 1 approved; see implementation details below

## Repository assessment

The requested workspace is empty: it contains no source files, project metadata, Git repository, or existing application to preserve. There are no repository-specific conventions or constraints to inherit. This document captures the proposed baseline before scaffolding.

## Product boundary for the first release

Start with a narrow, reviewable subset of Hindu Nakshatra mappings. Tamil and North Indian variants must be represented as distinct tradition/locale contexts where evidence supports differences. Only reviewed, published rules with attached source evidence may produce a culturally attributed recommendation. Unknown, disputed, or unverified content should return no definitive mapping and explain that the evidence is not ready.

The first vertical slice should let an anonymous visitor choose a tradition and a known Nakshatra, then receive a deterministic result showing names, botanical identity, region/tradition, review status, and source evidence. Planting partners and LLM explanations follow only after their own verified data and controls exist. No illustrative mapping or partner offer should appear as real.

## Proposed directory structure

```text
.
├── apps/
│   └── web/                 # SvelteKit + TypeScript public site and admin UI
├── cmd/
│   └── api/                 # Go HTTP API entry point
├── internal/
│   ├── config/              # Environment configuration and validation
│   ├── httpapi/             # Routes, middleware, request/response DTOs
│   ├── discovery/           # Deterministic intent validation and rule matching
│   ├── registry/            # Cultures, traditions, species, sources, review workflow
│   ├── partners/            # Partner availability and referrals
│   ├── privacy/             # Consent, retention, deletion
│   └── observability/       # Structured logging, request IDs, health/readiness
├── db/
│   ├── migrations/          # Versioned PostgreSQL SQL; source of schema truth
│   ├── queries/             # Query source if using sqlc
│   └── seeds/               # Explicitly sourced, reviewable import fixtures
├── api/openapi.yaml         # REST contract
├── docs/
│   ├── architecture/        # ADRs, schema diagram, deployment notes
│   ├── operations/          # Local setup, backups, production deployment
│   └── content/             # Evidence and seed import guidance
├── deploy/                  # Docker/production deployment configuration
├── scripts/                 # Repeatable development and data tasks
├── go.mod
├── compose.yaml
└── README.md
```

Keep the Go service as a modular monolith. The SvelteKit application should render public source-backed pages server-side for crawlability and call the REST API for discovery and administrative actions. Do not add Redis, vector search, or a separate service until measured requirements justify them.

## Architecture decision record

### ADR-001: modular monolith with relational cultural registry

**Decision.** Use SvelteKit/TypeScript for the web application, Go for a single modular API, PostgreSQL for the authoritative registry and application data, versioned SQL migrations, and REST documented in OpenAPI. Keep discovery deterministic in application code. An optional provider-independent LLM may later explain a result from a constrained evidence bundle, but it cannot choose, create, or alter mappings.

**Data access.** Prefer `sqlc` with explicit SQL queries and generated Go types, with PostgreSQL migrations kept as the schema source of truth. This retains SQL visibility, compile-time query/type checking, and avoids ORM lifecycle/migration coupling in a provenance-heavy schema. Reconsider only if query maintenance or development speed demonstrates a concrete need. Use parameterized queries throughout.

**Sessions and privacy.** Anonymous discovery is the default. Use opaque, expiring server-side session identifiers only when multi-turn state requires them; do not persist raw chat transcripts by default. Keep consent and referral contact data separate from cultural rules. Store only minimized event properties.

**Evidence publication.** A mapping is eligible for public discovery only if it is published, has an accepted review state, and links to at least one source evidence passage. Conflicts remain separate rules keyed to their tradition/region/version. Querying must return alternatives and evidence status rather than silently selecting a culturally universal answer.

**Planting.** Partner records, regional availability, offers, and verification dates are independent of cultural mappings. No real offer is shown until the partner and offer are verified. Referral contact information requires explicit purpose-specific consent; outbound clicks alone never count as a confirmed planting.

**Trade-offs.** This design favors auditability and straightforward operations over rapid schema abstraction or independent service scaling. PostgreSQL and explicit SQL require deliberate schema/query work, but support the required constraints and provenance. The modular monolith can be split later if workload boundaries justify it.

## Initial PostgreSQL schema proposal

All primary keys should be stable UUIDs. Use `timestamptz` for audit timestamps; scientific names remain searchable attributes and never serve as identifiers. Add created/updated timestamps and appropriate indexes consistently.

### Knowledge registry

- `cultures(id, slug UNIQUE, display_name, description, status)`
- `traditions(id, culture_id FK, slug, display_name, description, status, UNIQUE(culture_id, slug))`
- `tradition_locales(id, tradition_id FK, locale, region_code, region_name, variant_label, status, UNIQUE(tradition_id, locale, region_code, variant_label))`
- `sources(id, title, source_type, publisher, author, publication_date, url, language, access_date, license, attribution_requirements, status)`
- `source_evidence(id, source_id FK, locator, original_text, language, translation, translation_note, evidence_notes)`; retain exact page/section/line locator when available.
- `tree_species(id, scientific_name, family, taxonomy_source_id FK NULL, status, UNIQUE(scientific_name))`
- `tree_names(id, species_id FK, name, language, region_code, name_type, source_evidence_id FK NULL)`; index normalized name/language/region, avoid global uniqueness of common names.
- `mapping_rules(id, tradition_id FK, locale_id FK NULL, rule_type, matching_mode, parameters JSONB, species_id FK, status, review_status, evidence_class, version, valid_from, valid_until, created_by, published_at)`; constrain status/review values and JSONB to variable rule parameters only.
- `mapping_conditions(id, rule_id FK, input_type, operator, value_text, value_json JSONB NULL, ordinal)` for queryable normalized inputs where possible; validate operator/input combinations.
- `mapping_evidence(id, rule_id FK, source_evidence_id FK, relationship, UNIQUE(rule_id, source_evidence_id, relationship))`
- `rule_reviews(id, rule_id FK, reviewer_id, decision, notes, reviewed_at, rule_version)`
- `rule_relations(id, rule_id FK, related_rule_id FK, relation_type)` for conflicts, alternatives, supersession.

### Discovery, privacy, and operations

- `discovery_sessions(id, created_at, expires_at, locale, state JSONB)`; state only for minimum active flow, with TTL cleanup.
- `saved_discoveries(id, owner_id FK, rule_id FK, created_at, share_token_hash NULL)`; implement only when account/save flows are introduced.
- `users(id, email NULL UNIQUE, auth_subject NULL UNIQUE, created_at, deleted_at)`; omit accounts from the initial anonymous discovery slice.
- `privacy_consents(id, subject_id, purpose, status, policy_version, captured_at, withdrawn_at, evidence JSONB)`; avoid mixing marketing and partner referral purposes.
- `analytics_events(id, event_type, occurred_at, anonymous_session_id NULL, minimal_properties JSONB)`; enforce property allowlists at API boundary and retention cleanup.
- `audit_events(id, actor_id NULL, action, entity_type, entity_id, occurred_at, request_id, change_summary JSONB)`; never record secrets or unnecessary personal text.

### Planting partners (later phase)

- `partners(id, name, verification_status, verified_at, website, terms_url, evidence_capabilities, contact_reference, status)`
- `partner_locations(id, partner_id FK, region_code, locality, postal_prefix NULL, status)`
- `partner_species(id, partner_id FK, species_id FK, location_id FK NULL, availability_status, verified_at, UNIQUE(partner_id, species_id, location_id))`
- `planting_offers(id, partner_id FK, species_id FK, location_id FK NULL, price_amount NUMERIC NULL, currency CHAR(3) NULL, terms, maintenance_terms, replacement_terms, valid_from, valid_until, verified_at, status)`; price/currency nullability must be constrained together.
- `referral_events(id, referral_id UNIQUE, partner_id FK, offer_id FK NULL, anonymous_session_id NULL, consent_id FK NULL, event_type, occurred_at, minimal_metadata JSONB)`
- `planting_outcomes(id, partner_id FK, referral_event_id FK, status, evidence_reference, confirmed_at, verified_by)`

### Integrity and publication rules

Use restrictive foreign keys for referenced evidence and species so published history cannot be accidentally orphaned; use explicit archival/status transitions instead of cascading deletes for registry records. A publication transaction or service validation must ensure a published rule has accepted review and at least one evidence link. PostgreSQL check constraints handle local invariants; cross-table publication requirements need a controlled transaction/trigger and application validation. Add indexes for tradition/locale, rule type/status, species, normalized tree names, evidence source, and discovery lookup dimensions. Use JSONB for variable conditions or minimal event properties only, not entity relationships.

## Phased task checklist

- [ ] **Phase 0 — review (current):** repository inventory; architecture decision; proposed directory and schema; agree initial supported tradition subset and evidence publication bar.
- [ ] **Phase 1 — foundation:** scaffold SvelteKit and Go modules; Docker Compose/PostgreSQL; SQL migrations; config validation; request IDs, structured logs, health/readiness; CI and baseline tests.
- [ ] **Phase 2 — registry:** normalized registry tables; source/evidence import format; admin review and audit workflow; seed only documented evidence; conflict and publication enforcement.
- [ ] **Phase 3 — deterministic discovery:** typed REST/OpenAPI contract; input validation; matching service and tests; source-backed results and alternatives; anonymous flow.
- [ ] **Phase 4 — conversation:** one-question-at-a-time UI; no culture inference; optional LLM explanation adapter constrained to retrieved evidence; provider failure fallback.
- [ ] **Phase 5 — public content:** directory and tradition/species/source pages; SSR metadata, canonical/OG tags, sitemap/robots; accessibility and multilingual-ready names/content; privacy-conscious analytics and share cards.
- [ ] **Phase 6 — planting:** verified partner and offer registry; local suitability/availability; referral consent and attribution; manual confirmation evidence workflow.
- [ ] **Phase 7 — hardening:** auth/RBAC for review tools; rate limits and CSRF/session controls; retention/deletion; backup and deployment runbooks; security/dependency/container checks; acceptance coverage.

## Decisions needed before Phase 1

1. Confirm that the first public discovery slice is limited to known Nakshatra input and reviewed mappings, without birth-date astrology calculations.
2. Confirm whether to include an administrative review UI in the first foundation slice or begin with controlled SQL/import workflows.
3. Identify initial authoritative or community-authorized source material and reviewers; none is present in the repository, so no cultural mapping seed is proposed here.

No real cultural mappings or partner data are seeded. Phase 1 implementation adds synthetic test fixtures only.

## Phase 1 approved refinements

- Mappings are stable identities with immutable, versioned revisions. Review decisions and publication events reference a specific revision; editing means creating a new revision. Publication history preserves the revision shown publicly.
- Cultural review and botanical review are independent. A revision can have zero or multiple candidate botanical identifications, each with separate evidence, confidence/certainty, and review status. Public output never labels an uncertain or multiply approved candidate definitive.
- Source records preserve bibliographic data, access date, attribution and usage rights. Evidence passages may be referenced without copied text; text storage is limited to material with a documented reuse basis.
- The Go API enforces review authorization. Review UI is localhost-only in this phase; admin mutation requests also require explicit local-development configuration and a loopback caller. Non-local admin access fails closed.
- Build sequence is schema/migrations/OpenAPI, backend workflow/tests, then local UI/public discovery. No chat, birth calculations, user accounts, planting, analytics, marketing, or real imported mappings.
