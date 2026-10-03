-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 2, slice 2C: song import batches (docs/reference/schema.md "Step 2 tables").

-- +goose Up

CREATE TABLE import_batches (
    id            text NOT NULL CONSTRAINT import_batches_pkey PRIMARY KEY
                                CONSTRAINT import_batches_id_check CHECK (length(id) = 26),
    church_id     text NOT NULL CONSTRAINT import_batches_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    source_format text NOT NULL CONSTRAINT import_batches_format_check
                                CHECK (source_format IN ('paste', 'openlyrics', 'chordpro', 'easyworship', 'pptx')),
    status        text NOT NULL CONSTRAINT import_batches_status_check CHECK (status IN ('open', 'closed')),
    created_by    text NOT NULL CONSTRAINT import_batches_user_fkey REFERENCES users (id),
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT import_batches_church_id_key UNIQUE (church_id, id)
);
CREATE INDEX import_batches_church_status_idx ON import_batches (church_id, status, updated_at);

CREATE TABLE import_candidates (
    id                   text NOT NULL CONSTRAINT import_candidates_pkey PRIMARY KEY
                                       CONSTRAINT import_candidates_id_check CHECK (length(id) = 26),
    church_id            text NOT NULL,
    batch_id             text NOT NULL,
    position             integer NOT NULL CONSTRAINT import_candidates_position_check CHECK (position >= 0),
    kind                 text NOT NULL CONSTRAINT import_candidates_kind_check CHECK (kind IN ('song', 'reading')),
    draft                jsonb NOT NULL CONSTRAINT import_candidates_draft_check CHECK (jsonb_typeof(draft) = 'object'),
    duplicate_of_id      text,
    decision             text NOT NULL CONSTRAINT import_candidates_decision_check
                                       CHECK (decision IN ('pending', 'accept', 'merge', 'skip')),
    merge_into           text,
    merge_target_version integer,
    remove_unmatched     boolean NOT NULL,
    warnings             jsonb NOT NULL CONSTRAINT import_candidates_warnings_check CHECK (jsonb_typeof(warnings) = 'array'),
    outcome              text CONSTRAINT import_candidates_outcome_check CHECK (outcome IS NULL OR outcome IN ('applied', 'failed')),
    applied_song_id      text,
    error_code           text,
    CONSTRAINT import_candidates_batch_fkey FOREIGN KEY (church_id, batch_id)
        REFERENCES import_batches (church_id, id) ON DELETE CASCADE,
    CONSTRAINT import_candidates_merge_check CHECK ((decision = 'merge') = (merge_into IS NOT NULL)),
    CONSTRAINT import_candidates_merge_version_check CHECK ((decision = 'merge') = (merge_target_version IS NOT NULL)),
    CONSTRAINT import_candidates_outcome_decision_check CHECK (outcome IS NULL OR decision IN ('accept', 'merge')),
    CONSTRAINT import_candidates_applied_check CHECK ((COALESCE(outcome, '') = 'applied') = (applied_song_id IS NOT NULL)),
    CONSTRAINT import_candidates_failed_check CHECK ((COALESCE(outcome, '') = 'failed') = (error_code IS NOT NULL))
);
CREATE INDEX import_candidates_church_batch_idx ON import_candidates (church_id, batch_id, position);
