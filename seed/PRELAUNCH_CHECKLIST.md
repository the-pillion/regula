# Regula Pre-Launch Checklist

**Status: REQUIRED.** Do not flip any document to `is_publicly_visible=true` + `is_published=true` until every box on this checklist is ticked. The legal documents under `seed/legal_documents/` are technical drafts by the product team. Each one carries an `aside.legal-draft-banner` at the top — that banner must remain visible until lawyer sign-off is recorded below.

This checklist is for the operator (founder + commercialista + lawyer). Tick items by editing this file in PRs.

---

## 1. Lawyer review

- [ ] All 12 document keys reviewed by qualified Italian counsel (lawyer name + date recorded here: __________).
- [ ] Counsel has cleared the marketplace framing (TOS general §2, TOS-passenger §1, TOS-driver §1).
- [ ] Counsel has cleared the NCC compliance schedule (`ncc-italia.it.html`).
- [ ] Counsel has cleared the consumer-forum clause (TOS-passenger §12 — art. 33 c. cons.).
- [ ] Counsel has cleared the Platform Work Directive disclosure (TOS-driver §10, TOS general §7).
- [ ] Counsel has cleared the automated-decisions disclosure (privacy §7, gdpr-rights §8, dpia-summary-automated-decisions).
- [ ] Counsel has cleared the DPA stack disclosure (processor-list).
- [ ] Counsel has cleared the cookie banner UX against Garante 10 June 2021 guidelines.
- [ ] `legal-draft-banner` removed from every document after sign-off.

## 2. Italian company metadata (file: `seed/company.base.json`)

These placeholders appear in published documents. Empty values render as empty strings and look unprofessional or worse.

- [ ] `vat_number` (P.IVA) — required in: TOS, TOS-passenger, TOS-driver, privacy-policy, cookie-policy, impressum, ncc-italia, processor-list.
- [ ] `tax_code` (Codice Fiscale of the company) — required in: impressum.
- [ ] `rea_number` (REA Camera di Commercio) — required in: TOS, privacy-policy, impressum.
- [ ] `share_capital` (e.g. "10.000 EUR i.v.") — required in: impressum.
- [ ] `chamber_of_commerce` (e.g. "Camera di Commercio di Bolzano") — already seeded, verify still correct.
- [ ] `registered_street` — full street + number — required in: TOS, privacy-policy, impressum.
- [ ] `registered_postal_code` — verify (default 39100 Bolzano).
- [ ] `registered_city` — verify (default Bolzano).
- [ ] `registered_province` — verify (default BZ).
- [ ] `registered_country` / `registered_country_code` — verify (default Italy / IT).
- [ ] `dpo_name` — full name of the DPO (leave empty only if no DPO is required; consult counsel — for a transport platform with geolocation + automated decisions, a DPO is effectively mandatory).
- [ ] `dpo_address` — required if DPO is required.
- [ ] `email_legal` — currently `legal@thepillion.com` — confirm inbox exists, is monitored, has auto-reply.
- [ ] `email_privacy` — currently `privacy@thepillion.com` — same.
- [ ] `email_dpo` — currently `dpo@thepillion.com` — same.
- [ ] `email_support` — currently `support@thepillion.com` — same.
- [ ] `phone` — operator phone for impressum.
- [ ] `website` — already `https://www.thepillion.com`. Verify domain resolves + has TLS.
- [ ] `last_updated` and `effective_date` bumped to the publication date.
- [ ] `version` set per document (use semantic numbering like `1.0`).

## 3. Driver-settlement-specific placeholders

- [ ] `platform_commission_percent` and `platform_commission_default` injected to match the seeded default in price-arbiter. Today price-arbiter defaults to **12.5%**; the worked example in `driver-settlement.{it,en,de}.html` uses 12.5% explicitly — verify the percent token resolves to the same number.

## 4. Supervisory authority placeholders

- [ ] `supervisory_authority` — currently `Garante per la protezione dei dati personali` — verify.
- [ ] `supervisory_authority_url` — currently `https://www.garanteprivacy.it` — verify.
- [ ] `odr_url` — currently `https://ec.europa.eu/consumers/odr` — verify.

## 5. Processor list freshness (`seed/foundation.json` + `processor-list.*.html`)

The `processor-list` document is an editorial snapshot of `foundation.json`. Re-check before publishing:

- [ ] Every processor row in `foundation.json` matches the table in `processor-list.{it,en,de}.html` (name, role, country, DPA status).
- [ ] Stripe DPA — signed and on file.
- [ ] Twilio DPA — signed and on file.
- [ ] Firebase / Google Cloud DPA — signed and on file.
- [ ] FastGeo DPA — signed (or operator-internal documented if same-owner intra-group).
- [ ] Zitadel DPA — signed.
- [ ] Hosting provider chosen + DPA signed.
- [ ] Background check provider chosen (Checkr / PrefactGuard route) + DPA signed.
- [ ] Sub-processor lists of each vendor reviewed and within risk tolerance.

## 6. DPIA finalisation

- [ ] Geolocation DPIA (`dpia_records` row) moved from `draft` to `completed`. Sign-off recorded.
- [ ] Automated-decisions DPIA moved from `draft` to `completed`. Sign-off recorded.
- [ ] `dpia-summary-geolocation.*.html` updated to match the final DPIA's mitigations.
- [ ] `dpia-summary-automated-decisions.*.html` updated to match the final DPIA's mitigations.
- [ ] DPIA full files stored where the supervisory authority can access them on request.

## 7. Locale parity

- [ ] `ls seed/legal_documents/*.html | awk -F. '{print $(NF-2)}' | sort | uniq -c` shows each key with count = 3 (one IT, EN, DE per key).
- [ ] Total file count is 36 (12 keys × 3 locales).
- [ ] Every EN and DE document carries the `legal-translation-note` footer ("Italian version prevails…").

## 8. Public visibility flags

- [ ] In the Regula dashboard, set `documents.is_publicly_visible = TRUE` only for the 12 keys you intend to expose.
- [ ] Confirm each key has a `document_versions` row with `is_published = TRUE` for at least one locale.
- [ ] Old (pre-rewrite) versions: leave them in the version table with `is_published = FALSE`. **Do not delete** — they are needed for audit of historical acceptances.

## 9. Public-site / dashboard rendering

- [ ] CSS rules added for new classes: `.legal-draft-banner`, `.legal-summary`, `.legal-honest`, `.legal-toc`, `.legal-translation-note`, `.legal-footer`. (Separate follow-up; the documents render readably without CSS but look bland.)
- [ ] `/public/legal/{key}.html` returns 200 for each of the 12 keys.
- [ ] `/public/legal/{key}.json` returns 200 with content + metadata.
- [ ] ETag changes vs prior versions on first hit (verify caching is invalidated correctly).
- [ ] Public site `pillion-fe-temp` updated to surface the 5 new doc keys in its footer / legal index.

## 10. Acceptance flows

- [ ] On the next login / next ride request, passengers see and accept the new `terms-of-service-passenger` version.
- [ ] On the next login / new ride accept, drivers see and accept the new `terms-of-service-driver`, `driver-settlement`, and `ncc-italia` versions.
- [ ] `acceptance_events` row written for each accept (verify via dashboard).
- [ ] Cookie banner re-prompts users for consent on the new `cookie-policy` version.

## 11. Communications

- [ ] Email all existing waitlist members 30 days before launch announcing the new terms.
- [ ] In-app notice scheduled for first login after publication.
- [ ] Support team briefed on the new section numbering for ticket references.

## 12. Counsel sign-off record

| Document key | Reviewer | Date | Notes |
|---|---|---|---|
| terms-of-service | | | |
| terms-of-service-passenger | | | |
| terms-of-service-driver | | | |
| privacy-policy | | | |
| cookie-policy | | | |
| impressum | | | |
| gdpr-rights | | | |
| processor-list | | | |
| dpia-summary-geolocation | | | |
| dpia-summary-automated-decisions | | | |
| driver-settlement | | | |
| ncc-italia | | | |

---

## Placeholder inventory (every token used across the 36 files)

| Token | Where it lives |
|---|---|
| `{{brand_name}}` | `company.base.json` |
| `{{chamber_of_commerce}}` | `company.base.json` |
| `{{dpo_name}}` | `company.base.json` |
| `{{effective_date}}` | `company.base.json` (or per-doc override) |
| `{{email_dpo}}` | `company.base.json` |
| `{{email_legal}}` | `company.base.json` |
| `{{email_privacy}}` | `company.base.json` |
| `{{email_support}}` | `company.base.json` |
| `{{last_updated}}` | `company.base.json` (or per-doc override) |
| `{{legal_form}}` | `company.base.json` |
| `{{legal_name}}` | `company.base.json` |
| `{{odr_url}}` | `company.base.json` |
| `{{phone}}` | `company.base.json` |
| `{{platform_commission_default}}` | per-doc default (driver-settlement) |
| `{{platform_commission_percent}}` | per-doc default (driver-settlement) |
| `{{rea_number}}` | `company.base.json` |
| `{{registered_city}}` | `company.base.json` |
| `{{registered_country}}` | `company.base.json` |
| `{{registered_postal_code}}` | `company.base.json` |
| `{{registered_province}}` | `company.base.json` |
| `{{registered_street}}` | `company.base.json` |
| `{{share_capital}}` | `company.base.json` |
| `{{supervisory_authority}}` | `company.base.json` |
| `{{supervisory_authority_url}}` | `company.base.json` |
| `{{tax_code}}` | `company.base.json` |
| `{{vat_number}}` | `company.base.json` |
| `{{version}}` | per-document version slug |
| `{{website}}` | `company.base.json` |

Sanity grep before publishing:

```bash
cd pillion-services/regula
grep -ho '{{[a-z_]*}}' seed/legal_documents/*.html | sort -u
```

Should match the table above. New tokens introduced by future edits go straight here.
