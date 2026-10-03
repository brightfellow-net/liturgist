-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 3, slice 3B: liturgies, items, songs, sequences, assignments, history
-- (docs/reference/schema.md "Step 3 tables").

-- +goose Up

-- Composite foreign-key targets that the earlier migrations did not need.
CREATE UNIQUE INDEX readings_church_id_key ON readings (church_id, id);
CREATE UNIQUE INDEX song_sections_church_id_key ON song_sections (church_id, id);

CREATE TABLE liturgies (
    id          TEXT NOT NULL CONSTRAINT liturgies_pkey PRIMARY KEY
                              CONSTRAINT liturgies_id_check CHECK (length(id) = 26),
    church_id   TEXT NOT NULL CONSTRAINT liturgies_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    date        TEXT NOT NULL CONSTRAINT liturgies_date_check
                              CHECK (date GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    time        TEXT NOT NULL CONSTRAINT liturgies_time_check
                              CHECK (time = '' OR time GLOB '[0-2][0-9]:[0-5][0-9]'),
    service_id  TEXT,
    service_name TEXT NOT NULL,
    language    TEXT NOT NULL CONSTRAINT liturgies_language_check CHECK (language IN ('id', 'en', 'zh-Hans', 'zh-Hant')),
    template_id TEXT,
    state       TEXT NOT NULL CONSTRAINT liturgies_state_check
                              CHECK (state IN ('draft', 'in_review', 'needs_revision', 'approved', 'published')),
    version     INTEGER NOT NULL CONSTRAINT liturgies_version_check CHECK (version >= 1),
    archived_at TEXT,
    archived_by TEXT CONSTRAINT liturgies_archived_by_fkey REFERENCES users (id) ON DELETE RESTRICT,
    created_by  TEXT NOT NULL CONSTRAINT liturgies_created_by_fkey REFERENCES users (id) ON DELETE RESTRICT,
    edit_seq    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    CONSTRAINT liturgies_service_fkey FOREIGN KEY (church_id, service_id)
        REFERENCES services (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT liturgies_template_fkey FOREIGN KEY (church_id, template_id)
        REFERENCES templates (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT liturgies_archived_check CHECK ((archived_at IS NULL) = (archived_by IS NULL)),
    CONSTRAINT liturgies_slot_check CHECK (service_id IS NULL OR time <> ''),
    CONSTRAINT liturgies_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE UNIQUE INDEX liturgies_service_slot_key ON liturgies (church_id, service_id, date, time)
    WHERE service_id IS NOT NULL;
CREATE INDEX liturgies_church_date_idx ON liturgies (church_id, date, time, id);
CREATE INDEX liturgies_church_state_idx ON liturgies (church_id, state, archived_at);

CREATE TABLE liturgy_items (
    id            TEXT NOT NULL CONSTRAINT liturgy_items_pkey PRIMARY KEY
                                CONSTRAINT liturgy_items_id_check CHECK (length(id) = 26),
    church_id     TEXT NOT NULL,
    liturgy_id    TEXT NOT NULL,
    position      INTEGER NOT NULL CONSTRAINT liturgy_items_position_check CHECK (position >= 0 AND position < 60),
    title         TEXT NOT NULL,
    item_type     TEXT NOT NULL CONSTRAINT liturgy_items_type_check
                                CHECK (item_type IN ('song', 'reading', 'prayer', 'sermon', 'free_text', 'other')),
    duty_id       TEXT,
    text          TEXT NOT NULL,
    reading_id    TEXT,
    reading_label TEXT NOT NULL,
    version       INTEGER NOT NULL CONSTRAINT liturgy_items_version_check CHECK (version >= 1),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    CONSTRAINT liturgy_items_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE CASCADE,
    CONSTRAINT liturgy_items_duty_fkey FOREIGN KEY (church_id, duty_id)
        REFERENCES duties (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT liturgy_items_reading_fkey FOREIGN KEY (church_id, reading_id)
        REFERENCES readings (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT liturgy_items_text_check
        CHECK (item_type IN ('prayer', 'sermon', 'free_text', 'other') OR text = ''),
    CONSTRAINT liturgy_items_reading_check
        CHECK (item_type = 'reading' OR (reading_id IS NULL AND reading_label = '')),
    CONSTRAINT liturgy_items_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX liturgy_items_church_liturgy_idx ON liturgy_items (church_id, liturgy_id, position);
CREATE INDEX liturgy_items_church_duty_idx ON liturgy_items (church_id, duty_id) WHERE duty_id IS NOT NULL;
CREATE INDEX liturgy_items_church_reading_idx ON liturgy_items (church_id, reading_id) WHERE reading_id IS NOT NULL;

CREATE TABLE liturgy_item_songs (
    id         TEXT NOT NULL CONSTRAINT liturgy_item_songs_pkey PRIMARY KEY
                             CONSTRAINT liturgy_item_songs_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    position   INTEGER NOT NULL CONSTRAINT liturgy_item_songs_position_check CHECK (position >= 0 AND position < 10),
    song_id    TEXT,
    song_title TEXT NOT NULL,
    key        TEXT NOT NULL,
    note       TEXT NOT NULL,
    CONSTRAINT liturgy_item_songs_item_fkey FOREIGN KEY (church_id, item_id)
        REFERENCES liturgy_items (church_id, id) ON DELETE CASCADE,
    CONSTRAINT liturgy_item_songs_song_fkey FOREIGN KEY (church_id, song_id)
        REFERENCES songs (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT liturgy_item_songs_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX liturgy_item_songs_church_item_idx ON liturgy_item_songs (church_id, item_id, position);
CREATE INDEX liturgy_item_songs_church_song_idx ON liturgy_item_songs (church_id, song_id) WHERE song_id IS NOT NULL;

CREATE TABLE sequence_entries (
    id              TEXT NOT NULL CONSTRAINT sequence_entries_pkey PRIMARY KEY
                                  CONSTRAINT sequence_entries_id_check CHECK (length(id) = 26),
    church_id       TEXT NOT NULL,
    item_song_id    TEXT NOT NULL,
    position        INTEGER NOT NULL CONSTRAINT sequence_entries_position_check CHECK (position >= 0 AND position < 100),
    kind            TEXT NOT NULL CONSTRAINT sequence_entries_kind_check CHECK (kind IN ('section')),
    song_section_id TEXT,
    singing_part_id TEXT,
    key_change      TEXT NOT NULL,
    section_label   TEXT NOT NULL,
    note            TEXT NOT NULL,
    CONSTRAINT sequence_entries_song_fkey FOREIGN KEY (church_id, item_song_id)
        REFERENCES liturgy_item_songs (church_id, id) ON DELETE CASCADE,
    CONSTRAINT sequence_entries_section_fkey FOREIGN KEY (church_id, song_section_id)
        REFERENCES song_sections (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT sequence_entries_part_fkey FOREIGN KEY (church_id, singing_part_id)
        REFERENCES singing_parts (church_id, id) ON DELETE RESTRICT
) STRICT;
CREATE INDEX sequence_entries_church_song_idx ON sequence_entries (church_id, item_song_id, position);
CREATE INDEX sequence_entries_church_section_idx ON sequence_entries (church_id, song_section_id)
    WHERE song_section_id IS NOT NULL;
CREATE INDEX sequence_entries_church_part_idx ON sequence_entries (church_id, singing_part_id)
    WHERE singing_part_id IS NOT NULL;

CREATE TABLE assignments (
    id         TEXT NOT NULL CONSTRAINT assignments_pkey PRIMARY KEY
                             CONSTRAINT assignments_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL,
    liturgy_id TEXT NOT NULL,
    duty_id    TEXT NOT NULL,
    user_id    TEXT CONSTRAINT assignments_user_fkey REFERENCES users (id) ON DELETE RESTRICT,
    name       TEXT,
    name_key   TEXT,
    created_at TEXT NOT NULL,
    CONSTRAINT assignments_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE CASCADE,
    CONSTRAINT assignments_duty_fkey FOREIGN KEY (church_id, duty_id)
        REFERENCES duties (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT assignments_person_check
        CHECK ((user_id IS NULL) <> (name IS NULL) AND (name IS NULL) = (name_key IS NULL))
) STRICT;
CREATE UNIQUE INDEX assignments_user_key ON assignments (liturgy_id, duty_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX assignments_name_key ON assignments (liturgy_id, duty_id, name_key) WHERE name IS NOT NULL;
CREATE INDEX assignments_church_liturgy_idx ON assignments (church_id, liturgy_id);

CREATE TABLE liturgy_edits (
    id                    TEXT NOT NULL CONSTRAINT liturgy_edits_pkey PRIMARY KEY
                                        CONSTRAINT liturgy_edits_id_check CHECK (length(id) = 26),
    church_id             TEXT NOT NULL,
    liturgy_id            TEXT NOT NULL,
    user_id               TEXT NOT NULL CONSTRAINT liturgy_edits_user_fkey REFERENCES users (id) ON DELETE RESTRICT,
    seq                   INTEGER NOT NULL,
    command               TEXT NOT NULL CONSTRAINT liturgy_edits_command_check CHECK (command IN (
        'liturgy.create', 'liturgy.update', 'item.add', 'item.remove', 'item.update', 'item.songs',
        'items.reorder', 'assignment.add', 'assignment.remove', 'undo', 'redo')),
    target_edit_id        TEXT,
    item_id               TEXT,
    before                TEXT,
    after                 TEXT,
    liturgy_version_after INTEGER NOT NULL,
    item_version_after    INTEGER,
    status                TEXT NOT NULL CONSTRAINT liturgy_edits_status_check
                                        CHECK (status IN ('done', 'undone', 'dropped')),
    undo_seq              INTEGER,
    created_at            TEXT NOT NULL,
    CONSTRAINT liturgy_edits_liturgy_fkey FOREIGN KEY (church_id, liturgy_id)
        REFERENCES liturgies (church_id, id) ON DELETE CASCADE,
    CONSTRAINT liturgy_edits_seq_key UNIQUE (liturgy_id, seq)
) STRICT;
CREATE INDEX liturgy_edits_church_liturgy_idx ON liturgy_edits (church_id, liturgy_id, seq);
CREATE INDEX liturgy_edits_church_user_idx ON liturgy_edits (church_id, liturgy_id, user_id, status, seq);
