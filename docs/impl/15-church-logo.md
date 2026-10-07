# 15 — Church Logo (Implementation)

> **Document type: Implementation.** A church admin uploads a logo; it appears to the left of the church name in the app header, the published view and the print header. Deferred by [Q-5.1](README.md#4g-questions-for-the-owner-step-5) until after the first pilot week; the owner asked for it on 2026-10-07.
> Status: decisions **[L-1] to [L-8] Approved** 2026-10-07 ([L-1] and [L-2] were the owner's own choices; the rest as proposed). Spec Gate and adversarial review have not run; slices are drafted one at a time and each waits for the owner's approval before merging. Nothing is built.

## 1. Scope and what already exists

Checked against the code on 2026-10-07 (`main` at `3709621`).

| Item | State | Where |
|---|---|---|
| `Storage` port (`Put`, `Open`, `Delete`; keys `[a-z0-9/_.-]`) and the local-folder adapter | **Built**, nothing writes to it yet | `app/extensions.go:35`, `adapters/storage/localfs`, `server/server.go:45` |
| `files/` in backups and restore | **Built** (14 §3) | `internal/backup/archive.go`, `server/backup.go` |
| `churches.settings` JSON column (unknown keys preserved) | **Built**; holds `key_display`, `feedback_url`, `privacy_contact`, print options | `domain/church.go`, schema.md |
| Church settings page, `church.settings` scope, `GET/PATCH /church` | **Built** | `web/src/routes/settings/ChurchSettingsPage.tsx`, `adapters/httpapi/church_ops.go` |
| Where the name is shown | App header `web/src/routes/AppLayout.tsx:96` (from `/me`), published view `PublishedBody.tsx:25`, print `PrintBody.tsx:27` (both from the copy's `church_name`) | |
| JSON-only unsafe requests (CSRF layer 3) | **Built**: `POST/PUT/PATCH/DELETE` must be `application/json` | `adapters/httpapi/csrf.go` |
| Upload route, image checks, serving route | **Missing** | — |

| Left out | Why |
|---|---|
| SVG logos | Can carry scripts; needs a sanitiser ([L-2]) |
| The logo on the login and setup pages | Needs a route that works without a session; the owner chose the three places of [L-1] |
| The logo inside the published copy | A copy is a snapshot; an image per version would grow every backup and change the copy format ([L-5]) |
| Cropping or editing in the browser | The admin prepares the image; the server only shrinks it |
| Favicon, app icons, e-mail headers | Not requested |

## 2. Decisions

| ID | Proposal | Status |
|---|---|---|
| L-1 | The logo is shown to the left of the church name in the app header, the published view and the print header. Not on login or setup | **Approved** 2026-10-07 (owner's choice) |
| L-2 | Upload PNG, JPEG or WebP, at most 2 MiB. The server checks the real type, reads the size before decoding (at most 4096 px a side and 16 million pixels), scales down to fit 512 × 512 (never enlarges) and stores a PNG. Camera metadata and anything hidden in the file is dropped. No SVG | **Approved** 2026-10-07 (owner's choice) |
| L-3 | **Storage.** File key `church/<church id lower-cased>/logo-<version>.png` (ULIDs are upper-case, keys must not be). `version` is the first 16 hex characters of the SHA-256 of the stored PNG. The row's `settings` JSON gets `logo: {version, width, height}`; **no migration**. Replace = `Put` the new file, update the setting in a transaction, then delete the old file (best effort). A failure leaves an unused file, never a setting that points at nothing. Remove = clear the setting, then delete the file | **Approved** 2026-10-07 |
| L-4 | **API** (church-scoped). `PUT /church/logo` body `{"image": "<base64>"}` (JSON because of the CSRF rule; body limit 3 MiB), scope `church.settings`, returns the church. `DELETE /church/logo`, same scope, returns the church. `GET /church/logo`, any member: the PNG with `ETag: "<version>"`, `Cache-Control: private, max-age=31536000, immutable` when `?v=<version>` matches the current one and `private, no-cache` otherwise; 404 `not_found` when there is no logo. `ChurchView` gets `logo_url` (`null` or `/api/v1/church/logo?v=<version>`). `/me`'s church carries it too | **Approved** 2026-10-07 |
| L-5 | **Display.** The logo is **not** part of a published copy; every view shows the church's *current* logo, while the name stays as published. An `<img>` with `alt=""` (the name is next to it), at most 32 px high in the app header and 14 mm in the print header, `object-contain`, hidden when it fails to load (offline, file lost). A church renamed after publishing shows the old name beside the new logo on old copies; accepted | **Approved** 2026-10-07 |
| L-6 | **Settings page.** A "Logo" section on the church settings page, shown with `actions.edit`: the current logo, "Choose image" (`accept` PNG, JPEG, WebP), a size check in the browser before sending, "Remove". Strings in `en` and `id` | **Approved** 2026-10-07 |
| L-7 | **Layering.** `app` may import only the standard library and `domain` (`.golangci.yml`), so the image work is a port, `app.ImageNormalizer`, implemented in `adapters/images` with `golang.org/x/image` (WebP decoding, `draw.CatmullRom`; BSD-3, like `x/text`, already used). `app.ChurchLogo` holds the use cases and takes `Storage`, `ImageNormalizer`, the `Tx` and the clock | **Approved** 2026-10-07 |
| L-8 | **Known limits.** A JPEG's EXIF rotation is not applied (the standard library does not read it), so a phone photo may come out sideways; animated WebP and GIF are refused; offline the header shows the name only; a restored backup brings the logo back with the database, because `files/` is in the archive. Checksums for `files/` are still missing (14 §3.1) | **Approved** 2026-10-07 |

## 3. Slices

| Slice | Contents |
|---|---|
| 15A | `ImageNormalizer` and its adapter; `ChurchLogo` use cases; `PUT/GET/DELETE /church/logo`; `logo_url` in the church and `/me` views; OpenAPI regenerated |
| 15B | The settings section; the logo in the app header, published view and print header; i18n; docs (`docs/self-host/`: where the file lives, backups) |

Each slice is drafted in its own worktree and waits for the owner's approval before merging, as before.

## 4. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Store the uploaded bytes as they came | Decode, shrink and re-encode as PNG | Metadata (GPS), polyglot files and huge images stay out |
| Decode before checking the size | `DecodeConfig` first; refuse over 4096 px a side or 16 million pixels | A small file can expand to gigabytes (decompression bomb) |
| Trust the file name or `Content-Type` of the upload | Sniff the decoded format | Both are chosen by the sender |
| Put the church ID in the key as it is | Lower-case it; use only `[a-z0-9/_.-]` | `Storage` refuses other keys (`ErrInvalid`) |
| Overwrite `logo.png` in place | A new key per version, then switch the setting | A failed write must not leave a broken logo; a changed logo needs a new URL for caches |
| Serve the file without a version in the URL and with a year of caching | `?v=<version>` plus `immutable` | Browsers would keep an old logo |
| Accept SVG | PNG, JPEG, WebP only | Scripts in SVG |
| Make the logo route public | Members only | The logo is church data; the login page is not in scope |
| Let the PUT take any content type | JSON with base64, as the CSRF layer requires | A raw image body would be rejected by layer 3, and loosening it is a security decision of its own |

## 5. Error handling

| Case | Detection | Response | Notes |
|---|---|---|---|
| Not valid base64, or empty | Huma decoding / length | 422 `invalid_input` on `image` | |
| Body over 3 MiB | Huma body limit | 413 | |
| Decoded file over 2 MiB | App check | 422 `invalid_input` "Use an image under 2 MB." | Size limit counts the decoded bytes |
| Not PNG, JPEG or WebP, or cannot be decoded (corrupt, animated) | `ImageNormalizer` | 422 `invalid_input` "Use a PNG, JPEG or WebP image." | |
| More than 4096 px a side or 16 million pixels | `DecodeConfig` | 422 `invalid_input` "The image is too large." | Before decoding |
| No `church.settings` scope | Existing scope check | 403 `forbidden` | |
| Disk full on `Put` | `ErrStorageFull` mapping (14 §4) | 507 `storage_full` | The setting is not changed |
| `Put` fails, other | Error | 503 `unavailable`, logged | The setting is not changed |
| Setting updated, old file not deleted | `Delete` error | Logged at WARN, request succeeds | An unused file stays; harmless |
| `GET` with no logo, or the file is missing | `ErrNotFound` | 404 `not_found` | The page hides the image |
| `DELETE` with no logo | | 200, the church unchanged | Idempotent |

## 6. Test cases

| ID | Test | Expect |
|---|---|---|
| TC-614 | `images.Normalize` (PNG, JPEG, WebP fixtures) | Result is a PNG, at most 512 px on the long side, aspect ratio kept, transparency kept, a small image is not enlarged |
| TC-615 | Normalize, bad input | Text file, truncated PNG, animated WebP, GIF, SVG, a 5000 × 5000 image and a 100-million-pixel header are refused without allocating the full image |
| TC-616 | Normalize, metadata | A JPEG with EXIF GPS comes out without it |
| TC-617 | `ChurchLogo` use cases on SQLite and PostgreSQL | Set, replace (old file deleted, new key, new version), remove, remove twice, permission, the setting survives `PATCH /church` of other fields |
| TC-618 | `ChurchLogo` with a failing `Storage` | `Put` fails: setting unchanged; `Delete` fails: success and a WARN |
| TC-619 | Key | The key of a ULID church ID is lower-case and accepted by `localfs.ValidKey` |
| IT-607 | HTTP: PUT, GET with `?v`, GET with an old `v`, DELETE | Status codes, `ETag`, cache headers; PNG signature; `nosniff`; 401 without a session; CSRF still demands JSON |
| IT-608 | Backup and restore | A logo set before `backup` is served after `restore` |
| WT-L-001 | Settings page | The section shows only with edit rights; a 3 MB file is refused before sending; upload shows the new logo; remove returns to name only |
| WT-L-002 | Header, published view, print | The image sits left of the name, has empty `alt`, disappears on load error |
| E2E-W-022 | Upload, reload, remove (Playwright) | The logo appears in the header and in the print view, then goes |

Mutation checks to run when built: skip the size check, skip `DecodeConfig`, skip the re-encode, write before the setting, forget to delete the old file, drop the scope check.

## 7. References

| Topic | Location |
|---|---|
| `Storage` port and the local adapter | [04 §7](04-tenancy-extensions.md), `app/extensions.go` |
| JSON-only unsafe requests | [03 §6](03-identity-auth.md#6-csrf-protection) |
| Church settings and print settings | [04 §6](04-tenancy-extensions.md), [13 §6](13-publishing.md) |
| `files/` in the backup | [14 §3](14-self-host.md) |
| Deferral decision | [Q-5.1](README.md#4g-questions-for-the-owner-step-5), [13 §1](13-publishing.md#1-scope) |
