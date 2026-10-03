// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"testing"

	"github.com/brightfellow-net/liturgist/domain"
)

// TC-U-004: every row of the touch table of 11 §7.2, with a foreign row that
// does and one that does not touch.
func TestTouchedBy(t *testing.T) {
	asg := func(duty, user, name string) []byte {
		b := `{"id":"X","duty_id":"` + duty + `"`
		if user != "" {
			b += `,"user_id":"` + user + `"`
		}
		if name != "" {
			b += `,"name":"` + name + `"`
		}
		return []byte(b + "}")
	}
	f := func(cmd string, item domain.ItemID) domain.Foreign { return domain.Foreign{Command: cmd, ItemID: item} }
	fa := func(cmd string, img []byte) domain.Foreign {
		if cmd == domain.CmdAssignmentAdd {
			return domain.Foreign{Command: cmd, After: img}
		}
		return domain.Foreign{Command: cmd, Before: img}
	}
	for _, tc := range []struct {
		name    string
		target  domain.Edit
		foreign domain.Foreign
		want    bool
	}{
		{"update, same item, update", domain.Edit{Command: domain.CmdItemUpdate, ItemID: "X"}, f(domain.CmdItemUpdate, "X"), true},
		{"update, same item, an undo of a songs edit", domain.Edit{Command: domain.CmdItemUpdate, ItemID: "X"}, f(domain.CmdItemSongs, "X"), true},
		{"update, same item, removal", domain.Edit{Command: domain.CmdItemUpdate, ItemID: "X"}, f(domain.CmdItemRemove, "X"), true},
		{"update, another item", domain.Edit{Command: domain.CmdItemUpdate, ItemID: "X"}, f(domain.CmdItemUpdate, "Y"), false},
		{"update, a reorder", domain.Edit{Command: domain.CmdItemUpdate, ItemID: "X"}, f(domain.CmdItemsReorder, ""), false},
		{"update, a liturgy edit", domain.Edit{Command: domain.CmdItemUpdate, ItemID: "X"}, f(domain.CmdLiturgyUpdate, ""), false},
		{"songs, same item", domain.Edit{Command: domain.CmdItemSongs, ItemID: "X"}, f(domain.CmdItemUpdate, "X"), true},
		{"songs, another item", domain.Edit{Command: domain.CmdItemSongs, ItemID: "X"}, f(domain.CmdItemSongs, "Y"), false},
		{"add, same item", domain.Edit{Command: domain.CmdItemAdd, ItemID: "X"}, f(domain.CmdItemUpdate, "X"), true},
		{"add, another item's text", domain.Edit{Command: domain.CmdItemAdd, ItemID: "X"}, f(domain.CmdItemUpdate, "Y"), false},
		{"add, another item added", domain.Edit{Command: domain.CmdItemAdd, ItemID: "X"}, f(domain.CmdItemAdd, "Y"), true},
		{"add, a removal", domain.Edit{Command: domain.CmdItemAdd, ItemID: "X"}, f(domain.CmdItemRemove, "Y"), true},
		{"add, a reorder", domain.Edit{Command: domain.CmdItemAdd, ItemID: "X"}, f(domain.CmdItemsReorder, ""), true},
		{"add, a liturgy edit", domain.Edit{Command: domain.CmdItemAdd, ItemID: "X"}, f(domain.CmdLiturgyUpdate, ""), true},
		{"remove, same item", domain.Edit{Command: domain.CmdItemRemove, ItemID: "X"}, f(domain.CmdItemSongs, "X"), true},
		{"remove, another item's songs", domain.Edit{Command: domain.CmdItemRemove, ItemID: "X"}, f(domain.CmdItemSongs, "Y"), false},
		{"remove, a reorder", domain.Edit{Command: domain.CmdItemRemove, ItemID: "X"}, f(domain.CmdItemsReorder, ""), true},
		{"reorder, an item edit", domain.Edit{Command: domain.CmdItemsReorder}, f(domain.CmdItemUpdate, "X"), false},
		{"reorder, an assignment", domain.Edit{Command: domain.CmdItemsReorder}, fa(domain.CmdAssignmentAdd, asg("D", "U", "")), false},
		{"reorder, an add", domain.Edit{Command: domain.CmdItemsReorder}, f(domain.CmdItemAdd, "X"), true},
		{"reorder, a reorder", domain.Edit{Command: domain.CmdItemsReorder}, f(domain.CmdItemsReorder, ""), true},
		{"liturgy.update, an item edit", domain.Edit{Command: domain.CmdLiturgyUpdate}, f(domain.CmdItemUpdate, "X"), false},
		{"liturgy.update, a removal", domain.Edit{Command: domain.CmdLiturgyUpdate}, f(domain.CmdItemRemove, "X"), true},
		{"liturgy.update, a liturgy edit", domain.Edit{Command: domain.CmdLiturgyUpdate}, f(domain.CmdLiturgyUpdate, ""), true},
		{"assignment, same duty and member", domain.Edit{Command: domain.CmdAssignmentAdd, After: asg("D", "U", "")}, fa(domain.CmdAssignmentRemove, asg("D", "U", "")), true},
		{"assignment, same duty another member", domain.Edit{Command: domain.CmdAssignmentAdd, After: asg("D", "U", "")}, fa(domain.CmdAssignmentAdd, asg("D", "V", "")), false},
		{"assignment, another duty same member", domain.Edit{Command: domain.CmdAssignmentAdd, After: asg("D", "U", "")}, fa(domain.CmdAssignmentAdd, asg("E", "U", "")), false},
		{"assignment, same duty and name, other spelling", domain.Edit{Command: domain.CmdAssignmentRemove, Before: asg("D", "", "Budi  Santoso")}, fa(domain.CmdAssignmentAdd, asg("D", "", "budi santoso")), true},
		{"assignment, a member against a name", domain.Edit{Command: domain.CmdAssignmentAdd, After: asg("D", "U", "")}, fa(domain.CmdAssignmentAdd, asg("D", "", "U")), false},
		{"assignment, an item edit", domain.Edit{Command: domain.CmdAssignmentAdd, After: asg("D", "U", "")}, f(domain.CmdItemUpdate, "X"), false},
		{"assignment, a structural edit", domain.Edit{Command: domain.CmdAssignmentRemove, Before: asg("D", "U", "")}, f(domain.CmdItemsReorder, ""), false},
		{"an item edit with no item is never touched by a row with no item", domain.Edit{Command: domain.CmdItemUpdate}, f(domain.CmdItemUpdate, ""), false},
	} {
		if got := tc.target.TouchedBy(tc.foreign); got != tc.want {
			t.Errorf("%s: touched = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSkippable(t *testing.T) {
	for cmd, want := range map[string]bool{
		domain.CmdItemUpdate: true, domain.CmdItemSongs: true, domain.CmdAssignmentAdd: true, domain.CmdAssignmentRemove: true,
		domain.CmdItemAdd: false, domain.CmdItemRemove: false, domain.CmdItemsReorder: false, domain.CmdLiturgyUpdate: false,
	} {
		if got := domain.Skippable(cmd); got != want {
			t.Errorf("Skippable(%s) = %v, want %v", cmd, got, want)
		}
	}
	for _, cmd := range []string{domain.CmdUndo, domain.CmdRedo, domain.CmdLiturgyCreate} {
		if domain.Editing(cmd) {
			t.Errorf("%s is no editing command", cmd)
		}
	}
}
