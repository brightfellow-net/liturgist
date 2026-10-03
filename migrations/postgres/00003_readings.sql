-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 2, slice 2B: readings (docs/reference/schema.md "Step 2 tables").

-- +goose Up

CREATE TABLE readings (
    id                text NOT NULL CONSTRAINT readings_pkey PRIMARY KEY
                                    CONSTRAINT readings_id_check CHECK (length(id) = 26),
    church_id         text NOT NULL CONSTRAINT readings_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    reference         text NOT NULL,
    reference_display text NOT NULL,
    translation_id    text NOT NULL CONSTRAINT readings_translation_fkey REFERENCES translations (id),
    text              text NOT NULL,
    attribution       text NOT NULL,
    source_provider   text NOT NULL,
    search_fold       text NOT NULL,
    version           integer NOT NULL CONSTRAINT readings_version_check CHECK (version >= 1),
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    CONSTRAINT readings_church_ref_key UNIQUE (church_id, reference, translation_id)
);
CREATE INDEX readings_church_translation_idx ON readings (church_id, translation_id, updated_at);
