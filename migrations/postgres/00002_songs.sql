-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 2, slice 2A: song library (docs/reference/schema.md "Step 2 tables").

-- +goose Up

CREATE TABLE song_groups (
    id         text NOT NULL CONSTRAINT song_groups_pkey PRIMARY KEY
                             CONSTRAINT song_groups_id_check CHECK (length(id) = 26),
    church_id  text NOT NULL CONSTRAINT song_groups_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    CONSTRAINT song_groups_church_id_key UNIQUE (church_id, id)
);

CREATE TABLE songs (
    id               text NOT NULL CONSTRAINT songs_pkey PRIMARY KEY
                                   CONSTRAINT songs_id_check CHECK (length(id) = 26),
    church_id        text NOT NULL CONSTRAINT songs_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    song_group_id    text,
    language         text NOT NULL CONSTRAINT songs_language_check
                                   CHECK (language IN ('id', 'en', 'zh-Hans', 'zh-Hant')),
    title            text NOT NULL,
    title_key        text NOT NULL,
    alt_titles       jsonb NOT NULL CONSTRAINT songs_alt_titles_check CHECK (jsonb_typeof(alt_titles) = 'array'),
    hymnal_source    text NOT NULL,
    hymnal_number    text NOT NULL,
    hymnal_key       text,
    lyricist         text NOT NULL,
    composer         text NOT NULL,
    translator       text NOT NULL,
    default_key      text NOT NULL,
    copyright_holder text NOT NULL,
    copyright_line   text NOT NULL,
    ccli_song_number text NOT NULL,
    licence_status   text NOT NULL CONSTRAINT songs_licence_status_check
                                   CHECK (licence_status IN ('unknown', 'public_domain', 'church_licence', 'permission_obtained')),
    licence_notes    text NOT NULL,
    version          integer NOT NULL CONSTRAINT songs_version_check CHECK (version >= 1),
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CONSTRAINT songs_church_id_key UNIQUE (church_id, id),
    CONSTRAINT songs_group_fkey FOREIGN KEY (church_id, song_group_id)
        REFERENCES song_groups (church_id, id) ON DELETE RESTRICT,
    CONSTRAINT songs_hymnal_check CHECK ((hymnal_source = '') = (hymnal_number = '')
        AND (hymnal_key IS NULL) = (hymnal_number = ''))
);
CREATE UNIQUE INDEX songs_group_language_key ON songs (church_id, song_group_id, language)
    WHERE song_group_id IS NOT NULL;
CREATE INDEX songs_church_title_idx ON songs (church_id, title_key, id);
CREATE INDEX songs_church_hymnal_idx ON songs (church_id, hymnal_key) WHERE hymnal_key IS NOT NULL;
CREATE INDEX songs_church_licence_idx ON songs (church_id, licence_status);

CREATE TABLE song_sections (
    id        text NOT NULL CONSTRAINT song_sections_pkey PRIMARY KEY
                            CONSTRAINT song_sections_id_check CHECK (length(id) = 26),
    church_id text NOT NULL,
    song_id   text NOT NULL,
    position  integer NOT NULL CONSTRAINT song_sections_position_check CHECK (position >= 0),
    kind      text NOT NULL CONSTRAINT song_sections_kind_check
                            CHECK (kind IN ('verse', 'pre_chorus', 'chorus', 'bridge', 'tag', 'intro', 'ending', 'other')),
    number    integer,
    label     text,
    text      text NOT NULL,
    CONSTRAINT song_sections_song_fkey FOREIGN KEY (church_id, song_id)
        REFERENCES songs (church_id, id) ON DELETE CASCADE,
    CONSTRAINT song_sections_church_song_id_key UNIQUE (church_id, song_id, id),
    CONSTRAINT song_sections_number_check CHECK (
        (kind = 'verse' AND number IS NOT NULL AND number BETWEEN 1 AND 99) OR (kind <> 'verse' AND number IS NULL))
);
CREATE UNIQUE INDEX song_sections_verse_key ON song_sections (song_id, number) WHERE kind = 'verse';
CREATE INDEX song_sections_church_song_idx ON song_sections (church_id, song_id, position);

CREATE TABLE song_arrangement_entries (
    church_id  text NOT NULL,
    song_id    text NOT NULL,
    position   integer NOT NULL CONSTRAINT song_arrangement_position_check CHECK (position >= 0 AND position < 100),
    section_id text NOT NULL,
    CONSTRAINT song_arrangement_entries_pkey PRIMARY KEY (song_id, position),
    CONSTRAINT song_arrangement_song_fkey FOREIGN KEY (church_id, song_id)
        REFERENCES songs (church_id, id) ON DELETE CASCADE,
    CONSTRAINT song_arrangement_section_fkey FOREIGN KEY (church_id, song_id, section_id)
        REFERENCES song_sections (church_id, song_id, id) ON DELETE CASCADE
);
CREATE INDEX song_arrangement_church_song_idx ON song_arrangement_entries (church_id, song_id, position);

CREATE TABLE song_search (
    church_id   text NOT NULL,
    song_id     text NOT NULL,
    language    text NOT NULL,
    head_fold   text NOT NULL,
    lyrics_fold text NOT NULL,
    fts_head    tsvector NOT NULL,
    fts_lyrics  tsvector NOT NULL,
    CONSTRAINT song_search_pkey PRIMARY KEY (church_id, song_id),
    CONSTRAINT song_search_song_fkey FOREIGN KEY (church_id, song_id)
        REFERENCES songs (church_id, id) ON DELETE CASCADE
);
CREATE INDEX song_search_fts_head_idx ON song_search USING GIN (fts_head);
CREATE INDEX song_search_fts_lyrics_idx ON song_search USING GIN (fts_lyrics);
