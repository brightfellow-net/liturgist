-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 2, slice 2B: readings (docs/reference/schema.md "Step 2 tables").

-- +goose Up

CREATE TABLE readings (
    id                TEXT NOT NULL CONSTRAINT readings_pkey PRIMARY KEY
                                    CONSTRAINT readings_id_check CHECK (length(id) = 26),
    church_id         TEXT NOT NULL CONSTRAINT readings_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    reference         TEXT NOT NULL,
    reference_display TEXT NOT NULL,
    translation_id    TEXT NOT NULL CONSTRAINT readings_translation_fkey REFERENCES translations (id),
    text              TEXT NOT NULL,
    attribution       TEXT NOT NULL,
    source_provider   TEXT NOT NULL,
    search_fold       TEXT NOT NULL,
    version           INTEGER NOT NULL CONSTRAINT readings_version_check CHECK (version >= 1),
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    CONSTRAINT readings_church_ref_key UNIQUE (church_id, reference, translation_id)
) STRICT;
CREATE INDEX readings_church_translation_idx ON readings (church_id, translation_id, updated_at);
