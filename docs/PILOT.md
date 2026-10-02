# Liturgist — Pilot Plan (GKY Citragarden)

> Project plan for the first pilot. Product requirements live in [SPEC.md](SPEC.md); this file covers how the pilot is run.

## 1. Goal

GKY Citragarden prepares, reviews and distributes its liturgies in Liturgist instead of Word, and it saves them real time.

## 2. Baseline (measure before the pilot)

Ask the administrator and the liturgist:

- hours spent per week on the liturgy;
- review rounds per week;
- copying errors found after printing;
- how long the multimedia team spends rebuilding the content as slides.

## 3. Success criteria

| Measure | Target |
|---|---|
| Liturgies prepared entirely in the app | Every pilot service, for at least 4 weeks in a row |
| Administrator's weekly time | At least halved |
| Review rounds and copying errors | Clearly fewer than the baseline |
| Team members who accepted their invite and open their assignments | At least 80% |
| Liturgist and administrator would rather keep the app than go back | Yes |

## 4. Prerequisites

- Build steps 1–5 complete (SPEC §10).
- GKY's current liturgy document received and the print layout and starter template based on it (SPEC §9, decision 3).
- EasyWorship import working with GKY's version (6/7, to be confirmed), or the OpenLP route (SPEC §5.3).
- WhatsApp messages, reading mode and the print view working (SPEC §5.6).
- Backup restore tested once on the pilot instance.

## 5. Hosting

- The owner hosts the pilot: the community edition on a small VPS, with daily backups plus off-site copies.
- A short agreement with the church and the in-app privacy notice cover the personal data held (UU PDP).
- The church owns its data and can download a backup at any time.
- After the pilot, offer to move them to self-hosting (restore one backup file) or to the hosted SaaS.

## 6. Scope and timeline (about 8 weeks)

Start with **one Indonesian Sunday service**; add the other services, including Mandarin, once it runs smoothly.

| Weeks | Phase |
|---|---|
| 0 | **Setup:** import songs from EasyWorship together with the administrator; build the template from GKY's document; set up services, roles and translations; invite the team. **Training:** about 1 hour for the liturgist and administrator, plus a short Indonesian guide with screenshots for team members, shared on WhatsApp. Ask the administrator and liturgist for the baseline figures. |
| 1–2 | **Parallel run:** the app is used, and the old Word document is still produced as a backup. Compare the two. |
| 3–6 | **App first** for the first service; add other services when ready. The old way remains only as an emergency fallback. |
| 7–8 | **Wrap-up:** interviews, survey, decision. |

## 7. Feedback

- A WhatsApp group "Liturgist Pilot" with the liturgist, administrator and a multimedia team member, for questions and bugs.
- The in-app "Kirim masukan" (send feedback) link, pointing to that group or a simple form.
- A weekly 15-minute check-in with the liturgist and administrator.
- At the end: interviews with the liturgist, administrator and multimedia team (before any work on slides starts), and a 5-question survey for team members.
- A usability test with 2–3 older volunteers: find your part in Sunday's liturgy and read it out loud.

## 8. Data and privacy

- No analytics or tracking scripts. Usage figures come from the app's own database: liturgies created and published, accepted invites, `last_seen_at` per user.
- The in-app privacy notice explains what is stored, why, and who can see it.

## 9. Risk rules

- No updates to the pilot instance from Friday to Sunday.
- A named contact person (the owner) reachable on weekends in case something breaks before a service.
- Daily backups plus off-site copies; restore tested before week 1.
- The old Word process stays usable until the end of week 2.

## 10. Outcome

At the end of week 8, decide with the church:

- **Continue:** keep using the app; plan the move to self-hosting or the SaaS.
- **Iterate:** continue with fixes for the main problems found, and review again after 4 weeks.
- **Stop:** record what didn't work and why.

Then review the feedback from the liturgist, administrator and multimedia team before starting work on slides.
