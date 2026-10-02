-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step-1 schema (docs/reference/schema.md). Timestamps: fixed-width UTC text.

-- +goose Up

CREATE TABLE translations (
    id       TEXT NOT NULL CONSTRAINT translations_pkey PRIMARY KEY
                           CONSTRAINT translations_id_check CHECK (length(id) = 26),
    code     TEXT NOT NULL CONSTRAINT translations_code_key UNIQUE,
    name     TEXT NOT NULL,
    language TEXT NOT NULL CONSTRAINT translations_language_check
                           CHECK (language IN ('id', 'en', 'zh-Hans', 'zh-Hant'))
) STRICT;

CREATE TABLE churches (
    id                     TEXT NOT NULL CONSTRAINT churches_pkey PRIMARY KEY
                                         CONSTRAINT churches_id_check CHECK (length(id) = 26),
    name                   TEXT NOT NULL,
    default_ui_language    TEXT NOT NULL CONSTRAINT churches_ui_language_check
                                         CHECK (default_ui_language IN ('en', 'id')),
    default_language       TEXT NOT NULL CONSTRAINT churches_language_check
                                         CHECK (default_language IN ('id', 'en', 'zh-Hans', 'zh-Hant')),
    default_translation_id TEXT NOT NULL CONSTRAINT churches_translation_fkey REFERENCES translations (id),
    time_zone              TEXT NOT NULL,
    settings               TEXT NOT NULL CONSTRAINT churches_settings_check
                                         CHECK (json_valid(settings) AND json_type(settings) = 'object'),
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL
) STRICT;

CREATE TABLE users (
    id            TEXT NOT NULL CONSTRAINT users_pkey PRIMARY KEY
                                CONSTRAINT users_id_check CHECK (length(id) = 26),
    name          TEXT NOT NULL,
    email         TEXT CONSTRAINT users_email_key UNIQUE,
    phone         TEXT CONSTRAINT users_phone_key UNIQUE
                       CONSTRAINT users_phone_check
                       CHECK (phone GLOB '+[1-9]*' AND phone NOT GLOB '+*[^0-9]*' AND length(phone) BETWEEN 8 AND 16),
    password_hash TEXT NOT NULL,
    preferences   TEXT NOT NULL CONSTRAINT users_preferences_check
                                CHECK (json_valid(preferences) AND json_type(preferences) = 'object'),
    last_seen_at  TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    CONSTRAINT users_identifier_check CHECK (email IS NOT NULL OR phone IS NOT NULL)
) STRICT;

CREATE TABLE memberships (
    id         TEXT NOT NULL CONSTRAINT memberships_pkey PRIMARY KEY
                             CONSTRAINT memberships_id_check CHECK (length(id) = 26),
    church_id  TEXT NOT NULL CONSTRAINT memberships_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL CONSTRAINT memberships_user_fkey REFERENCES users (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    CONSTRAINT memberships_church_user_key UNIQUE (church_id, user_id),
    CONSTRAINT memberships_church_id_key UNIQUE (church_id, id)
) STRICT;
CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE roles (
    id          TEXT NOT NULL CONSTRAINT roles_pkey PRIMARY KEY
                              CONSTRAINT roles_id_check CHECK (length(id) = 26),
    church_id   TEXT NOT NULL CONSTRAINT roles_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    name_key    TEXT NOT NULL,
    description TEXT NOT NULL,
    origin      TEXT CONSTRAINT roles_origin_check CHECK (origin IN ('church_admin', 'liturgist', 'editor')),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    CONSTRAINT roles_church_name_key UNIQUE (church_id, name_key),
    CONSTRAINT roles_church_origin_key UNIQUE (church_id, origin),
    CONSTRAINT roles_church_id_key UNIQUE (church_id, id)
) STRICT;

CREATE TABLE role_scopes (
    church_id TEXT NOT NULL,
    role_id   TEXT NOT NULL,
    scope     TEXT NOT NULL,
    CONSTRAINT role_scopes_pkey PRIMARY KEY (role_id, scope),
    CONSTRAINT role_scopes_role_fkey FOREIGN KEY (church_id, role_id)
        REFERENCES roles (church_id, id) ON DELETE CASCADE
) STRICT;
CREATE INDEX role_scopes_church_role_idx ON role_scopes (church_id, role_id);

CREATE TABLE membership_roles (
    church_id     TEXT NOT NULL,
    membership_id TEXT NOT NULL,
    role_id       TEXT NOT NULL,
    CONSTRAINT membership_roles_pkey PRIMARY KEY (membership_id, role_id),
    CONSTRAINT membership_roles_membership_fkey FOREIGN KEY (church_id, membership_id)
        REFERENCES memberships (church_id, id) ON DELETE CASCADE,
    CONSTRAINT membership_roles_role_fkey FOREIGN KEY (church_id, role_id)
        REFERENCES roles (church_id, id) ON DELETE CASCADE
) STRICT;
CREATE INDEX membership_roles_role_idx ON membership_roles (church_id, role_id);

CREATE TABLE invites (
    id               TEXT NOT NULL CONSTRAINT invites_pkey PRIMARY KEY
                                   CONSTRAINT invites_id_check CHECK (length(id) = 26),
    church_id        TEXT NOT NULL CONSTRAINT invites_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    token_hash       TEXT NOT NULL CONSTRAINT invites_token_hash_key UNIQUE
                                   CONSTRAINT invites_token_hash_check
                                   CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
    name             TEXT NOT NULL,
    email            TEXT,
    phone            TEXT CONSTRAINT invites_phone_check
                          CHECK (phone GLOB '+[1-9]*' AND phone NOT GLOB '+*[^0-9]*' AND length(phone) BETWEEN 8 AND 16),
    created_by       TEXT CONSTRAINT invites_created_by_fkey REFERENCES users (id) ON DELETE SET NULL,
    created_at       TEXT NOT NULL,
    expires_at       TEXT NOT NULL,
    accepted_at      TEXT,
    accepted_user_id TEXT CONSTRAINT invites_accepted_user_fkey REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at     TEXT,
    CONSTRAINT invites_church_id_key UNIQUE (church_id, id),
    CONSTRAINT invites_identifier_check CHECK (email IS NOT NULL OR phone IS NOT NULL),
    CONSTRAINT invites_state_check CHECK (NOT (accepted_at IS NOT NULL AND cancelled_at IS NOT NULL)),
    CONSTRAINT invites_accepted_user_check CHECK (accepted_user_id IS NULL OR accepted_at IS NOT NULL),
    CONSTRAINT invites_expiry_check CHECK (expires_at > created_at)
) STRICT;
CREATE INDEX invites_church_open_idx ON invites (church_id, expires_at)
    WHERE accepted_at IS NULL AND cancelled_at IS NULL;
CREATE UNIQUE INDEX invites_church_email_open_key ON invites (church_id, email)
    WHERE email IS NOT NULL AND accepted_at IS NULL AND cancelled_at IS NULL;
CREATE UNIQUE INDEX invites_church_phone_open_key ON invites (church_id, phone)
    WHERE phone IS NOT NULL AND accepted_at IS NULL AND cancelled_at IS NULL;

CREATE TABLE invite_roles (
    church_id TEXT NOT NULL,
    invite_id TEXT NOT NULL,
    role_id   TEXT NOT NULL,
    CONSTRAINT invite_roles_pkey PRIMARY KEY (invite_id, role_id),
    CONSTRAINT invite_roles_invite_fkey FOREIGN KEY (church_id, invite_id)
        REFERENCES invites (church_id, id) ON DELETE CASCADE,
    CONSTRAINT invite_roles_role_fkey FOREIGN KEY (church_id, role_id)
        REFERENCES roles (church_id, id) ON DELETE CASCADE
) STRICT;
CREATE INDEX invite_roles_church_role_idx ON invite_roles (church_id, role_id);

CREATE TABLE sessions (
    token_hash   TEXT NOT NULL CONSTRAINT sessions_pkey PRIMARY KEY
                               CONSTRAINT sessions_token_hash_check
                               CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
    user_id      TEXT NOT NULL CONSTRAINT sessions_user_fkey REFERENCES users (id) ON DELETE CASCADE,
    user_agent   TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    CONSTRAINT sessions_seen_check CHECK (last_seen_at >= created_at),
    CONSTRAINT sessions_expiry_check CHECK (expires_at > created_at)
) STRICT;
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE password_resets (
    id         TEXT NOT NULL CONSTRAINT password_resets_pkey PRIMARY KEY
                             CONSTRAINT password_resets_id_check CHECK (length(id) = 26),
    user_id    TEXT NOT NULL CONSTRAINT password_resets_user_fkey REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL CONSTRAINT password_resets_token_hash_key UNIQUE
                             CONSTRAINT password_resets_token_hash_check
                             CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
    created_by TEXT CONSTRAINT password_resets_created_by_fkey REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT,
    CONSTRAINT password_resets_expiry_check CHECK (expires_at > created_at)
) STRICT;
CREATE INDEX password_resets_user_idx ON password_resets (user_id);
CREATE UNIQUE INDEX password_resets_user_open_key ON password_resets (user_id) WHERE used_at IS NULL;

CREATE TABLE auth_throttle (
    key               TEXT NOT NULL CONSTRAINT auth_throttle_pkey PRIMARY KEY,
    kind              TEXT NOT NULL CONSTRAINT auth_throttle_kind_check CHECK (kind IN ('idip', 'id', 'ip')),
    failures          INTEGER NOT NULL CONSTRAINT auth_throttle_failures_check CHECK (failures >= 0),
    window_started_at TEXT NOT NULL,
    locked_until      TEXT,
    CONSTRAINT auth_throttle_lock_check CHECK (locked_until IS NULL OR locked_until >= window_started_at)
) STRICT;

CREATE TABLE setup_tokens (
    id         INTEGER NOT NULL CONSTRAINT setup_tokens_pkey PRIMARY KEY
                                CONSTRAINT setup_tokens_singleton_check CHECK (id = 1),
    token_hash TEXT NOT NULL CONSTRAINT setup_tokens_token_hash_check
                             CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    CONSTRAINT setup_tokens_expiry_check CHECK (expires_at > created_at)
) STRICT;

INSERT INTO translations (id, code, name, language) VALUES
    ('01M3XY2HBEKN8PETK6KCN370RJ', 'TB',  'Terjemahan Baru (LAI)',              'id'),
    ('01M3XY2HBEKN8PETK6KDMMG2WG', 'TB2', 'Terjemahan Baru Edisi Kedua (LAI)',  'id'),
    ('01M3XY2HBEKN8PETK6KHCT82HX', 'BIS', 'Bahasa Indonesia Sehari-hari (LAI)', 'id'),
    ('01M3XY2HBEKN8PETK6KHKD8V8T', 'CUV', '和合本 (Chinese Union Version)',      'zh-Hans'),
    ('01M3XY2HBEKN8PETK6KK6A7NB2', 'KJV', 'King James Version',                 'en'),
    ('01M3XY2HBEKN8PETK6KPANC6YT', 'WEB', 'World English Bible',                'en');
