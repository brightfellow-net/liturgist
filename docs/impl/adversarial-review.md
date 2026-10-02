# Adversarial Review — Implementation Documents Index

Reviewed document: [`README.md`](README.md)  
Review date: 2026-10-02

This review covers the index itself. It does not validate the full implementation requirements in the linked documents.

## Findings

### [CRITICAL] This is an index, not an executable specification

**Location:** Document type; sections 1 and 4

**Problem:** The README calls itself a “Reference” and says the linked documents define how to build each step. Handing only this file to an implementation agent leaves core behavior—data models, migrations, authorization, and API details—unspecified.

**Fix:** Do not use this README as the execution spec. Name the complete required document set in the handoff and require the agent to read it before implementation.

### [CRITICAL] No rule resolves conflicts across the linked documents

**Location:** Sections 1–3

**Problem:** The agent is directed to follow `SPEC.md`, five implementation documents, and a schema reference. The README says some proposals “amend” or “resolve” SPEC decisions, but gives no general precedence rule for conflicts, omissions, or stale cross-references. An agent can choose a different source as authoritative and implement incompatible behavior.

**Fix:** State which document takes precedence, how amendments override earlier decisions, and what the agent must do when two current documents still conflict.

### [HIGH] “All proposals approved” has no attributable approval record

**Location:** Section 2

**Problem:** The README asserts all proposals were approved on a date, but does not identify the approver or link to an approval record. The implementation agent cannot distinguish an authorized decision from an accidental status edit.

**Fix:** Identify the approving owner and link to a durable approval record, or label the statuses as unverified.

### [HIGH] Referenced content is not pinned to a reviewed version

**Location:** Sections 1 and 4

**Problem:** Links point to mutable paths and anchors. If a linked document changes after review, the README can still say “approved” while the implementation agent follows different requirements.

**Fix:** Record the commit or document revision that was reviewed and approved; require reapproval when an approved proposal’s substance changes.

### [HIGH] Setup links can be invalidated by ordinary restarts

**Location:** P-24

**Problem:** “Each start while not set up” replaces the setup token. A restart after the link is printed makes that link unusable, with no stated recovery path beyond starting again and obtaining another link.

**Fix:** Specify token rotation conditions, how an operator retrieves or regenerates a valid link, and how restart or crash recovery behaves.

### [HIGH] The CSRF summary leaves a security boundary undefined

**Location:** P-20

**Problem:** Requests with neither `Sec-Fetch-Site` nor `Origin` are allowed as non-browser clients, while unsafe requests with bodies must be JSON. The summary does not say whether bodyless unsafe requests are rejected, whether all mutating routes require a body, or how content type is validated. Those details decide whether the stated defense is actually enforced.

**Fix:** Define the exact checks for every unsafe request, including bodyless requests, accepted content types, and routes that can mutate state.

### [HIGH] Reset-password and emergency-admin CLI scope is underspecified

**Location:** P-22 and P-23

**Problem:** The reset command’s eligibility rules are described for admin-created resets, but the README does not say whether CLI resets use the same restrictions. The emergency grant command accepts an `<identifier>` without stating how duplicate or ambiguous identifiers are handled.

**Fix:** Specify the CLI authorization and eligibility rules, identifier matching and ambiguity behavior, and the exact audit record for each command.

### [HIGH] The migration override can bypass the stated downgrade protection

**Location:** P-11

**Problem:** `--allow-newer-schema` is described as an override “off by default,” but its effect and scope are absent here. It is unclear whether it permits startup only, migration writes, or schema-version changes that could damage data.

**Fix:** Define exactly what the override permits, what it never permits, and what warning or audit output it produces.

### [HIGH] The multi-tenant rule is only partially reconciled

**Location:** P-25

**Problem:** The README says the community resolver returns the one church and the server refuses to start with more than one. It does not say what happens to existing data, migrations, or deployments if a second church is introduced, or whether this restriction applies to all deployment modes.

**Fix:** State the restriction’s scope and the required migration or upgrade path before multi-church support can be enabled.

### [MEDIUM] “No lock-out” has no defined invariant or recovery behavior

**Location:** P-16

**Problem:** It says someone always holds both `roles.manage` and `members.manage`, but does not say whether this is checked transactionally for every role/member mutation or how concurrent edits are handled. Two administrators could each remove the other’s scopes based on stale state.

**Fix:** Specify the invariant, the transaction boundary, and the response when a concurrent change would violate it.

### [MEDIUM] Session replacement has unclear account-switch behavior

**Location:** P-19

**Problem:** Every login and several account flows create a new token and delete a session “the browser already had.” It is unclear whether this means the current cookie’s session only, all sessions for that user, or sessions associated with a different account.

**Fix:** Define which session rows are revoked for each flow, including login to a different account in the same browser.

### [MEDIUM] Throttling can penalize shared networks without defined recovery

**Location:** P-18

**Problem:** An IP-wide threshold can lock out legitimate users behind a shared network. The README gives no exception, unblock path, or behavior when client IP parsing fails or trusted proxy configuration is wrong.

**Fix:** Specify malformed/untrusted proxy handling, counter cleanup and operator recovery, and how shared-IP lockouts affect legitimate accounts.

### [MEDIUM] Cleanup timing and races are not addressed

**Location:** P-32

**Problem:** “Hourly cleanup” does not say whether expiration checks are authoritative at use time or whether a token remains valid until cleanup runs. Concurrent cleanup and token redemption could produce inconsistent results.

**Fix:** Require validity checks in the redemption transaction and define cleanup as storage maintenance only, with an explicit expiration clock basis.

### [MEDIUM] Generated-file reproducibility is asserted but not defined

**Location:** P-07

**Problem:** CI fails if regeneration changes committed OpenAPI or TypeScript files, but the README does not identify generator versions, invocation, or environment. Different tool versions can cause spurious diffs or hide real changes.

**Fix:** Pin generator versions and document the canonical generation command and environment.

### [MEDIUM] “Default UI language” and “content language” are not operationally distinguished

**Location:** P-24 and P-31

**Problem:** The setup wizard asks for both, but the README does not define whether content language applies per church, per service, or per user, nor how translations map to those settings.

**Fix:** Define the stored entities and selection/fallback rules for UI language, content language, and translation.

### [MEDIUM] The approval process has no exit condition for deferred HIGH findings

**Location:** Section 4

**Problem:** The process requires a fix/accept/defer decision for every HIGH finding, but does not say who may accept or defer one, what rationale is required, or whether deferred findings block implementation.

**Fix:** Name the decision owner, require a recorded rationale and follow-up for each deferment, and state whether implementation may proceed with deferred HIGH findings.
