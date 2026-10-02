# 05 — Web App Shell (Implementation)

> **Document type: Implementation.** Step 1 of [SPEC.md §10](../SPEC.md#10-suggested-build-order).
> Status: **Draft**. Items marked **[P-xx]** are proposals awaiting approval ([index](README.md#3-proposed-decisions)).

## 1. Scope

The React single-page app for step 1: workspace layout, pages, routing, API client, translations, and the accessibility basics every later page builds on. Libraries are fixed by the decisions log ([SPEC.md §11](../SPEC.md#11-decisions-log), row "Frontend libraries").

## 2. Pages in step 1

**[P-28]**

| Route | Page | Who | API used |
|---|---|---|---|
| `/setup#t=…` | Setup wizard: church, first admin, UI language, content language, translation, time zone, key display | Anyone with the setup link | `GET /setup/status`, `GET /translations`, `POST /setup` |
| `/login` | "Email or phone number" + password; `?next=` return path; "Forgot password?" opens the text "Ask your church admin to send you a reset link." | Logged out | `POST /auth/login` |
| `/invite#t=…` | Accept invite: new account (name, email and phone pre-filled from the invite and editable, password), or "log in to accept" / "join with my current account"; always ends with "You are joining {church} as {name}" | Invitee | `POST /invites/inspect`, `/invites/accept`, `/invites/accept-existing` |
| `/reset#t=…` | Shows "This link was created by {name}", then set a new password | Holder of a reset link | `POST /auth/reset/inspect`, `POST /auth/reset` |
| `/` | Home: welcome text; placeholder for "My assignments" (step 5) | Members | `GET /me` |
| `/profile` | Name, text size, UI language, change password, "Log out on all other devices" (for a lost phone) | Logged in | `PATCH /me`, `POST /me/password`, `POST /me/sessions/end-others` |
| `/settings/church` | Church settings | View: all members; edit: per `actions` (`church.settings`) | `GET/PATCH /church`, `GET /translations` |
| `/settings/members` | Members (assign roles, remove, reset link) and invites (create with roles, copy/share link, regenerate, cancel), usage "9 of 12" when limited | Members with `members.view` (actions per `actions`) | `/members`, `/invites`, `/roles` |
| `/settings/roles` | Role editor: list roles with member counts; create, rename, edit description, tick scopes (each with a plain-language description); delete with confirmation showing how many members hold it | Members with `roles.manage` | `/roles`, `/scopes` |
| `/privacy` | Privacy notice: what is stored (names, phone numbers, emails; IP addresses only briefly, for blocking password guessing, deleted within about an hour), why, who can see it, plus the church's contact text | Anyone | `GET /church` when logged in; static text otherwise |
| `*` | Not found | Anyone | — |

Navigation: people with no roles (team members) see **Home** and **Profile** (Liturgies arrive in step 5); members holding any of `church.settings`, `members.view`, `members.manage`, `roles.manage` also see **Settings**. The menu is driven by `membership.scopes` from `GET /me`.

## 3. Workspace layout

```
package.json                pnpm workspace root (packageManager pinned)
pnpm-workspace.yaml         web, packages/*
web/
  src/main.tsx              router, QueryClient, i18n init
  src/routes/…              one file per page above
  src/components/ui/…       shadcn/ui components (copied into the repo)
  src/lib/api.ts            openapi-fetch client from packages/api-client
  src/lib/errors.ts         problem-details → translated message by `code`
  src/lib/fragmentToken.ts  read `#t=` once, then remove it from the address bar
  vite.config.ts            dev proxy /api → http://localhost:8080; PWA plugin added in step 5
packages/api-client/        openapi.json, src/schema.d.ts (generated), src/index.ts
packages/i18n/              en.json (source), id.json
```

## 4. API client and data

- `createClient<paths>({ baseUrl: "" })` from `openapi-fetch`, typed by the generated `schema.d.ts`.
- All server data goes through TanStack Query. Query keys: `["me"]`, `["church"]`, `["members"]`, `["invites"]`, `["translations"]`.
- On a 401 `unauthenticated` from any query: clear the cache and go to `/login?next=<current path>`.
- On 409 `not_set_up`: go to `/setup`.
- Forms: React Hook Form + Zod. Zod schemas mirror the server rules (lengths, required fields) for instant feedback only; the server's 422 `errors` are mapped onto fields and always win.
- Buttons and menu items are shown or hidden from `actions` in API responses only.

## 5. Links and routing

- React Router. Internal links use route constants from `src/routes/paths.ts`; no string-built paths elsewhere.
- Links received from the API (invite and reset links) are shown and shared as received.
- Tokens: `fragmentToken.ts` reads `t` from `location.hash`, then calls `history.replaceState` to remove the fragment, so the token doesn't stay in the address bar or history.
- If `/invite`, `/reset` or `/setup` opens without a `#t=` fragment (e.g. a mail scanner or a copy-paste dropped it), the page shows "This link is incomplete. Ask your church admin for a new link." (for `/setup`: "Open the setup link printed in the server log.") and makes no API call.
- "Share to WhatsApp" for invite links uses `https://wa.me/?text=<encoded message with link>`; "Copy" uses the Clipboard API with a fallback text field.

## 6. Translations

- i18next with `packages/i18n/en.json` as the source and `id.json` as the second language ([SPEC.md §8](../SPEC.md#8-non-functional-requirements), UI language).
- Language chosen in this order: `me.user.preferences.ui_language` → `me.church.default_ui_language` → `en`. Before login (login, setup, invite pages): the browser language if it starts with `id`, else `en`; the page has a language switch.
- Keys are dotted and grouped by page: `login.title`, `errors.invalid_credentials`, …. Every API error `code` in [01 §10](01-foundation.md#10-error-format-and-codes) has an `errors.<code>` key.
- A unit test fails if `id.json` lacks any key from `en.json`, or has keys `en.json` doesn't.
- Dates and times: `Intl.DateTimeFormat` with the church's `time_zone`.

## 7. Accessibility and text size

Following [SPEC.md §5.8](../SPEC.md#58-accessibility-and-older-volunteers):

- All sizes in `rem`; the root font size is 100%, 112.5% or 125% for text size normal / large / larger (`User.preferences.text_size`).
- Tap targets ≥ 48 px; every button has visible text.
- Every password field has a "Show password" toggle button (text label, not only an eye icon).
- Dark mode via `prefers-color-scheme`.
- Playwright tests run axe on every step-1 page and fail on any WCAG 2.2 A or AA violation.

## 8. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Decide permissions in the UI (e.g. `if role.name === "Church admin"`) | Use `actions` (and `scopes` only for menu visibility) from the API | Rules live only in Go; churches rename roles |
| Hand-write API types or `fetch` calls | Generated types + `openapi-fetch` | Types stay in sync with the server |
| Hard-code UI text in components | i18next keys in `en.json` / `id.json` | English-first i18n from day one |
| Show raw server `detail` text to users | Translate by `code` | `detail` is English and technical |
| Leave `#t=` tokens in the address bar | Remove with `replaceState` after reading | Shoulder-surfing, history, screenshots |
| Use `px` for text or fixed heights for text containers | `rem`, flexible layout | Must work at 200% text size |
| Store session data in `localStorage` | Rely on the HttpOnly cookie | XSS cannot steal an HttpOnly cookie |

## 9. Test case specifications

### Unit tests (Vitest + Testing Library)

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-W-001 | `fragmentToken` | `location.hash = "#t=abc"` | Returns `abc`; hash removed | No hash → `null`, and the invite page shows the "incomplete link" message without calling the API |
| TC-W-002 | Error mapping | Problem `{code:"too_many_attempts"}` with `Retry-After: 900` | Translated text mentioning 15 minutes | Unknown code → generic error text |
| TC-W-003 | i18n completeness | `en.json`, `id.json` | Same key sets | Nested keys |
| TC-W-004 | Members page | `actions.remove=false` | No remove button | All actions false → read-only list |
| TC-W-005 | Language choice | User pref `null`, church default `id` | Indonesian UI | Logged out, browser `id-ID` → Indonesian |

### End-to-end tests (Playwright against `liturgist serve` with a temp data dir)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| E2E-W-001 | First-time setup | Fresh server; read setup link from log | Wizard completes; lands on Home as church admin | Stop server |
| E2E-W-002 | Invite and accept | Admin logged in | Create invite → open link in a new context → set password → Home as team member with reduced navigation | — |
| E2E-W-006 | Role editor | Admin logged in | Create role "Multimedia" with `members.view`; assign to a member; that member now sees Settings → Members but not Roles | — |
| E2E-W-003 | Login, throttle message | User exists | Wrong password 6× → "too many attempts" message | — |
| E2E-W-004 | Reset link | Admin creates reset link | Member sets new password and is logged in; old password fails | — |
| E2E-W-005 | Accessibility | Each step-1 page | axe reports no A/AA violations | — |

## 10. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `unauthenticated` | 401 | "Please log in again." | Redirect to `/login?next=…` |
| `not_set_up` | 409 | — | Redirect to `/setup` |
| Missing `#t=` fragment | Client-side, before any API call | "This link is incomplete. Ask your church admin for a new link." | Link to login |
| `invalid_token` | 400 + `reason` | expired: "This link has expired. Ask your church admin for a new one." (similar for used/cancelled/unknown) | Link to login |
| `limit_reached` | 403 | "Your church has reached its limit of {max} team members." | Show usage; remove a member or cancel an invite |
| `forbidden` | 403 | "You don't have permission to do this." | Stay on page |
| `not_found` on church endpoints with `me.membership == null` | 404 | "You are no longer a member of this church." | Log out button |
| Network error | `fetch` rejects | "Can't reach the server. Check your connection." | Retry button; TanStack Query retries reads 2× |
| `unavailable` | 503 | "The server is busy. Please try again." | Retry button |

## 11. References

| Topic | Location |
|---|---|
| Frontend libraries (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — row "Frontend libraries" |
| UI language | [SPEC.md §8](../SPEC.md#8-non-functional-requirements) |
| Accessibility | [SPEC.md §5.8](../SPEC.md#58-accessibility-and-older-volunteers) |
| Setup wizard | [SPEC.md §5.7](../SPEC.md#57-first-time-experience), [03 §10](03-identity-auth.md#10-first-time-setup) |
| API operations | [03-identity-auth.md](03-identity-auth.md), [04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api) |
| Error codes | [01 §10](01-foundation.md#10-error-format-and-codes) |
