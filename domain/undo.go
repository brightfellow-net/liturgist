// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import "encoding/json"

// UndoWindow is how many of a person's latest editing rows undo looks at (11 §7.2).
const UndoWindow = 50

// Editing reports whether a history row with this command is an editing
// command: a possible undo target and part of the 50-row window. Undo and
// redo rows and the creation of the liturgy are not (11 §7.2).
func Editing(cmd string) bool {
	return cmd != CmdUndo && cmd != CmdRedo && cmd != CmdLiturgyCreate
}

// StructuralCommand reports whether a command changes the structure of the
// liturgy: its fields, or which items exist and in what order (10 §5).
func StructuralCommand(cmd string) bool {
	switch cmd {
	case CmdLiturgyUpdate, CmdItemAdd, CmdItemRemove, CmdItemsReorder:
		return true
	}
	return false
}

// Skippable reports whether a refused undo of an edit with this command may
// leave the edit behind as skipped. Structural edits keep blocking: an item
// removed earlier comes back at its old position only when every later
// structural edit of the same person has been reversed first (11 §7.2).
func Skippable(cmd string) bool { return !StructuralCommand(cmd) }

// Foreign is what the conflict test needs to know of a row written by
// another person after the target: for an undo or redo row, the command of
// the edit it acted on. Before and After are filled for assignment commands only.
type Foreign struct {
	Command       string
	ItemID        ItemID
	Before, After []byte
}

// TouchedBy reports whether the foreign row touches the edit, so that the
// edit can no longer be undone (or redone) without overwriting that person's
// work: the table of 11 §7.2.
func (e Edit) TouchedBy(f Foreign) bool {
	switch e.Command {
	case CmdItemUpdate, CmdItemSongs:
		return f.ItemID != "" && f.ItemID == e.ItemID
	case CmdItemAdd, CmdItemRemove:
		return (f.ItemID != "" && f.ItemID == e.ItemID) || StructuralCommand(f.Command)
	case CmdItemsReorder, CmdLiturgyUpdate:
		return StructuralCommand(f.Command)
	case CmdAssignmentAdd, CmdAssignmentRemove:
		if f.Command != CmdAssignmentAdd && f.Command != CmdAssignmentRemove {
			return false
		}
		mine, ok := AssignmentKeyOf(e.Before, e.After)
		if !ok {
			return false
		}
		theirs, ok := AssignmentKeyOf(f.Before, f.After)
		return ok && mine == theirs
	}
	return false
}

// AssignmentKeyOf names the duty and the person of the assignment held by
// the image of an assignment row: the same duty and person is the same key.
func AssignmentKeyOf(images ...[]byte) (string, bool) {
	for _, b := range images {
		if len(b) == 0 {
			continue
		}
		var a struct {
			DutyID string `json:"duty_id"`
			UserID string `json:"user_id"`
			Name   string `json:"name"`
		}
		if json.Unmarshal(b, &a) != nil || a.DutyID == "" {
			continue
		}
		if a.UserID != "" {
			return a.DutyID + "|u|" + a.UserID, true
		}
		name := a.Name
		key, err := ValidateAssignmentName(&name)
		if err != nil {
			return "", false
		}
		return a.DutyID + "|n|" + key, true
	}
	return "", false
}
