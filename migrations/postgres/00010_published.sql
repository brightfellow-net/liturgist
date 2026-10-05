-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 5, slice 5A: published versions of liturgies
-- (docs/impl/13-publishing.md 3, 9, P-76, P-84).

-- +goose Up

-- NO ACTION, not CASCADE or RESTRICT: a liturgy that has a version cannot be
-- deleted (13 §2), while deleting a whole church still removes both tables'
-- rows in one statement.
CREATE TABLE published_versions (
    id           text NOT NULL CONSTRAINT published_versions_pkey PRIMARY KEY
                               CONSTRAINT published_versions_id_check CHECK (length(id) = 26),
    church_id    text NOT NULL,
    liturgy_id   text NOT NULL,
    number       integer NOT NULL CONSTRAINT published_versions_number_check CHECK (number >= 1),
    content      text NOT NULL,
    published_by text NOT NULL CONSTRAINT published_versions_user_fkey REFERENCES users (id) ON DELETE RESTRICT,
    published_at timestamptz NOT NULL,
    CONSTRAINT published_versions_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE NO ACTION,
    CONSTRAINT published_versions_liturgy_number_key UNIQUE (church_id, liturgy_id, number),
    CONSTRAINT published_versions_church_id_key UNIQUE (church_id, id)
);

CREATE TABLE published_assignees (
    church_id  text NOT NULL,
    version_id text NOT NULL,
    user_id    text NOT NULL CONSTRAINT published_assignees_user_fkey REFERENCES users (id) ON DELETE RESTRICT,
    CONSTRAINT published_assignees_pkey PRIMARY KEY (church_id, version_id, user_id),
    CONSTRAINT published_assignees_version_fkey FOREIGN KEY (church_id, version_id)
        REFERENCES published_versions (church_id, id) ON DELETE CASCADE
);
CREATE INDEX published_assignees_church_user_idx ON published_assignees (church_id, user_id, version_id);
