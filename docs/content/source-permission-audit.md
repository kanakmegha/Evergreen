# Everroot Source Permission Audit

**Date:** 2026-10-16  
**Reviewer:** OpenCode agent (automated, assisted by direct term-page inspection)  
**Phase:** Permission audit and workflow design only — NO data ingestion, NO scraping, NO import of real cultural mappings.  
**Status:** DRAFT for human legal review

---

## Scope and method

This audit assesses six research leads proposed for eventual collection of tree associations across traditions. For each source I inspected the publicly available site/terms where reachable, and recorded what is confirmed versus what remains an unresolved legal question.

Per the project's critical instruction:
- No scraping, bulk-download, copying, translation, or import of third-party cultural content has occurred and will not occur until rights are assessed.
- Permission is not inferred from public accessibility, absence of a copyright notice, or an AI interpretation of terms.
- Where terms are ambiguous or unreachable, the permission state is `unknown` and an explanation is given.
- `allowed_under_licence` and `public_domain_verified` are not used without specific supporting evidence.

**Controlled permission states:** `allowed_under_licence`, `written_permission_required`, `restricted`, `unknown`, `public_domain_verified`.

---

## Part A — Source-by-source permission assessment

### Source 1 — JharParks Nakshatra Van

| Field | Value |
|---|---|
| Source ID | `src-jharparks-nakshatra-van` |
| Title | Nakshatra Van — JharParks (Jharkhand Udyan Samiti) |
| Publisher / rightsholder | Jharkhand Udyan Samiti (JharParks), autonomous body under the Department of Forests, Environment and Climate Change, Government of Jharkhand |
| URL | https://jharparks.org/NakshatraVan.php |
| Source type | Government park / public garden webpage |
| Publication date | Page footer states "Last updated: 13/07/2025"; foundation stone laid 15 Nov 2002; construction completed 2 May 2003 |
| Access date | 2026-10-16 (date of this audit) |
| Licence name/version | **None stated** on the inspected page |
| Licence URL | None |
| Commercial use | Not permitted (no licence grant; all rights reserved) |
| Extraction | Not permitted (no licence grant) |
| Local storage | Not permitted (no licence grant) |
| Redistribution | Not permitted (no licence grant) |
| Adaptation / translation | Not permitted (no licence grant) |
| Attribution requirements | None stated |
| Share-alike conditions | None stated |
| Restrictions / third-party rights | Footer: "© Copyright 2025 by Jharparks" — copyright reserved. No open licence. |
| Automated access permitted? | **Unknown** — no robots.txt or automated-access policy inspected |
| Written permission required? | **Yes** (no licence grant; all rights reserved) — contact to confirm |
| Rightsholder contact | dirceo.jharparks@gmail.com, 0651-2482241, C6 Block C Forest Officers Colony Doranda Ranchi-834002 |
| Evidence supporting assessment | Direct inspection of https://jharparks.org/NakshatraVan.php (footer copyright, no licence, government body) |
| Permission state | `written_permission_required` (pending rightsholder confirmation) |

**Content of interest (do not copy):** The page publishes a 27-row Nakshatra → tree → scientific-name table, plus Rashi Van and Navagraha Van mappings, medicinal notes, and textual exposition drawn from Narada Samhita, Sharadatilaka, Vidyarnava Tantra, Narada Purana, Mantra Maharnava, Rajanighantu, Brihat Samhita, and Surya Siddhanta. These are cultural claims and botanical identifications that require separate rights assessment per material type (see Part B).

---

### Source 2 — Global Vikas Trust Nakshatra Garden PDF

| Field | Value |
|---|---|
| Source ID | `src-gvt-nakshatragarden-pdf` |
| Title | Nakshatra Garden (Hindi) information board PDF |
| Publisher / rightsholder | Global Vikas Trust (Mumbai, India) |
| URL | https://www.globalvikastrust.org/krishikul-information-boards/nakshatragarden/nakshatragarden-hindi.pdf |
| Source type | PDF information board / educational material |
| Publication date | Unknown |
| Access date | Could not access (see below) |
| Licence name/version | **Unknown** — document could not be inspected |
| Licence URL | None known |
| Commercial use | Unknown |
| Extraction | Unknown |
| Local storage | Unknown |
| Redistribution | Unknown |
| Adaptation / translation | Unknown |
| Attribution requirements | Unknown |
| Share-alike conditions | Unknown |
| Restrictions / third-party rights | Unknown |
| Automated access permitted? | Unknown |
| Written permission required? | **Yes** — document unreachable; written confirmation needed before any use |
| Rightsholder contact | connect@globalvikastrust.org, +91 8591314651, 101 & 102 Aabhar Condominium, Plot No.46, Near Jain Mandir, Ram Krishna Nagar, Khar West, Mumbai 400052 |
| Evidence supporting assessment | Attempted fetch returned HTTP 404 (document not found at the supplied URL). The GVT Terms of Use (inspected separately) state all content is protected and unauthorized use is prohibited. |
| Permission state | `unknown` (document inaccessible) → treat as `written_permission_required` until confirmed otherwise |

**Note:** The supplied PDF URL returned 404. Either the path is incorrect or the document has been removed. Do not attempt to locate or fetch alternate mirrors without rightsholder authorization.

---

### Source 3 — Global Vikas Trust Terms of Use

| Field | Value |
|---|---|
| Source ID | `src-gvt-terms-of-use` |
| Title | Terms of Use — Global Vikas Trust |
| Publisher / rightsholder | Global Vikas Trust |
| URL | https://www.globalvikastrust.org/terms-of-use |
| Source type | Legal terms |
| Publication date | © 2026 (footer) |
| Access date | 2026-10-16 |
| Licence name/version | **None** — all rights reserved |
| Licence URL | None |
| Commercial use | Not permitted |
| Extraction | Not permitted ("Unauthorized use of any materials is prohibited") |
| Local storage | Not permitted |
| Redistribution | Not permitted |
| Adaptation / translation | Not permitted |
| Attribution requirements | None stated (no licence grant) |
| Share-alike conditions | None |
| Restrictions / third-party rights | "All content and materials available on the Services, including but not limited to text, graphics, logos, and software, are the property of GLOBAL VIKAS TRUST or its licensors and are protected by intellectual property laws. Unauthorized use of any materials is prohibited." |
| Automated access permitted? | Not stated |
| Written permission required? | **Yes** |
| Rightsholder contact | connect@globalvikastrust.org |
| Evidence supporting assessment | Direct inspection of the Terms of Use page (full text retrieved) |
| Permission state | `restricted` / `written_permission_required` |

**Confirmed fact:** GVT explicitly reserves all rights and prohibits unauthorized use. No collection from GVT digital properties may proceed without written permission.

---

### Source 4 — Kew Plants of the World Online (POWO)

| Field | Value |
|---|---|
| Source ID | `src-kew-powo` |
| Title | Plants of the World Online (POWO) |
| Publisher / rightsholder | Board of Trustees of the Royal Botanic Gardens, Kew (RBG Kew) |
| URL | https://powo.science.kew.org/ |
| Source type | Botanical database / taxonomic reference |
| Publication date | Continuous database; individual records have metadata |
| Access date | 2026-10-16 |
| Licence name/version | Dataset metadata: **Creative Commons Attribution (CC BY)**. Multimedia assets: by default CC BY, © RBG Kew. Individual dataset records carry their own licence field. |
| Licence URL | http://www.opendefinition.org/licenses/cc-by |
| Commercial use | **Allowed** under CC BY (with attribution) |
| Extraction | **Allowed** under CC BY for dataset metadata (with attribution) |
| Local storage | **Allowed** under CC BY (with attribution) |
| Redistribution | **Allowed** under CC BY (with attribution) |
| Adaptation / translation | **Allowed** under CC BY (with attribution) |
| Attribution requirements | Yes — appropriate credit/citation required. Example POWO citation: "POWO (2019). 'Plants of the World Online. Facilitated by the Royal Botanic Gardens, Kew. Published on the Internet; http://www.plantsoftheworldonline.org/ Retrieved [date]'." |
| Share-alike conditions | None (CC BY is not share-alike) |
| Restrictions / third-party rights | Must not imply Kew endorsement/affiliation. Kew does not warrant quality/accuracy. Multimedia may carry additional restrictions. Data contributed by others is subject to the data owner's terms. |
| Automated access permitted? | **Conditional** — API traffic limits apply ("must not exceed a certain number of requests per second... We reserve the right to suspend API access for users that exceed these limits"). |
| Written permission required? | No for CC BY-licensed dataset metadata (attribution suffices); **Yes** for any material not under CC BY or where rights are unclear. |
| Rightsholder contact | Biodiversity Informatics team (email via kew.org contact form) |
| Evidence supporting assessment | Direct inspection of https://www.kew.org/science/collections-and-resources/data-and-digital/terms-of-use and https://powo.science.kew.org/ |
| Permission state | `allowed_under_licence` (CC BY) **for dataset metadata only**, subject to per-record licence field verification |

**Confirmed fact:** Kew publishes dataset metadata under CC BY and requires attribution. Botanical facts (scientific names, native range, uses) extracted from CC BY records may be used with attribution; photographs/illustrations and any non-CC BY material require separate assessment.

---

### Source 5 — Kew science-data terms of use

| Field | Value |
|---|---|
| Source ID | `src-kew-terms` |
| Title | Terms and conditions — Kew Science Data & Resources |
| Publisher / rightsholder | Board of Trustees of the Royal Botanic Gardens, Kew |
| URL | https://www.kew.org/science/collections-and-resources/data-and-digital/terms-of-use |
| Source type | Legal terms |
| Publication date | Not stated |
| Access date | 2026-10-16 |
| Licence name/version | CC BY for dataset metadata; CC BY default for multimedia |
| Licence URL | http://www.opendefinition.org/licenses/cc-by |
| Commercial use | Allowed under CC BY (with attribution) |
| Extraction | Allowed under CC BY (with attribution) |
| Local storage | Allowed under CC BY (with attribution) |
| Redistribution | Allowed under CC BY (with attribution) |
| Adaptation / translation | Allowed under CC BY (with attribution) |
| Attribution requirements | Yes — cite as scientific research |
| Share-alike conditions | None |
| Restrictions / third-party rights | No implied Kew endorsement; no accuracy warranty; third-party contributed data subject to owner's terms; API traffic limits |
| Automated access permitted? | Conditional (traffic limits) |
| Written permission required? | No for CC BY material; Yes otherwise |
| Rightsholder contact | Biodiversity Informatics team |
| Evidence supporting assessment | Direct inspection of the Kew Science Data & Resources Terms of Use page |
| Permission state | `allowed_under_licence` (CC BY) for covered material |

---

### Source 6 — UNESCO Intangible Cultural Heritage lists

| Field | Value |
|---|---|
| Source ID | `src-unesco-ich` |
| Title | Lists of Intangible Cultural Heritage — UNESCO ICH |
| Publisher / rightsholder | UNESCO (Secretariat of the 2003 Convention); content submitted by States Parties |
| URL | https://ich.unesco.org/en/lists |
| Source type | Intergovernmental heritage inventory |
| Publication date | Ongoing; elements inscribed by year |
| Access date | 2026-10-16 |
| Licence name/version | **Not found on the inspected lists page** |
| Licence URL | None found |
| Commercial use | Unknown |
| Extraction | Unknown |
| Local storage | Unknown |
| Redistribution | Unknown |
| Adaptation / translation | Unknown |
| Attribution requirements | Unknown |
| Share-alike conditions | Unknown |
| Restrictions / third-party rights | "The sole responsibility for the content of each nomination file lies with the submitting States Parties concerned." Content may embody community-held traditional knowledge. |
| Automated access permitted? | Unknown |
| Written permission required? | **Unknown** — likely yes for community-held traditional knowledge; UNESCO Secretariat cannot license third-party/State Party content on their behalf |
| Rightsholder contact | UNESCO ICH Secretariat (via ich.unesco.org) |
| Evidence supporting assessment | Direct inspection of the ICH lists page; no open licence statement found in retrieved content |
| Permission state | `unknown` → `written_permission_required` for any cultural content |

**Unresolved legal question:** UNESCO ICH content is nominated by States Parties and may involve community-held traditional knowledge. The 2003 Convention requires community consent and involvement for safeguarding. UNESCO's publication of nomination files does not grant Everroot a licence to extract or reuse that content. A separate UNESCO terms-of-use page and, where applicable, community authorization must be obtained before any collection.

---

### Summary table

| Source | Permission state | Collection allowed today? | Written permission needed? |
|---|---|---|---|
| JharParks Nakshatra Van | `written_permission_required` | No | Yes |
| GVT Nakshatra Garden PDF | `unknown` | No | Yes (confirm access first) |
| GVT Terms of Use | `restricted` | No | Yes |
| Kew POWO (dataset metadata) | `allowed_under_licence` (CC BY) | Yes, with attribution, subject to per-record licence | No (for CC BY material) |
| Kew science-data terms | `allowed_under_licence` (CC BY) | Yes, with attribution | No (for CC BY material) |
| UNESCO ICH lists | `unknown` | No | Likely yes (community/State Party consent) |

**Sources cleared for limited collection today (with attribution):** Kew POWO dataset metadata and Kew multimedia where the per-record licence is CC BY.

**Sources requiring written permission before any collection:** JharParks, GVT (website + PDF), UNESCO ICH cultural content.

---

## Part B — Rights separation by material type

A single source may bundle material under different rights regimes. Everroot must assess each material type independently and must not assume a licence for one type covers another.

| Material type | Example in candidate sources | Rights considerations | Typical permission state |
|---|---|---|---|
| Factual botanical data | POWO scientific names, native range, uses | Facts are generally not copyrightable in many jurisdictions, but database structure/selection may be; follow the dataset's stated licence | Per-dataset licence (CC BY for POWO metadata) |
| Cultural claims or mappings | JharParks Nakshatra→tree table; GVT Nakshatra Garden | May be traditional knowledge; attribution to source tradition/region required; do not collapse conflicting sources | `unknown` / `written_permission_required` |
| Original explanatory prose | JharParks concept text; GVT site text | Copyright-protected expression; licence required | `written_permission_required` |
| Creative table or compilation | The 27-row Nakshatra tree table | Compilation copyright may exist independently of underlying facts | `written_permission_required` |
| Translations | UNESCO nomination translations; any Hindi→English rendering | Translation is a derivative work; translation rights must be recorded separately | `written_permission_required` |
| Photographs, diagrams, illustrations | POWO images; JharParks gallery images | Separate copyright; POWO images default CC BY but may carry other restrictions | Per-asset licence |
| Community-held / potentially restricted traditional knowledge | UNESCO ICH nominations; Nakshatra Van Ayurvedic/Tantric notes | May require community consent (Free, Prior, and Informed Consent principles under UNESCO 2003 Convention); not licensable by the publisher alone | `unknown` / `written_permission_required` |

**Rule:** A `source_permission` record must reference one or more `material_type` records, each with its own permission state. Collection is allowed only for material types whose recorded state permits the proposed use.

---

## Part C — Permission gate design

### Requirements

The gate must block collection or publication when:
1. The applicable permission state is `unknown` or `restricted`.
2. Commercial use is not allowed for a commercial deployment.
3. The proposed operation exceeds the recorded licence scope.
4. Required written permission is missing.
5. Required attribution or licence conditions cannot be met.
6. Required community consent or authorization is unresolved.

A human reviewer may override only by recording:
- documented legal grounds,
- supporting evidence,
- reviewer identity,
- timestamp, and
- scope of the override.

Overrides must never be silent.

### Design

The gate is implemented as a Go service (`internal/permission`) with:
- A SQL-backed permission lookup (append-only, immutable).
- A decision function `Evaluate(ctx, sourceID, materialType, operation, deploymentCommercial, attributionMet, communityConsent)` returning `allowed`/`blocked` plus reasons.
- An override function `RecordOverride(...)` that writes to an append-only `permission_overrides` table and requires legal grounds + evidence.

The gate does **not** consult an LLM for legal conclusions. It only applies recorded, human-reviewed evidence.

### Pseudocode / decision logic

```
Evaluate(sourceID, materialType, operation, isCommercial, attributionMet, communityConsent):
  row = lookup source_permissions for (sourceID, materialType)
  if not found: return BLOCKED, "no permission record"
  if row.state in {unknown, restricted}: return BLOCKED, row.state
  if row.written_permission_required and not row.written_permission_on_file: return BLOCKED, "written permission missing"
  if isCommercial and not row.commercial_use_allowed: return BLOCKED, "commercial use not allowed"
  if operation not in row.allowed_operations: return BLOCKED, "operation outside licence scope"
  if not attributionMet and row.attribution_required: return BLOCKED, "attribution condition not met"
  if not communityConsent and row.community_consent_required: return BLOCKED, "community consent unresolved"
  return ALLOWED
```

See `internal/permission/gate.go` for the proposed implementation skeleton.

---

## Part D — Data collection and review design (controlled workflow)

A ten-step workflow that keeps provenance and permission evidence attached to every record:

1. **Register the source** — create a `sources` row (title, publisher/rightsholder, URL, source type, publication date, access date, bibliographic reference, language, contact) plus a `source_permissions` row with the permission state, licence, allowed operations, attribution requirements, and reviewer. State starts `unreviewed`.
2. **Identify collectible material types** — for the source, list each material type present (botanical data, cultural claims, prose, compilation, translations, images, traditional knowledge) and record a per-type permission state.
3. **Run the permission gate** — for each proposed collection operation, call `Evaluate()`. Blocked material types are excluded from collection.
4. **Collect only permitted material** — collect bibliographic citations, evidence locators (page/section/URL fragment), and short excerpts only where rights permit. Never collect full text unless `reuse_status = permitted`.
5. **Store citations and locators, not full text** — store source reference + locator in `source_evidence`; store copied passage text only when `reuse_status = permitted` and translation rights allow.
6. **Extract cultural claims into separate records** — each claim becomes its own record keyed to tradition/region/Nakshatra; conflicting claims from different sources remain separate (never collapsed into one "universal" answer).
7. **Separate cultural verification from botanical identification** — cultural claims go to cultural review; botanical candidates (scientific name, certainty, confidence) go to independent botanical review, each with its own evidence.
8. **Queue records for qualified human review** — records enter a review queue; cultural reviewer and botanical reviewer act independently; decisions are append-only.
9. **Import approved records into Everroot's staging workflow** — approved records enter `mapping_revisions` / `botanical_candidates` via the existing internal API (localhost-only, authorized reviewer).
10. **Publish only approved, rights-cleared records** — publication passes the existing PostgreSQL + API publication gates (latest approved review, verified source, permitted/citation-only rights, newest revision only). Unapproved or rights-blocked records never reach the public discovery API.

---

## Part E — Repository changes and report

### E.1 Permission registry schema (new migration)

Proposed migration `db/migrations/0010_source_permissions.sql` creates:

- `source_permissions` — one row per (source, material_type), recording the human-reviewed permission state, licence, allowed operations, attribution/commercial/community flags, written-permission status, reviewer, evidence, and timestamps.
- `permission_overrides` — append-only record of human overrides with legal grounds, evidence, reviewer, timestamp, and scope.
- `permission_gate_decisions` — append-only audit log of every gate evaluation (allowed/blocked + reasons), for reproducibility.

The migration is written to be runnable against an empty or already-migrated database and to preserve all existing tables (no destructive changes).

### E.2 Migration

Created at `db/migrations/0010_source_permissions.sql`.

### E.3 Source-audit report format

The report (this document) is the source-audit report. Going forward, each source should have a machine-readable audit record (JSON or DB row) containing all Part A fields plus:
- `audit_id`, `auditor`, `audited_at`, `evidence_urls[]`, `findings`, `open_questions`, `next_action`, `review_due_at`.

### E.4 Import pipeline and permission-gate tests

Proposed tests (to be added under `internal/permission` and `internal/httpapi`):
- `TestPermissionGateBlocksUnknown` — state `unknown` → blocked.
- `TestPermissionGateBlocksRestricted` — state `restricted` → blocked.
- `TestPermissionGateBlocksMissingWrittenPermission` — written permission required but not on file → blocked.
- `TestPermissionGateBlocksCommercialUse` — commercial deployment, commercial not allowed → blocked.
- `TestPermissionGateBlocksExceedingScope` — operation not in `allowed_operations` → blocked.
- `TestPermissionGateBlocksMissingAttribution` — attribution required but not met → blocked.
- `TestPermissionGateBlocksMissingCommunityConsent` — consent required but unresolved → blocked.
- `TestPermissionGateAllowsClearedMaterial` — all conditions met → allowed.
- `TestPermissionOverrideRequiresLegalGrounds` — override without grounds/evidence/reviewer/timestamp/scope rejected.
- `TestPermissionOverrideIsRecorded` — override written to append-only table, never silent.
- Integration test: import pipeline rejects a source whose gate evaluation is blocked.

### E.5 Safe storage of source references and rights evidence

- Store only bibliographic citations and locators by default.
- Store copied passage text only when `reuse_status = permitted` and translation rights allow (enforced by existing triggers in `0001`–`0009`).
- Never store secrets, credentials, or unnecessary personal text in `audit_events` / `permission_gate_decisions`.
- All permission records are append-only; corrections are new versions, not in-place edits (consistent with the existing immutability model).
- Link every `mapping_revision` and `botanical_candidate` to its source evidence so public output always shows provenance.

---

## Verified facts vs. unresolved questions (summary)

**Confirmed by direct inspection:**
- GVT Terms of Use: all rights reserved, unauthorized use prohibited, contact connect@globalvikastrust.org.
- Kew POWO terms: dataset metadata CC BY, attribution required, no implied endorsement, API traffic limits, contact Biodiversity Informatics team.
- JharParks page: © Copyright 2025 by Jharparks, no open licence, government of Jharkhand body, contact dirceo.jharparks@gmail.com.
- The GVT Nakshatra Garden PDF URL returned HTTP 404 (unreachable).
- UNESCO ICH lists page contains no open licence statement; content responsibility rests with submitting States Parties.

**Unresolved (require human legal review):**
- Exact licence (if any) for JharParks content — written confirmation needed.
- Whether the GVT PDF can be accessed at a corrected URL, and its licence — written confirmation needed.
- UNESCO ICH licence and community consent requirements — written confirmation needed.
- Per-dataset licence field for specific POWO records to be used — verify each record before use.
- Whether any candidate source contains community-held traditional knowledge requiring FPIC-style consent.

**Sources needing written permission before further collection:**
- JharParks Nakshatra Van
- Global Vikas Trust (website and Nakshatra Garden PDF)
- UNESCO ICH cultural content

**Sources cleared for limited collection today:**
- Kew POWO dataset metadata under CC BY (with attribution, per-record licence check, no implied endorsement).