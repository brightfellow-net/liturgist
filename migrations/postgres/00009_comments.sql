-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 4, slice 4B: comments on liturgies and their items
-- (docs/impl/12-review.md 4, 5, P-72, P-73).

-- +goose Up

-- item_id has no foreign key on purpose: a comment outlives its removed item,
-- and undoing the removal brings the same id back (12 §4).
CREATE TABLE liturgy_comments (
    id          text NOT NULL CONSTRAINT liturgy_comments_pkey PRIMARY KEY
                              CONSTRAINT liturgy_comments_id_check CHECK (length(id) = 26),
    church_id   text NOT NULL,
    liturgy_id  text NOT NULL,
    item_id     text,
    item_title  text NOT NULL,
    author_id   text NOT NULL CONSTRAINT liturgy_comments_author_fkey REFERENCES users (id) ON DELETE RESTRICT,
    body        text NOT NULL,
    resolved_at timestamptz,
    resolved_by text CONSTRAINT liturgy_comments_resolved_by_fkey REFERENCES users (id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL,
    CONSTRAINT liturgy_comments_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE CASCADE,
    CONSTRAINT liturgy_comments_resolved_check CHECK ((resolved_at IS NULL) = (resolved_by IS NULL)),
    CONSTRAINT liturgy_comments_church_id_key UNIQUE (church_id, id)
);
CREATE INDEX liturgy_comments_church_liturgy_idx ON liturgy_comments (church_id, liturgy_id, created_at, id);
CREATE INDEX liturgy_comments_church_open_idx ON liturgy_comments (church_id, liturgy_id) WHERE resolved_at IS NULL;
