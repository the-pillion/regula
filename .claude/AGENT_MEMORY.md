# Regula — Agent Memory

Cross-session context for any AI agent (Claude, Codex, Gemini) or future contributor working on Regula. Captures what was learned, decided, and why — beyond what `README.md` and `ARCHITECTURE.md` already cover.

Last updated: 2026-04-26.

> **Big shift recorded 2026-04-26 (afternoon):** Regula will own its **own admin dashboard** (for lawyers / compliance team), not delegate admin to `pillion-core`. Plus public visibility of documents moves from env whitelist (`REGULA_PUBLIC_LEGAL_KEYS`) to a **DB column on `documents`** (`is_publicly_visible`). See Decisions 8 + 9.

---

## 1. What Regula is (anchor reminder)

Regula is the **standalone compliance-records service** for Pillion. It is the single source of truth for:

- versioned legal documents (privacy, terms, cookie, impressum, community guidelines, etc)
- locale + audience variants of those documents
- acceptance ledger (append-only, who accepted what version when)
- consent ledger (event history — granted/revoked, never overwritten)
- consent purpose registry
- processor + subprocessor registry
- retention policy registry
- Article 30 processing activities
- DPIA records
- per-subject evidence bundle reads

It is **not** an IAM, not a public website, not a legal interpreter. It stores evidence and serves it back. It might be "legal CMS" but yet not but looking forwared. — and looking forward, Regula **will grow a thin legal-CMS layer** for lawyers to manage document content + visibility directly without developer intervention. That CMS layer is the only acceptable scope expansion. Everything else stays narrow. That narrowness is the point.

Stack: Go 1.26.1 + Chi 5 + pgx 5 + sqlc-generated Postgres store + Neon as primary DB + distroless image. No framework, no Redis, no tracing. Port 8085. Prod host `regula.ms.thepillion.com` via Traefik. In order to run locally, use docker only.

Auth: Zitadel **opaque tokens via introspection** (not JWT-only). Validates issuer + audience + machine-identity allowlist. Caller services use OAuth2 `client_credentials` to fetch short-lived tokens.

---

## 2. Naming caution (do not confuse)

- **Regula** (this service) → legal/GDPR evidence ledger.
- **`RegulatoryZone`** / **`ZoneRequirement`** in `pillion-core` → city/region transport regulations (e.g. Bolzano permit rules, EU country regulations). Lives in core's DB. Stores driver permit constraints, vehicle requirements per zone. **Unrelated to Regula.**

If a task says "regulatory" or "regulation," ask which domain. Wrong guess = wrong repo, wrong DB, wrong domain.

- **PrefactGuard** (also in `pillion-services/`) → trust-and-safety machinery (driver document verification, fraud signals, rating, user behaviour check). **Different from Regula.** PrefactGuard = platform compliance posture (who can drive). Regula = legislative compliance posture (what user accepted, what consent state is).

---

## 3. How Regula fits in the Pillion system

Three repos in `pillion/`:

- `pillion-dashboard` — React/TanStack admin/onboarding frontend, Cloudflare Worker.
- `pillion-core` — Laravel monolith. Business logic, ride lifecycle, profiles, admin UI.
- `pillion-services/` — sibling Go microservices: `regula`, `prefactguard`, `payment-service`, `price-arbiter`, `compass` (tracking), `herald` (notifications), and others.

**Regula is consumed by `pillion-core`** through an HTTP client. Configuration in `pillion-core/config/services.php` under `regula`. Toggle via `REGULA_ENABLED`. Base URL via `REGULA_URL`.

Core-side integration points:

- `app/Services/RegulaClient.php` — HTTP client wrapper.
- `app/IAM/RegulaIdentityConsentRecorder.php` — writes acceptance + consent during identity flows.
- `app/Http/Controllers/LegalDocumentController.php` — public legal page reads (will likely shrink/remove once `/public/*` lands).
- `app/Http/Controllers/Admin/LegalDocumentController.php` + `Admin/DocumentController.php` — admin publishing UI.
- `app/Http/Controllers/Admin/ComplianceRegistryController.php` — admin manages processors/retention/Art.30/DPIA.
- `app/Http/Controllers/ProfileController.php` — consent toggles in profile.
- `app/Http/Controllers/GdprController.php` — GDPR export reads subject evidence bundle.

**Regula does not subscribe to events.** It only knows what services tell it via writes. There is no webhook listener, no Zitadel event subscription, no user lifecycle awareness. The `subject_ref` field is just an opaque string — typically the Zitadel `sub`. But Its open discussion for betternes.

### Registration flow (canonical)

```
User submits signup (with terms + privacy checkbox + marketing checkbox)
        |
        v
Zitadel creates the identity (or webhook fires post-creation)
        |
        v
pillion-core receives signup completion
        |
        +--> writes user row in core DB (profile, role, status)
        |
        +--> POST Regula /v1/acceptances
        |     subject_ref = zitadel sub
        |     document_version = current published terms + privacy versions
        |     ip, user_agent, source_service=pillion-core
        |
        +--> POST Regula /v1/consents (one per purpose chosen)
              subject_ref = zitadel sub
              purpose = newsletter-email | marketing-email | marketing-sms
              status = granted | revoked
              document_version = privacy version that was active
```

Same pattern fires for later events: profile consent toggles, new terms acceptance, GDPR export reads.

**Important property:** never overwrite. Each consent change is a new event row. Three flips → three rows. That is the audit trail.

---

## 4. The two API surfaces

### `/v1/*` — internal, authenticated

All routes require Zitadel-introspected bearer tokens. Audit-logged. Used by `pillion-core` for writes (acceptance, consent, document publishing, governance registry management) and for admin reads (subject evidence bundle, version lists, registry lists).

### `/public/*` — anonymous, GET-only, hardened (added 2026-04-26)

Designed for embedding on `thepillion.com`, dashboard worker, marketing site, iframes — without proxying through `pillion-core`, upcoming mobile app soon. Lives in `internal/api/public.go`.

Routes:

- `GET /public/legal/{key}.html` — published document body as HTML
- `GET /public/legal/{key}.md` — same as markdown
- `GET /public/legal/{key}.json` — body + metadata
- `GET /public/legal/{key}/versions.json` — list of published versions
- `GET /public/legal/{key}/versions/{version}.html|.md|.json` — pinned historical version
- `GET /public/subprocessors.html` — server-rendered table
- `GET /public/subprocessors.json` — same data as JSON

Hardening rules (non-negotiable):

1. **Public visibility lives in the DB, not env.** Add column `is_publicly_visible BOOLEAN NOT NULL DEFAULT FALSE` on `documents`. Public surface filters on `is_publicly_visible = TRUE` AND `is_published = TRUE` on the version. No more `REGULA_PUBLIC_LEGAL_KEYS` env (planned removal — see follow-ups). Note on type: Postgres has a native `BOOLEAN` (no `TINYINT` like MySQL); if future states are needed (`internal` / `public` / `restricted` / `archived`), migrate to `visibility VARCHAR(32) NOT NULL DEFAULT 'internal'` instead. Boolean now, enum-ish later if real need emerges. Lawyers flip the bit in the dashboard; no developer involvement, no redeploy.
2. **Only `is_published=true` versions returned.** Drafts never leak.
3. **Subprocessor projection filters out** `notes`, `dpa_status`, `transfer_mechanism`. Only safe transparency fields (`display_name`, `relationship_type`, `service_area`, `website_url`, `primary_country`, `data_location`, active records).
4. **DPIA, full Article 30 register, retention-policy details, acceptance/consent ledgers** never exposed publicly. Internal `/v1/*` only.
5. **Cache-Control: public, max-age=300, s-maxage=3600, stale-while-revalidate=86400** (configurable). But in future, a separte dedicated service we might introduce for public urls for caching and protection.
6. **ETag** from `content_sha256`. Supports `If-None-Match` → 304.
7. **Vary: Accept-Language, Origin**.
8. **CORS** only for explicit origin allowlist (`REGULA_PUBLIC_CORS_ALLOWED_ORIGINS`). No wildcard.
9. **Audit log middleware not applied** to `/public/*` to avoid flooding logs with anonymous reads.
10. **GET only.** Different middleware chain. POST/PATCH/DELETE never reach this surface.

### Public vs internal exposure matrix

| Data | `/public/*` | `/v1/*` (admin) | Notes |
|---|---|---|---|
| Privacy / Terms / Cookie / Impressum (published) | yes | yes | Locale + audience aware |
| Community guidelines (published) | yes | yes | Optional public doc |
| Subprocessor list (filtered, active) | yes | yes | Public table omits internal notes, DPA status, transfer mechanism text |
| Processor full record (DPA status, notes) | no | yes | Admin-only via `/v1/processors` |
| Retention policies | no | yes | Internal record |
| Article 30 processing activities | no | yes | Supervisory authority on request, not public |
| DPIA records | no | yes | Internal + auditors only |
| Acceptance ledger | no | yes | Per-subject audit only |
| Consent ledger | no | yes | Per-subject audit only |
| Subject evidence bundle | no | yes | GDPR export reads via core's `GdprController` |

**Rule for public surface growth:** stay strict. Add a new `/public/*` route only when GDPR transparency norms or product UX justifies it. Default no.

---

## 5. Decisions made in conversation (with reasoning)

### Decision 1 — Public read endpoint belongs on Regula, not behind a core proxy

**Choice:** add `/public/*` directly to Regula.

**Why:**
- Legal pages are inherently public (regulators, scrapers, signed-out users).
- Routing via `pillion-core` adds latency, load, and deploy coupling for nothing.
- CDN-cacheable.
- Embeds (iframe / server-side include / `<object>`) work cleanly from one URL.
- README's "keep Regula internal" guidance is a default safety stance, not a hard rule. Published legal text is the legitimate exception.

**Tradeoff acknowledged:** public surface must stay narrowly scoped. Don't let it grow.

**Once `/public/*` is consumed in production, remove the public legal-page rendering path from `pillion-core/LegalDocumentController.php` so there is one source of public legal content, not two.**

### Decision 2 — Server-rendered HTML over JS-fetched tables

**Choice:** `/public/subprocessors.html` returns a server-rendered HTML table. Same for legal docs.

**Why:**
- Cacheable at CDN.
- No CORS round-trip.
- No flash-of-empty-content.
- Works without JS (accessibility, crawlers).
- Same auth/cache story as the legal docs.

JS fetch only if live filtering/sorting is needed. For a static "here are our subprocessors" table, server-side wins.

### Decision 3 — DPIA, Article 30, retention details stay private

**Choice:** never publish DPIA, full Article 30 register, retention-policy specifics.

**Why:**
- DPIA = internal risk assessment. Auditors and regulators get it on request. Publishing harms posture more than it helps.
- Article 30 = regulator-on-demand record under GDPR. Not a public transparency document.
- Retention specifics = better summarized in privacy policy text in plain language than dumped as raw rows.

Public surface gets only what GDPR transparency norms expect: legal documents + subprocessor list.

### Decision 4 — BCP-47 locale codes (`it-IT`, `de-DE`, `es-ES`, `en-GB`)

**Choice:** when country variants matter, use BCP-47 (`language-COUNTRY`). When only language matters, plain language code (`it`, `de`).

**Why:**
- No schema change needed — `locale` is already a free string.
- Privacy policy can stay one EU-wide doc per language (`it`, `de`, `es`, `en`).
- Terms of service must be country-specific (consumer law, VAT, gig-worker rules differ) → use `it-IT`, `de-DE`, `es-ES`.
- Future "Italian for Switzerland" (`it-CH`) just works.

### Decision 5 — One key per logical document, not URL-segment trees

**Choice:** `/public/legal/{key}.html?lang=it-IT`, not `/legal/document/?name=...&country=...&lang=...` (Uber-style query soup).

**Why:**
- Cleaner, more cacheable URLs.
- `latest` is implied by absence of `versions/{x}` segment.
- Document keys already use kebab-case in the DB; URLs match.

### Decision 6 — Launch with minimum legal doc set, not Uber-scale

**Choice:** 5–8 documents max for v1. Not 50.

**Why:**
- Uber's surface accreted over a decade with a legal team.
- A single founder shipping an Italy-first MVP cannot maintain 50 documents.
- Each unmaintained legal page is a liability, not protection.

### Decision 7 — Source legal text from a generator service first, lawyer second

**Choice:** start with iubenda / Termly / similar (€10–€50/month). Get an Italian gig-economy lawyer review only after first ~100 paying drivers.

**Why:**
- Founder cannot draft GDPR-compliant Italian legal text from scratch.
- Generators give a defensible starting point in multiple languages.
- Lawyer review is high-value once there are real driver contracts and real money.

---

## 6. Multi-country, multi-language strategy

### EU language law reality

You **cannot** operate consumer-facing in an EU country with English-only.

- **Italy** — Italian (Codice del Consumo art. 35 requires plain Italian for consumer terms). English allowed *additionally*.
- **Germany** (Munich, Berlin) — German.
- **Spain** (Barcelona, Madrid) — Spanish. Catalan recommended in Catalonia, not legally required.
- **France** (if you go) — French (Loi Toubon).
- English-only fallback is fine for a tourist-marketing landing page. Not for terms users sign.

Drivers are subject to the same rule — their working contracts must be in their working language.

### Country variants

Privacy policy can be **one EU-wide document per language**. GDPR is harmonized.

Terms of service usually need **per-country variants**:
- VAT treatment differs.
- Consumer cancellation/withdrawal rights differ.
- Driver classification differs sharply (Italian autonomous vs German employee-style vs Spanish "rider law").
- Jurisdiction + governing-law clause differs.

Either fully separate per-country docs OR a "core terms + thin per-country addendum" pattern. Fully separate is simpler for v1.

### Concrete launch matrix (Italy primary, DE/ES tourist cities)

```
key                           locales (BCP-47)         audience
privacy-policy                it, de, es, en           all
cookie-policy                 it, de, es, en           all
impressum                     de, en                   all
terms-of-service-passenger    it-IT, de-DE, es-ES, en  passenger
terms-of-service-driver       it-IT, de-DE, es-ES, en  driver
community-guidelines          it, de, es, en           all   (optional)
```

≈ 6 keys × 4 locales ≈ 20 `document_versions` rows. Manageable.

**For first launch (Italy only):** seed `it-IT` versions of the passenger + driver terms, `it` versions of privacy + cookie + community guidelines, `en` fallback for non-Italian-speaking users browsing. That is enough.

Add `de-DE` + `es-ES` only when you actually open Munich / Barcelona for real bookings, **not pre-emptively**. Pre-emptive translations rot.

### Things you do NOT need at launch

- Uber-style linked sub-pages (`/legal/document/?name=...&country=...&lang=...`) — same data through complex URLs. Already covered by `/public/legal/{key}.html?lang=...`.
- Per-city documents. Cities are operational; legal unit is country.
- "Global English baseline" + "country override" CMS overlay system. Just write each version standalone.
- Accessibility statement (only required at certain user thresholds; add later).
- Cookie consent banner copy (the banner is product UX; the policy text it links to is `cookie-policy`, already handled).

---

## 7. Where each piece of compliance data is surfaced

| Data | Audience | Surface | Public? |
|---|---|---|---|
| Privacy / Terms / Cookie / Impressum | Everyone | `/public/legal/...` + embedded on `thepillion.com` | Yes |
| Subprocessor list | Everyone (GDPR transparency norm) | `/public/subprocessors.html` (filtered) | Yes |
| Processor full record | Internal | Admin panel in core via `/v1/processors` | No |
| Retention summary (high-level) | Optional public | Could be a public page later | Your call |
| Retention policy specifics | Internal | Admin panel via `/v1/retention-policies` | No |
| DPIA records | Internal + auditors | Admin panel via `/v1/dpia-records` | No, never public |
| Article 30 register | Supervisory authorities only | Admin panel + export on request | No, not public |
| Acceptance + consent ledgers | Subject + auditors | GDPR export via `GdprController`, admin views | No |

---

## 8. Embedding patterns for HTML pages

Three valid ways to embed Regula's `/public/*` content into `thepillion.com` or any frontend:

1. **iframe** — simplest, totally isolated styling.
   ```html
   <iframe src="https://regula.ms.thepillion.com/public/subprocessors.html" loading="lazy"></iframe>
   ```
2. **Server-side include** — Laravel pulls the HTML in `LegalDocumentController` and injects into a Blade template. Inherits page styling.
3. **`<object>` tag** — between iframe and SSI. Rare.

JS fetch only when live filtering or sorting is needed. Otherwise stay server-rendered.

CSS for the embedded subprocessors table can be styled via a `.regula-subprocessors` class (already on the table). Style on the embedding side.

---

## 9. Operational and configuration cheat sheet

### Required env

- `REGULA_DATABASE_URL`
- `ZITADEL_ISSUER`
- `ZITADEL_PROJECT_ID`
- `REGULA_ALLOWED_SERVICE_IDS`
- `REGULA_ZITADEL_API_CLIENT_ID`
- `REGULA_ZITADEL_API_CLIENT_SECRET`

### Common env

- `REGULA_SERVICE_NAME` (default `regula`)
- `REGULA_HTTP_PORT` (default `8085`)
- `ZITADEL_INTROSPECTION_URI`
- `ZITADEL_INTROSPECTION_CACHE_TTL_SECONDS` (default `15`)
- `REGULA_ALLOWED_AUDIENCES`
- `REGULA_AUTO_MIGRATE`
- `REGULA_LOG_LEVEL`
- `REGULA_DOCUMENT_CACHE_TTL_SECONDS` (default `120`)
- `REGULA_DOCUMENT_CACHE_MAX_ITEMS` (default `64`)
- `REGULA_TRAEFIK_RATELIMIT_AVERAGE|BURST|PERIOD`

### Public surface env (added 2026-04-26)

- `REGULA_PUBLIC_LEGAL_KEYS` (CSV, default `privacy-policy,terms-of-service,cookie-policy,impressum`)
- `REGULA_PUBLIC_CACHE_MAX_AGE_SECONDS` (default `300`)
- `REGULA_PUBLIC_CACHE_SHARED_MAX_AGE_SECONDS` (default `3600`)
- `REGULA_PUBLIC_CACHE_STALE_REVALIDATE_SECONDS` (default `86400`)
- `REGULA_PUBLIC_CORS_ALLOWED_ORIGINS` (CSV, default empty = no CORS)

### Core-side env (in `pillion-core/.env`)

- `REGULA_ENABLED`
- `REGULA_URL` (prod `https://regula.ms.thepillion.com`, dev `http://host.docker.internal:8085`)
- `REGULA_TIMEOUT_SECONDS` (default `4`)

### Key files

- `cmd/regula/main.go` — entrypoint
- `internal/api/router.go` — route registration (auth + public groups)
- `internal/api/public.go` — anonymous read surface (legal docs + subprocessors)
- `internal/auth/middleware.go` + `zitadel.go` — opaque-token introspection
- `internal/cache/document_cache.go` — TTL cache for latest published doc reads
- `internal/config/config.go` — env loading
- `internal/seed/seed.go` — seed CLI logic
- `internal/store/` — sqlc-generated query layer
- `db/migrations/000001_init.up.sql` — schema
- `db/query/documents.sql`, `governance.sql`, `acceptances.sql`, `consents.sql` — sqlc sources
- `seed/foundation.json` + `seed/legal_documents/` — bootstrap data

### Commands

Inside container:
```bash
/regula serve
/regula migrate
/regula seed foundation
/regula reset-db --yes                    # dev/staging only
/regula reset-and-seed foundation --yes   # dev/staging only
```

From host:
```bash
docker compose up --build -d regula
docker compose exec regula /regula migrate
docker compose exec regula /regula seed foundation
```

`reset-db` and `reset-and-seed` are **never** run in production.

---

## 10. Things to refuse (scope creep watch)

If a future task or agent suggests any of the following, push back hard:

- Adding non-legal-document data to `/public/*`.
- Adding browser-session auth, OAuth login flows, or end-user direct access to Regula.
- Storing full website pages instead of document body content.
- Adding Redis, distributed tracing, or a heavy framework.
- Building a second admin UI inside Regula (admin lives in `pillion-core`).
- Letting acceptance or consent rows become mutable. Always append-only.
- Overwriting old document versions instead of publishing a new version.
- Pre-translating documents to languages for markets that are not actually launching.

Regula stays narrow. That narrowness is why it can run reliably on a small VPS.

---

## 11. Operating principles (carry forward)

1. **Append-only ledgers** — acceptance + consent are history, not state.
2. **Versioned, never overwritten** — old text must remain queryable to prove what was active at any past moment.
3. **Locale + audience as first-class** — not bolted on later.
4. **Separate DB from monolith** — Neon. Compliance schema must not churn with business schema.
5. **M2M-only auth** for `/v1/*`. Public reads only on `/public/*` with explicit hardening.
6. **Boring stack** — Go + Chi + sqlc + Postgres. Resist adding deps.
7. **Cache only the hot read** (`latest published document`). Everything else uncached.
8. **Rate-limit at Traefik**, not in-process.
9. **Body-only storage** — no full website pages, no chrome, no hostnames.
10. **One source of public legal text** — Regula, not duplicated in core.

---

## 12. Outstanding follow-ups (not yet done)

1. Add separate Traefik rate-limit middleware in `docker-compose.prod.yml` for `/public/*` (lower limits, IP-based) distinct from `/v1/*`.
2. Once `/public/*` is wired into `thepillion.com`, remove the public legal-page rendering path from `pillion-core/LegalDocumentController.php`.
3. Seed Italian (`it-IT` + `it`) versions of the launch document set when legal text is ready.
4. Consider a `/public/legal/{key}` (no extension) route that 302-redirects to `.html` with `lang` derived from `Accept-Language` — friendlier embed URL.
5. When second country launches, decide: per-country addenda doc or fully separate per-country terms doc. Either works; pick before seeding multi-country.

---

## 13. Quick mental model (for a new agent reading cold)

> Regula is a write-once ledger. Any service holding a Zitadel M2M token can append acceptance or consent events, publish new legal versions, or update governance registries via `/v1/*`. Anyone on the internet can read published legal documents and the active subprocessor list via `/public/*`. Nothing else is exposed publicly.
>
> The service that owns the user-facing moment (signup, profile change, admin action) is responsible for telling Regula what happened. Regula has no event subscriptions, no user awareness, no opinions about who someone is — only opaque `subject_ref` strings and structured records.
>
> Schema, auth boundary, and public surface are the three things to keep stable. Everything else can iterate.
