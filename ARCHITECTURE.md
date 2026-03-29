# Regula Architecture

## Goal
Provide an append-only compliance evidence service for Pillion and future microservices.

## Design Principles
- standalone service and database
- append-only events for evidence
- immutable document versions
- internal-only authenticated API
- minimal moving parts and low memory usage

## Core Domains
1. Documents
- legal/privacy/terms/consent texts
- versioned per locale and audience

2. Acceptances
- when a subject accepted a specific document version
- stores timestamp, source, and evidence hash

3. Consents
- purpose-based opt-in/opt-out ledger
- newsletter consent is just a purpose
- stores current state via latest event, not destructive updates

4. Evidence Bundles
- one internal endpoint to retrieve the current subject-facing legal evidence set

## Why PostgreSQL
- multiple services can write safely
- better long-term audit querying
- stronger concurrency and operational safety than SQLite for a shared internal service

## Why No Object Storage Yet
R2/S3 is not needed for the first phase because the service stores structured legal text and evidence events in PostgreSQL.
Object storage becomes useful later for signed PDF snapshots, archived bundles, or binary attachments.
