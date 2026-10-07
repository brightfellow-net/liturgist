// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// The logo is a known settings key: it round-trips, other keys are kept, and a broken value reads as no logo.
func TestChurchSettingsLogo(t *testing.T) {
	in := `{"key_display":"do","logo":{"version":"0123456789abcdef","width":512,"height":256},"future":{"a":1}}`
	var s ChurchSettings
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if s.Logo == nil || *s.Logo != (ChurchLogo{Version: "0123456789abcdef", Width: 512, Height: 256}) {
		t.Fatalf("logo %+v", s.Logo)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"logo":{"version":"0123456789abcdef","width":512,"height":256}`, `"future":{"a":1}`, `"key_display":"do"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("%s lacks %s", out, want)
		}
	}
	s.Logo = nil
	if out, _ = json.Marshal(s); strings.Contains(string(out), "logo") {
		t.Errorf("a church without a logo writes %s", out)
	}

	for name, bad := range map[string]string{
		"short version": `{"logo":{"version":"abc","width":1,"height":1}}`,
		"upper case":    `{"logo":{"version":"0123456789ABCDEF","width":1,"height":1}}`,
		"no size":       `{"logo":{"version":"0123456789abcdef","width":0,"height":0}}`,
		"not an object": `{"logo":"x.png"}`,
	} {
		var s ChurchSettings
		if err := json.Unmarshal([]byte(bad), &s); err != nil || s.Logo != nil {
			t.Errorf("%s: err %v, logo %+v", name, err, s.Logo)
		}
	}
}
