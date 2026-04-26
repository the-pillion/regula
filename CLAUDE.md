# Regula — Claude entry point

Your memory lives in the `../../raw` and `../../graphify-out` (cross-agent shared memory) and locally in `./.claude/` (Regula-specific). Those are the source of truth.

You or this session is mainly responsible for the **Regula** microservice. You are the agent that manages, edits, changes, develops Regula and you are overall responsible for this service.

## Read first, in order

1. **[.claude/AGENT_MEMORY.md](.claude/AGENT_MEMORY.md)** — full cross-session context, decisions, reasoning, scope-creep watchlist. **Mandatory cold-start read.**
2. **[README.md](README.md)** — service overview, stack, env, commands, public + internal API surfaces.
3. **[ARCHITECTURE.md](ARCHITECTURE.md)** — system design notes.
4. **`../../raw/service-regula.md`** — cross-agent shared notes.

## Critical anchors (do not violate)

- **Append-only ledgers** — `acceptance_events` and `consent_events` are history. Never edited, never overwritten. New row per change.
- **Versioned, never overwritten** — new legal text = new `document_versions` row. Old versions stay queryable forever.
- **M2M auth on `/v1/*`** — Zitadel opaque-token introspection. No browser sessions on `/v1/*`.
- **Browser auth allowed only on the lawyer dashboard** (when built) — Zitadel OAuth code flow, restricted role.
- **`/public/*` is hardened** — anonymous, GET-only, only documents flagged `is_publicly_visible=TRUE` (DB column, not env), filtered subprocessor projection. Never expose DPIA, Article 30 register, retention specifics, or any ledgers.
- **Boring stack** — Go + Chi + sqlc + Postgres + distroless. No Redis, no tracing, no SPA framework, no heavy dependencies.
- **Body-only document storage** — never store full website pages.

## Refuse these (push back hard)

- Adding non-legal-document data to `/public/*`.
- Letting acceptance or consent rows become mutable.
- Overwriting old document versions.
- Adding Redis / distributed tracing / heavy framework dependencies.
- Building an SPA dashboard. Use Go templates + htmx (or similar minimal stack).
- Pre-translating documents to languages for markets not yet launching.
- Hardcoding document keys, audience values, or locale lists in Go — these are data, not code.

## Local development

- **Docker only.** No local Go install assumed.
- Build + test:
  ```bash
  docker run --rm -v "$PWD":/src -w /src golang:1.26-alpine sh -c 'go build ./... && go test ./...'
  ```
- Run service: `docker compose up --build -d regula`
- Migrate: `docker compose exec regula /regula migrate`
- Seed: `docker compose exec regula /regula seed foundation`

## Naming caution

- **Regula** = this service (legal/GDPR evidence ledger).
- **`RegulatoryZone`** in `pillion-core` = city/region transport permits. Unrelated.
- **PrefactGuard** in `pillion-services/` = trust & safety (driver verification, fraud, ratings, behaviour checks). Different concern.

If a task mentions "regulatory" or "regulation," confirm which one before acting.

## Active direction (2026-04-26)

Two shifts in flight:

1. **Public visibility moves to a DB column** (`documents.is_publicly_visible`), replacing `REGULA_PUBLIC_LEGAL_KEYS` env. Lawyers self-serve, no redeploy per new public document.
2. **Regula gains its own admin dashboard** for the legal/compliance team. Reverses the prior "admin lives in core" stance. Goal: lawyers manage content + visibility + governance registry without engineering involvement ("gig layer" — Decision 10).

See `.claude/AGENT_MEMORY.md` Decisions 8, 9, 10 for full reasoning. Open design questions (markdown editor vs paste, side-by-side translation, approval workflow) deferred until dashboard work begins.