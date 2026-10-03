-- Copyright 2026 Brightfellow contributors
-- SPDX-License-Identifier: Apache-2.0
-- Step 3, slice 3D: the undo floor of a liturgy and the skipped flag of an edit
-- (docs/impl/11-liturgy-editor.md 7.2, P-65).

-- +goose Up

ALTER TABLE liturgies ADD COLUMN undo_floor_seq INTEGER NOT NULL DEFAULT 0
    CONSTRAINT liturgies_undo_floor_check CHECK (undo_floor_seq >= 0);
ALTER TABLE liturgy_edits ADD COLUMN skipped INTEGER NOT NULL DEFAULT 0
    CONSTRAINT liturgy_edits_skipped_check CHECK (skipped IN (0, 1));
