-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 4, slice 4A: the history of a liturgy's review states
-- (docs/impl/12-review.md 5, P-73).

-- +goose Up

CREATE TABLE liturgy_state_changes (
    id         text NOT NULL CONSTRAINT liturgy_state_changes_pkey PRIMARY KEY
                             CONSTRAINT liturgy_state_changes_id_check CHECK (length(id) = 26),
    church_id  text NOT NULL,
    liturgy_id text NOT NULL,
    from_state text NOT NULL CONSTRAINT liturgy_state_changes_from_check
                             CHECK (from_state IN ('draft', 'in_review', 'needs_revision', 'approved', 'published')),
    to_state   text NOT NULL CONSTRAINT liturgy_state_changes_to_check
                             CHECK (to_state IN ('draft', 'in_review', 'needs_revision', 'approved', 'published')),
    user_id    text NOT NULL CONSTRAINT liturgy_state_changes_user_fkey REFERENCES users (id) ON DELETE RESTRICT,
    note       text NOT NULL,
    edit_seq   integer NOT NULL CONSTRAINT liturgy_state_changes_seq_check CHECK (edit_seq >= 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT liturgy_state_changes_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE CASCADE,
    CONSTRAINT liturgy_state_changes_church_id_key UNIQUE (church_id, id)
);
CREATE INDEX liturgy_state_changes_church_liturgy_idx ON liturgy_state_changes (church_id, liturgy_id, created_at, id);
