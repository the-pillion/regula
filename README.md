# Regula

Regula is Pillion's standalone compliance-records service.

It exists to keep legal and compliance evidence out of the main business services and to give the platform one dedicated source of truth for:
- versioned legal documents
- legal document publication history
- acceptance evidence
- consent history
- subject-facing evidence exports
- multilingual legal content

This service is intended to be small, explicit, and operationally boring. It does not try to interpret the law. It stores the records and evidence your platform needs in order to prove what text was active, what a user accepted, and when a consent state changed.

## Status
Current implementation status:
- standalone Go service is running
- Neon-backed PostgreSQL is configured as the primary database
- Docker image and Docker Compose runtime are working
- multilingual legal document seeding is working for `en`, `it`, and `de`
- core legal documents are seeded into the live database
- document retrieval is working through the API
- document content is stored as document-body HTML, not full website pages
- hardcoded public-site hostnames were removed from the stored legal content
- small in-memory TTL cache is enabled for latest published document reads

Still planned:
- Laravel monolith integration for public legal page reads
- consent and acceptance writes from monolith flows
- admin tooling for document publishing and evidence inspection
- future locale expansion beyond `en`, `it`, and `de`

## Why This Service Exists
The monolith should not be the long-term source of truth for compliance evidence. Legal content and consent history need different guarantees than ride lifecycle, pricing, or notifications.

Regula solves these problems:
- legal documents need versioning and publication timestamps
- terms/privacy text must be retrievable per locale and audience
- acceptance history must be append-only and auditable
- consent changes must be preserved as an event history
- future services should not each reinvent their own compliance ledger

This service gives Pillion one place to keep evidence that is stable, queryable, and reusable across services.

## Scope
Regula owns:
- legal documents and their versions
- locale-specific legal content
- audience-specific legal content
- consent purpose registry
- acceptance events
- consent events
- subject evidence bundle reads

Regula does not own:
- user authentication and login
- driver onboarding workflow itself
- payment logic
- ride lifecycle
- marketing delivery
- legal interpretation or legal advice

Zitadel remains the identity system. Regula is not an IAM system.

## Legal and Compliance Role
Regula is designed to support practical GDPR and platform-accountability needs, including:
- proving which privacy policy or terms text was active at a given moment
- proving which version a user accepted
- preserving consent grants and revocations as history, not mutable current-state only
- separating legal evidence from core business data
- supporting multilingual legal content from the start

Important boundary:
Regula helps you build a defensible compliance posture, but it does not by itself make the platform legally compliant. You still need:
- valid legal texts
- correct lawful-basis decisions
- real retention schedules
- processor contracts and vendor governance
- operational breach handling
- accurate product behavior around consent and rights

## Core Features
### 1. Versioned Legal Documents
Each document has a stable key, for example:
- `privacy-policy`
- `terms-of-service`
- `cookie-policy`
- `impressum`

Each document can have many versions split by:
- `locale`
- `audience`
- `version`
- `effective_from`
- `is_published`

That means you can publish:
- one English privacy policy for all users
- one Italian terms version for passengers only
- one German driver terms version for approved drivers

### 2. Acceptance Ledger
Acceptance events record:
- subject reference
- document version
- timestamp
- source service
- source app
- IP address
- user agent
- evidence digest
- metadata

This is the core evidence trail for terms/privacy acceptance.

### 3. Consent Ledger
Consent changes are stored as events, not overwritten state. Each event records:
- subject reference
- consent purpose
- legal document version
- status `granted` or `revoked`
- changed timestamp
- source service
- source app
- evidence digest
- metadata

Examples of consent purposes:
- `newsletter-email`
- `marketing-email`
- `marketing-sms`

### 4. Subject Evidence Bundle
The service can return a combined view per subject with:
- acceptance history
- consent history
- current consent states

This is intended as a foundation for later export and audit workflows.

### 5. Multilingual Support
Regula already supports multilingual legal content at the data model level and in the current seed set.

Current seeded locales:
- `en`
- `it`
- `de`

Future locales do not require schema changes. They only require new document versions with the relevant locale code.

## Important Content Rule
Do not store full public website pages as the canonical legal document source.

The service should store the legal document body only.

Why:
- full pages contain layout chrome and navigation that are not part of the legal text
- full pages often include canonical URLs and environment-specific hostnames
- full pages mix presentation concerns into legal evidence
- translations are easier to manage when the legal body is isolated

Current seed data now stores extracted `<main>` content only, not the full site HTML.

## Document Format
Regula supports both:
- `html`
- `markdown`

Recommended policy:
- use `html` for imported legal text from the current site or CMS
- use `markdown` for future editor-driven workflows if you want cleaner authoring and version control

The canonical source can be either, but it must be explicit per version via `content_type`.

## Architecture
### Stack
- Go 1.26.1
- Chi 5.2.5
- pgx 5.8.0
- sqlc-generated PostgreSQL store
- distroless runtime image
- Neon PostgreSQL as the primary database target
- optional local Postgres for isolated development

### Why This Stack
This stack was chosen because it is:
- lightweight
- predictable
- low-memory
- easy to deploy on restricted VPS resources
- easy to reason about without framework overhead

No heavy application framework was added. The service is intentionally direct.

## Data Model
Main tables:
- `documents`
- `document_versions`
- `consent_purposes`
- `acceptance_events`
- `consent_events`
- `schema_migrations`

High-level model:
- `documents` stores the stable identity of each legal document
- `document_versions` stores locale/audience/version-specific content
- `acceptance_events` records acceptance of a specific document version
- `consent_events` records changes to consent state linked to a specific legal version
- `consent_purposes` stores the defined consent namespaces used by the platform

## API
All routes except `/healthz` and `/readyz` require bearer authentication.

### Health
- `GET /healthz`
- `GET /readyz`

### Documents
- `POST /v1/documents`
- `POST /v1/documents/{key}/versions`
- `GET /v1/documents/{key}/versions/latest?locale=...&audience=...`

### Consent Purposes
- `POST /v1/consent-purposes`

### Evidence Writes
- `POST /v1/acceptances`
- `POST /v1/consents`

### Evidence Reads
- `GET /v1/subjects/{subjectRef}/acceptances`
- `GET /v1/subjects/{subjectRef}/consents/history`
- `GET /v1/subjects/{subjectRef}/consents/current`
- `GET /v1/subjects/{subjectRef}/bundle`

## Authentication
All routes except `/healthz` and `/readyz` require bearer authentication.

Primary production model:
- Zitadel opaque access tokens validated via introspection
- introspection with a dedicated Regula API client
- strict issuer check
- strict audience allowlist
- strict machine-identity allowlist

The service intentionally does not trust `aud` alone. ZITADEL's own security guidance recommends validating issuer, expiration, and additional authorization signals beyond audience checks. Regula therefore requires both:
- a configured allowed audience
- a configured allowlist of machine principals such as the monolith service-account `client_id`, `azp`, or `sub`

Required production variables:
- `ZITADEL_ISSUER`
- `ZITADEL_PROJECT_ID`
- `REGULA_ZITADEL_API_CLIENT_ID`
- `REGULA_ZITADEL_API_CLIENT_SECRET`
- `REGULA_ALLOWED_SERVICE_IDS`
- optional `REGULA_ALLOWED_AUDIENCES` if you do not want to default to `ZITADEL_PROJECT_ID`
- optional `ZITADEL_INTROSPECTION_URI` if you do not want it derived from the issuer

Recommended caller model:
- the monolith authenticates with Zitadel using `client_credentials`
- the monolith requests a short-lived access token
- the token includes a known audience accepted by Regula
- Regula introspects the access token against Zitadel and caches the result briefly to keep the service lightweight

What Regula needs from Zitadel:
- one machine client or service account for each calling service that needs access
- short-lived access tokens obtained via OAuth 2.0 `client_credentials`
- access tokens issued by Zitadel, validated through introspection

What Regula does not need:
- browser login
- user sessions
- public client auth
- direct end-user access

## Caching
Regula uses a very small in-memory TTL cache for latest published document reads only.

What is cached:
- `GET /v1/documents/{key}/versions/latest?...`

What is not cached:
- write operations
- acceptance history
- consent history
- subject bundle reads

Why this is enough:
- legal documents are read often but change rarely
- latest published version lookup is a clear hot path
- Redis is unnecessary at this stage
- this keeps memory and operations small

Current cache settings:
- `REGULA_DOCUMENT_CACHE_TTL_SECONDS`
- `REGULA_DOCUMENT_CACHE_MAX_ITEMS`

## Seeding
Regula includes seed data and a real CLI seed flow.

Current seed set includes:
- `privacy-policy`
- `terms-of-service`
- `cookie-policy`
- `impressum`
- locales `en`, `it`, `de`
- baseline consent purposes

Seed files live in:
- [seed/foundation.json](/Users/manpreet/Documents/project/startup/pillion-services/regula/seed/foundation.json)
- [/Users/manpreet/Documents/project/startup/pillion-services/regula/seed/legal_documents](/Users/manpreet/Documents/project/startup/pillion-services/regula/seed/legal_documents)

The seed logic is idempotent because it uses upsert behavior for:
- documents
- document versions
- consent purposes

That allows you to reset and repopulate development or staging data safely.

## Commands
Run inside the Regula container:
```bash
/regula serve
/regula migrate
/regula seed foundation
/regula reset-db --yes
/regula reset-and-seed foundation --yes
```

Typical operational commands from the project directory:
```bash
docker compose up --build -d regula
docker compose exec regula /regula migrate
docker compose exec regula /regula seed foundation
docker compose exec regula /regula reset-and-seed foundation --yes
```

## Environment Variables
Required:
- `REGULA_DATABASE_URL`
- `ZITADEL_ISSUER`
- `ZITADEL_PROJECT_ID`
- `REGULA_ALLOWED_SERVICE_IDS`

Common:
- `REGULA_SERVICE_NAME`
- `REGULA_HTTP_PORT`
- `ZITADEL_INTROSPECTION_URI`
- `ZITADEL_INTROSPECTION_CACHE_TTL_SECONDS`
- `REGULA_ZITADEL_API_CLIENT_ID`
- `REGULA_ZITADEL_API_CLIENT_SECRET`
- `REGULA_ALLOWED_AUDIENCES`
- `REGULA_AUTO_MIGRATE`
- `REGULA_LOG_LEVEL`
- `REGULA_DOCUMENT_CACHE_TTL_SECONDS`
- `REGULA_DOCUMENT_CACHE_MAX_ITEMS`

Current production-style database target is Neon.

Example:
```env
REGULA_SERVICE_NAME=regula
REGULA_HTTP_PORT=8085
REGULA_DATABASE_URL=postgresql://USER:PASSWORD@HOST/DATABASE?sslmode=require

ZITADEL_ISSUER=https://ztdl.apps.visifan.com
ZITADEL_PROJECT_ID=366080639541182472
ZITADEL_INTROSPECTION_URI=https://ztdl.apps.visifan.com/oauth/v2/introspect
ZITADEL_INTROSPECTION_CACHE_TTL_SECONDS=15

REGULA_ZITADEL_API_CLIENT_ID=366254444754501640
REGULA_ZITADEL_API_CLIENT_SECRET=replace-me
REGULA_ALLOWED_SERVICE_IDS=pillion-svc
REGULA_ALLOWED_AUDIENCES=366080639541182472

REGULA_AUTO_MIGRATE=false
REGULA_LOG_LEVEL=warning
REGULA_DOCUMENT_CACHE_TTL_SECONDS=120
REGULA_DOCUMENT_CACHE_MAX_ITEMS=64
```

## Neon Database
Regula is now configured to use Neon as the active database by default.

Why Neon is appropriate here:
- separate database ownership from the monolith
- managed Postgres without extra VPS database maintenance
- good fit for low-resource service architecture
- keeps compliance data isolated from business schema churn

Local Postgres remains available only as an optional development profile.

## Docker and Compose
### Development Compose
The default `docker-compose.yml` is for development and local verification.

Primary runtime:
- `regula` service container

Optional local profile:
- `regula-postgres` under profile `local-db`

Normal runtime with Neon:
```bash
docker compose up --build -d regula
```

Isolated local DB runtime:
```bash
REGULA_DATABASE_URL=postgresql://regula:regula@regula-postgres:5432/regula?sslmode=disable docker compose --profile local-db up --build -d
```

### Production Compose
Production should use [docker-compose.prod.yml](/Users/manpreet/Documents/project/startup/pillion-services/regula/docker-compose.prod.yml), not the default development compose file.

Files for production:
- [docker-compose.prod.yml](/Users/manpreet/Documents/project/startup/pillion-services/regula/docker-compose.prod.yml)
- [.env.prod.example](/Users/manpreet/Documents/project/startup/pillion-services/regula/.env.prod.example)

First production deploy:
```bash
cp .env.prod.example .env
# fill in secrets
docker compose -f docker-compose.prod.yml up -d --build
```

Production redeploy after code change:
```bash
docker compose -f docker-compose.prod.yml build regula
docker compose -f docker-compose.prod.yml up -d --no-deps regula
```

## VPS Deployment with Traefik
If you want to expose Regula at:
- `regula.ms.thepillion.com`

then the production deployment model is:
- Traefik on the VPS
- `docker-compose.prod.yml` for Regula
- external Docker network `traefik`
- DNS `A` record for `regula.ms.thepillion.com` pointing to your VPS IP
- TLS handled by Traefik

The production compose file already contains the Traefik router and service labels for:
- `Host(`regula.ms.thepillion.com`)`
- `websecure` entrypoint
- `letsencrypt` cert resolver
- backend service port `8085`

Important deployment note:
Regula is still an internal API service. Exposing it through Traefik gives it a stable hostname and TLS termination, but the routes must remain authenticated. Do not treat it as a public anonymous website.

If you later want public legal pages served through this domain, the recommended pattern is:
- keep Regula as the internal source of truth
- let a frontend or monolith read from Regula and present the public pages
- avoid putting public rendering concerns directly into Regula unless you really need that boundary

## Operational Guidance
### Safe Flow for Development or Staging
1. Configure `REGULA_DATABASE_URL`
2. Start the service with `docker compose up --build -d regula`
3. Run `reset-and-seed foundation --yes` only in non-production environments
4. Verify the seeded locales through the API
5. Integrate the monolith reads and write flows

### Safe Flow for Production
1. Copy `.env.prod.example` to `.env`
2. Fill in Neon production database credentials and strong auth tokens
3. Deploy with `docker compose -f docker-compose.prod.yml up -d --build`
4. Run `docker compose -f docker-compose.prod.yml exec regula /regula migrate`
5. Run `docker compose -f docker-compose.prod.yml exec regula /regula seed foundation` if initial legal texts should be loaded
6. Verify reads for `en`, `it`, and `de`
7. Enable the monolith integration
8. Replace static bearer tokens with Zitadel M2M JWT auth

### Do Not Use `reset-db` in Production
`reset-db` and `reset-and-seed` are destructive commands intended for development, staging, or controlled reinitialization only.

## Recommended Integration Pattern
Short-term:
- Laravel monolith reads published legal documents from Regula
- Laravel monolith writes acceptance and consent evidence to Regula
- Regula remains internal-only behind authenticated calls

Later:
- service-to-service JWT auth via Zitadel
- admin publishing tools
- subject export workflow
- retention-policy and processor-registry modules if you decide to extend Regula further

## What Is Done
Done now:
- Neon-backed DB setup
- multilingual seed import for `en`, `it`, `de`
- sanitized legal content body extraction
- hardcoded public host removal from stored content
- seed/reset CLI support
- live service verification

## What Is Next
Best next engineering steps:
1. integrate the Laravel legal pages so they read from Regula
2. send consent and acceptance writes from signup and profile flows
3. replace static bearer tokens with Zitadel service JWTs
4. add admin publishing workflow for new legal versions
5. add structured evidence export and audit endpoints

## File Guide
Key files in this service:
- [cmd/regula/main.go](/Users/manpreet/Documents/project/startup/pillion-services/regula/cmd/regula/main.go)
- [internal/api/router.go](/Users/manpreet/Documents/project/startup/pillion-services/regula/internal/api/router.go)
- [internal/seed/seed.go](/Users/manpreet/Documents/project/startup/pillion-services/regula/internal/seed/seed.go)
- [internal/config/config.go](/Users/manpreet/Documents/project/startup/pillion-services/regula/internal/config/config.go)
- [db/migrations/000001_init.up.sql](/Users/manpreet/Documents/project/startup/pillion-services/regula/db/migrations/000001_init.up.sql)
- [db/query/documents.sql](/Users/manpreet/Documents/project/startup/pillion-services/regula/db/query/documents.sql)
- [seed/foundation.json](/Users/manpreet/Documents/project/startup/pillion-services/regula/seed/foundation.json)

## Final Positioning
Regula is not meant to become a giant legal platform.

It should stay a narrow, dependable service that answers questions like:
- what legal text was active?
- what did the user accept?
- when did consent change?
- what evidence do we have?
- which locale and audience variant applied?

That narrowness is the main reason it can remain fast, understandable, and safe to operate on a small VPS.
