-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 4, slice 4A: the history of a liturgy's review states
-- (docs/impl/12-review.md 5, P-73).

-- +goose Up

CREATE TABLE liturgy_state_changes (
    id         TEXT NOT NULL CONSTRAINT liturgy_state_changes_pkey PRIMARY KEY
                             CONSTRAINT liturgy_state_changes_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL,
    liturgy_id TEXT NOT NULL,
    from_state TEXT NOT NULL CONSTRAINT liturgy_state_changes_from_check
                             CHECK (from_state IN ('draft', 'in_review', 'needs_revision', 'approved', 'published')),
    to_state   TEXT NOT NULL CONSTRAINT liturgy_state_changes_to_check
                             CHECK (to_state IN ('draft', 'in_review', 'needs_revision', 'approved', 'published')),
    user_id    TEXT NOT NULL CONSTRAINT liturgy_state_changes_user_fkey REFERENCES users (id) ON DELETE RESTRICT,
    note       TEXT NOT NULL,
    edit_seq   INTEGER NOT NULL CONSTRAINT liturgy_state_changes_seq_check CHECK (edit_seq >= 0),
    created_at TEXT NOT NULL,
    CONSTRAINT liturgy_state_changes_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE CASCADE,
    CONSTRAINT liturgy_state_changes_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX liturgy_state_changes_church_liturgy_idx ON liturgy_state_changes (church_id, liturgy_id, created_at, id);
