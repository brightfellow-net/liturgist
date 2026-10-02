-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step-1 schema (docs/reference/schema.md). Timestamps: timestamptz; JSON: jsonb.

-- +goose Up

CREATE TABLE translations (
    id       text NOT NULL CONSTRAINT translations_pkey PRIMARY KEY
                           CONSTRAINT translations_id_check CHECK (length(id) = 26),
    code     text NOT NULL CONSTRAINT translations_code_key UNIQUE,
    name     text NOT NULL,
    language text NOT NULL CONSTRAINT translations_language_check
                           CHECK (language IN ('id', 'en', 'zh-Hans', 'zh-Hant'))
);

CREATE TABLE churches (
    id                     text NOT NULL CONSTRAINT churches_pkey PRIMARY KEY
                                         CONSTRAINT churches_id_check CHECK (length(id) = 26),
    name                   text NOT NULL,
    default_ui_language    text NOT NULL CONSTRAINT churches_ui_language_check
                                         CHECK (default_ui_language IN ('en', 'id')),
    default_language       text NOT NULL CONSTRAINT churches_language_check
                                         CHECK (default_language IN ('id', 'en', 'zh-Hans', 'zh-Hant')),
    default_translation_id text NOT NULL CONSTRAINT churches_translation_fkey REFERENCES translations (id),
    time_zone              text NOT NULL,
    settings               jsonb NOT NULL CONSTRAINT churches_settings_check
                                         CHECK (jsonb_typeof(settings) = 'object'),
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL
);

CREATE TABLE users (
    id            text NOT NULL CONSTRAINT users_pkey PRIMARY KEY
                                CONSTRAINT users_id_check CHECK (length(id) = 26),
    name          text NOT NULL,
    email         text CONSTRAINT users_email_key UNIQUE,
    phone         text CONSTRAINT users_phone_key UNIQUE
                       CONSTRAINT users_phone_check
                       CHECK (phone ~ '^\+[1-9][0-9]{6,14}$'),
    password_hash text NOT NULL,
    preferences   jsonb NOT NULL CONSTRAINT users_preferences_check
                                CHECK (jsonb_typeof(preferences) = 'object'),
    last_seen_at  timestamptz,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT users_identifier_check CHECK (email IS NOT NULL OR phone IS NOT NULL)
);

CREATE TABLE memberships (
    id         text NOT NULL CONSTRAINT memberships_pkey PRIMARY KEY
                             CONSTRAINT memberships_id_check CHECK (length(id) = 26),
    church_id  text NOT NULL CONSTRAINT memberships_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    user_id    text NOT NULL CONSTRAINT memberships_user_fkey REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    CONSTRAINT memberships_church_user_key UNIQUE (church_id, user_id),
    CONSTRAINT memberships_church_id_key UNIQUE (church_id, id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE roles (
    id          text NOT NULL CONSTRAINT roles_pkey PRIMARY KEY
                              CONSTRAINT roles_id_check CHECK (length(id) = 26),
    church_id   text NOT NULL CONSTRAINT roles_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    name        text NOT NULL,
    name_key    text NOT NULL,
    description text NOT NULL,
    origin      text CONSTRAINT roles_origin_check CHECK (origin IN ('church_admin', 'liturgist', 'editor')),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT roles_church_name_key UNIQUE (church_id, name_key),
    CONSTRAINT roles_church_origin_key UNIQUE (church_id, origin),
    CONSTRAINT roles_church_id_key UNIQUE (church_id, id)
);

CREATE TABLE role_scopes (
    church_id text NOT NULL,
    role_id   text NOT NULL,
    scope     text NOT NULL,
    CONSTRAINT role_scopes_pkey PRIMARY KEY (role_id, scope),
    CONSTRAINT role_scopes_role_fkey FOREIGN KEY (church_id, role_id)
        REFERENCES roles (church_id, id) ON DELETE CASCADE
);
CREATE INDEX role_scopes_church_role_idx ON role_scopes (church_id, role_id);

CREATE TABLE membership_roles (
    church_id     text NOT NULL,
    membership_id text NOT NULL,
    role_id       text NOT NULL,
    CONSTRAINT membership_roles_pkey PRIMARY KEY (membership_id, role_id),
    CONSTRAINT membership_roles_membership_fkey FOREIGN KEY (church_id, membership_id)
        REFERENCES memberships (church_id, id) ON DELETE CASCADE,
    CONSTRAINT membership_roles_role_fkey FOREIGN KEY (church_id, role_id)
        REFERENCES roles (church_id, id) ON DELETE CASCADE
);
CREATE INDEX membership_roles_role_idx ON membership_roles (church_id, role_id);

CREATE TABLE invites (
    id               text NOT NULL CONSTRAINT invites_pkey PRIMARY KEY
                                   CONSTRAINT invites_id_check CHECK (length(id) = 26),
    church_id        text NOT NULL CONSTRAINT invites_church_fkey REFERENCES churches (id) ON DELETE CASCADE,
    token_hash       text NOT NULL CONSTRAINT invites_token_hash_key UNIQUE
                                   CONSTRAINT invites_token_hash_check
                                   CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    name             text NOT NULL,
    email            text,
    phone            text CONSTRAINT invites_phone_check
                          CHECK (phone ~ '^\+[1-9][0-9]{6,14}$'),
    created_by       text CONSTRAINT invites_created_by_fkey REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL,
    expires_at       timestamptz NOT NULL,
    accepted_at      timestamptz,
    accepted_user_id text CONSTRAINT invites_accepted_user_fkey REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at     timestamptz,
    CONSTRAINT invites_church_id_key UNIQUE (church_id, id),
    CONSTRAINT invites_identifier_check CHECK (email IS NOT NULL OR phone IS NOT NULL),
    CONSTRAINT invites_state_check CHECK (NOT (accepted_at IS NOT NULL AND cancelled_at IS NOT NULL)),
    CONSTRAINT invites_accepted_user_check CHECK (accepted_user_id IS NULL OR accepted_at IS NOT NULL),
    CONSTRAINT invites_expiry_check CHECK (expires_at > created_at)
);
CREATE INDEX invites_church_open_idx ON invites (church_id, expires_at)
    WHERE accepted_at IS NULL AND cancelled_at IS NULL;
CREATE UNIQUE INDEX invites_church_email_open_key ON invites (church_id, email)
    WHERE email IS NOT NULL AND accepted_at IS NULL AND cancelled_at IS NULL;
CREATE UNIQUE INDEX invites_church_phone_open_key ON invites (church_id, phone)
    WHERE phone IS NOT NULL AND accepted_at IS NULL AND cancelled_at IS NULL;

CREATE TABLE invite_roles (
    church_id text NOT NULL,
    invite_id text NOT NULL,
    role_id   text NOT NULL,
    CONSTRAINT invite_roles_pkey PRIMARY KEY (invite_id, role_id),
    CONSTRAINT invite_roles_invite_fkey FOREIGN KEY (church_id, invite_id)
        REFERENCES invites (church_id, id) ON DELETE CASCADE,
    CONSTRAINT invite_roles_role_fkey FOREIGN KEY (church_id, role_id)
        REFERENCES roles (church_id, id) ON DELETE CASCADE
);
CREATE INDEX invite_roles_church_role_idx ON invite_roles (church_id, role_id);

CREATE TABLE sessions (
    token_hash   text NOT NULL CONSTRAINT sessions_pkey PRIMARY KEY
                               CONSTRAINT sessions_token_hash_check
                               CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    user_id      text NOT NULL CONSTRAINT sessions_user_fkey REFERENCES users (id) ON DELETE CASCADE,
    user_agent   text NOT NULL,
    created_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    CONSTRAINT sessions_seen_check CHECK (last_seen_at >= created_at),
    CONSTRAINT sessions_expiry_check CHECK (expires_at > created_at)
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE password_resets (
    id         text NOT NULL CONSTRAINT password_resets_pkey PRIMARY KEY
                             CONSTRAINT password_resets_id_check CHECK (length(id) = 26),
    user_id    text NOT NULL CONSTRAINT password_resets_user_fkey REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL CONSTRAINT password_resets_token_hash_key UNIQUE
                             CONSTRAINT password_resets_token_hash_check
                             CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    created_by text CONSTRAINT password_resets_created_by_fkey REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    CONSTRAINT password_resets_expiry_check CHECK (expires_at > created_at)
);
CREATE INDEX password_resets_user_idx ON password_resets (user_id);
CREATE UNIQUE INDEX password_resets_user_open_key ON password_resets (user_id) WHERE used_at IS NULL;

CREATE TABLE auth_throttle (
    key               text NOT NULL CONSTRAINT auth_throttle_pkey PRIMARY KEY,
    kind              text NOT NULL CONSTRAINT auth_throttle_kind_check CHECK (kind IN ('idip', 'id', 'ip')),
    failures          integer NOT NULL CONSTRAINT auth_throttle_failures_check CHECK (failures >= 0),
    window_started_at timestamptz NOT NULL,
    locked_until      timestamptz,
    CONSTRAINT auth_throttle_lock_check CHECK (locked_until IS NULL OR locked_until >= window_started_at)
);

CREATE TABLE setup_tokens (
    id         integer NOT NULL CONSTRAINT setup_tokens_pkey PRIMARY KEY
                                CONSTRAINT setup_tokens_singleton_check CHECK (id = 1),
    token_hash text NOT NULL CONSTRAINT setup_tokens_token_hash_check
                             CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    CONSTRAINT setup_tokens_expiry_check CHECK (expires_at > created_at)
);

INSERT INTO translations (id, code, name, language) VALUES
    ('01M3XY2HBEKN8PETK6KCN370RJ', 'TB',  'Terjemahan Baru (LAI)',              'id'),
    ('01M3XY2HBEKN8PETK6KDMMG2WG', 'TB2', 'Terjemahan Baru Edisi Kedua (LAI)',  'id'),
    ('01M3XY2HBEKN8PETK6KHCT82HX', 'BIS', 'Bahasa Indonesia Sehari-hari (LAI)', 'id'),
    ('01M3XY2HBEKN8PETK6KHKD8V8T', 'CUV', '和合本 (Chinese Union Version)',      'zh-Hans'),
    ('01M3XY2HBEKN8PETK6KK6A7NB2', 'KJV', 'King James Version',                 'en'),
    ('01M3XY2HBEKN8PETK6KPANC6YT', 'WEB', 'World English Bible',                'en');
