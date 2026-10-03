-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 3, slice 3A: planning setup (docs/reference/schema.md "Step 3 tables").

-- +goose Up

CREATE TABLE duties (
    id         TEXT NOT NULL CONSTRAINT duties_pkey PRIMARY KEY
                             CONSTRAINT duties_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL CONSTRAINT duties_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    name_key   TEXT NOT NULL,
    position   INTEGER NOT NULL CONSTRAINT duties_position_check CHECK (position >= 0),
    created_at TEXT NOT NULL,
    CONSTRAINT duties_church_name_key UNIQUE (church_id, name_key),
    CONSTRAINT duties_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX duties_church_position_idx ON duties (church_id, position, id);

CREATE TABLE singing_parts (
    id         TEXT NOT NULL CONSTRAINT singing_parts_pkey PRIMARY KEY
                             CONSTRAINT singing_parts_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL CONSTRAINT singing_parts_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    name_key   TEXT NOT NULL,
    position   INTEGER NOT NULL CONSTRAINT singing_parts_position_check CHECK (position >= 0),
    created_at TEXT NOT NULL,
    CONSTRAINT singing_parts_church_name_key UNIQUE (church_id, name_key),
    CONSTRAINT singing_parts_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX singing_parts_church_position_idx ON singing_parts (church_id, position, id);

CREATE TABLE templates (
    id         TEXT NOT NULL CONSTRAINT templates_pkey PRIMARY KEY
                             CONSTRAINT templates_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL CONSTRAINT templates_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    name_key   TEXT NOT NULL,
    language   TEXT NOT NULL CONSTRAINT templates_language_check CHECK (language IN ('id', 'en', 'zh-Hans', 'zh-Hant')),
    version    INTEGER NOT NULL CONSTRAINT templates_version_check CHECK (version >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CONSTRAINT templates_church_name_key UNIQUE (church_id, name_key),
    CONSTRAINT templates_church_id_key UNIQUE (church_id, id)
) STRICT;

CREATE TABLE template_items (
    id              TEXT NOT NULL CONSTRAINT template_items_pkey PRIMARY KEY
                                  CONSTRAINT template_items_id_check CHECK (length(id) = 26),
    church_id       TEXT NOT NULL,
    template_id     TEXT NOT NULL,
    position        INTEGER NOT NULL CONSTRAINT template_items_position_check CHECK (position >= 0 AND position < 60),
    title           TEXT NOT NULL,
    item_type       TEXT NOT NULL CONSTRAINT template_items_type_check
                                  CHECK (item_type IN ('song', 'reading', 'prayer', 'sermon', 'free_text', 'other')),
    default_text    TEXT NOT NULL,
    default_duty_id TEXT,
    CONSTRAINT template_items_template_fkey FOREIGN KEY (church_id, template_id)
        REFERENCES templates (church_id, id) ON DELETE CASCADE,
    CONSTRAINT template_items_duty_fkey FOREIGN KEY (church_id, default_duty_id)
        REFERENCES duties (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT template_items_text_check
        CHECK (item_type IN ('prayer', 'sermon', 'free_text', 'other') OR default_text = '')
) STRICT;
CREATE INDEX template_items_church_template_idx ON template_items (church_id, template_id, position);
CREATE INDEX template_items_church_duty_idx ON template_items (church_id, default_duty_id)
    WHERE default_duty_id IS NOT NULL;

CREATE TABLE services (
    id                  TEXT NOT NULL CONSTRAINT services_pkey PRIMARY KEY
                                      CONSTRAINT services_id_check CHECK (length(id) = 26),
    church_id           TEXT NOT NULL CONSTRAINT services_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    name_key            TEXT NOT NULL,
    language            TEXT NOT NULL CONSTRAINT services_language_check CHECK (language IN ('id', 'en', 'zh-Hans', 'zh-Hant')),
    default_template_id TEXT,
    version             INTEGER NOT NULL CONSTRAINT services_version_check CHECK (version >= 1),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    CONSTRAINT services_template_fkey FOREIGN KEY (church_id, default_template_id)
        REFERENCES templates (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT services_church_name_key UNIQUE (church_id, name_key),
    CONSTRAINT services_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX services_church_template_idx ON services (church_id, default_template_id)
    WHERE default_template_id IS NOT NULL;

CREATE TABLE service_times (
    id         TEXT NOT NULL CONSTRAINT service_times_pkey PRIMARY KEY
                             CONSTRAINT service_times_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL,
    service_id TEXT NOT NULL,
    weekday    INTEGER NOT NULL CONSTRAINT service_times_weekday_check CHECK (weekday BETWEEN 1 AND 7),
    time       TEXT NOT NULL CONSTRAINT service_times_time_check CHECK (time GLOB '[0-2][0-9]:[0-5][0-9]'),
    CONSTRAINT service_times_service_fkey FOREIGN KEY (church_id, service_id)
        REFERENCES services (church_id, id) ON DELETE CASCADE,
    CONSTRAINT service_times_slot_key UNIQUE (service_id, weekday, time)
) STRICT;
CREATE INDEX service_times_church_service_idx ON service_times (church_id, service_id);

CREATE TABLE church_seeds (
    church_id  TEXT NOT NULL CONSTRAINT church_seeds_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    seed_key   TEXT NOT NULL CONSTRAINT church_seeds_key_check CHECK (length(seed_key) BETWEEN 1 AND 40),
    applied_at TEXT NOT NULL,
    CONSTRAINT church_seeds_pkey PRIMARY KEY (church_id, seed_key)
) STRICT;
