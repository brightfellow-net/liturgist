// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/domain"
)

// TC-P-001: duty and part names.
func TestValidateEntryName(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		kind    domain.ListKind
		in      string
		wantKey string
		bad     bool
	}{
		{domain.KindDuty, "  Pemandu Pujian ", "pemandu pujian", false},
		{domain.KindDuty, long(60), long(60), false},
		{domain.KindDuty, long(61), "", true},
		{domain.KindSingingPart, long(40), long(40), false},
		{domain.KindSingingPart, long(41), "", true},
		{domain.KindDuty, "", "", true},
		{domain.KindDuty, "   ", "", true},
		{domain.KindDuty, "Line\nbreak", "", true},
		{domain.KindDuty, "!!!", "", true},
		{domain.KindDuty, "Pemusik", "pemusik", false},
		{domain.KindDuty, "PÉMUSIK", "pemusik", false}, // folded: case and accents
		{domain.KindDuty, "主礼", "主礼", false},
	}
	for _, c := range cases {
		name := c.in
		key, err := domain.ValidateEntryName(c.kind, &name)
		var bad *domain.InvalidInputError
		if c.bad != errors.As(err, &bad) || (!c.bad && key != c.wantKey) {
			t.Errorf("%q (%s): key %q err %v", c.in, c.kind, key, err)
		}
	}
}

func st(weekday int, clock string) domain.ServiceTime {
	return domain.ServiceTime{Weekday: weekday, Time: clock}
}

func validTemplate() domain.Template {
	return domain.Template{Name: " Ibadah ", Language: "id", Items: []domain.TemplateItem{
		{Title: "Votum", Type: domain.ItemFreeText, DefaultText: "a\r\nb\r\n"},
		{Title: "Pujian", Type: domain.ItemSong},
	}}
}

// TC-P-002: template validation.
func TestValidateTemplate(t *testing.T) {
	tpl := validTemplate()
	if err := domain.ValidateTemplate(&tpl); err != nil {
		t.Fatal(err)
	}
	if tpl.Name != "Ibadah" || tpl.NameKey != "ibadah" || tpl.Items[0].DefaultText != "a\nb" {
		t.Errorf("normalised: %+v", tpl)
	}
	for name, mutate := range map[string]func(*domain.Template){
		"no name":          func(t *domain.Template) { t.Name = "" },
		"language":         func(t *domain.Template) { t.Language = "fr" },
		"empty title":      func(t *domain.Template) { t.Items[0].Title = " " },
		"long title":       func(t *domain.Template) { t.Items[0].Title = strings.Repeat("x", 201) },
		"unknown type":     func(t *domain.Template) { t.Items[0].Type = "dance" },
		"text on song":     func(t *domain.Template) { t.Items[1].DefaultText = "x" },
		"text on reading":  func(t *domain.Template) { t.Items[1].Type, t.Items[1].DefaultText = domain.ItemReading, "x" },
		"text too long":    func(t *domain.Template) { t.Items[0].DefaultText = strings.Repeat("x", 5001) },
		"too many items":   func(t *domain.Template) { t.Items = make([]domain.TemplateItem, 61) },
		"name too long":    func(t *domain.Template) { t.Name = strings.Repeat("n", 101) },
		"name only symbol": func(t *domain.Template) { t.Name = "---" },
	} {
		tpl := validTemplate()
		mutate(&tpl)
		var bad *domain.InvalidInputError
		if err := domain.ValidateTemplate(&tpl); !errors.As(err, &bad) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// 60 items are fine, 61 report the limit.
	tpl = domain.Template{Name: "T", Language: "id", Items: make([]domain.TemplateItem, 60)}
	for i := range tpl.Items {
		tpl.Items[i] = domain.TemplateItem{Title: "x", Type: domain.ItemOther}
	}
	if err := domain.ValidateTemplate(&tpl); err != nil {
		t.Errorf("60 items: %v", err)
	}
	tpl.Items = append(tpl.Items, tpl.Items[0])
	var bad *domain.InvalidInputError
	if err := domain.ValidateTemplate(&tpl); !errors.As(err, &bad) || bad.Reason != domain.ReasonLimit || bad.Max != 60 || bad.Used != 61 {
		t.Errorf("61 items: %v", err)
	}
}

// TC-P-003: service times.
func TestValidateService(t *testing.T) {
	mk := func(times ...domain.ServiceTime) domain.Service {
		return domain.Service{Name: "Ibadah Umum", Language: "id", Times: times}
	}
	ok := mk(st(7, "09:00"), st(7, "07:00"), st(3, "19:00"))
	if err := domain.ValidateService(&ok); err != nil {
		t.Fatal(err)
	}
	if got := ok.Times; got[0].Weekday != 3 || got[1].Time != "07:00" || got[2].Time != "09:00" {
		t.Errorf("times sorted by weekday and time: %v", got)
	}
	for name, s := range map[string]domain.Service{
		"no times":       mk(),
		"weekday 0":      mk(st(0, "07:00")),
		"weekday 8":      mk(st(8, "07:00")),
		"7:00":           mk(st(7, "7:00")),
		"24:00":          mk(st(3, "24:00")),
		"minute 60":      mk(st(3, "10:60")),
		"duplicate time": mk(st(7, "07:00"), st(7, "07:00")),
	} {
		s := s
		var bad *domain.InvalidInputError
		if err := domain.ValidateService(&s); !errors.As(err, &bad) {
			t.Errorf("%s: %v", name, err)
		}
	}
	empty := mk()
	var bad *domain.InvalidInputError
	if err := domain.ValidateService(&empty); !errors.As(err, &bad) || bad.Reason != domain.ReasonRequired {
		t.Errorf("no times: %v", err)
	}
	// 14 times are fine, 15 report the limit.
	var times []domain.ServiceTime
	for i := 0; i < 14; i++ {
		times = append(times, st(1+i%7, []string{"07:00", "09:00"}[i/7]))
	}
	s14 := mk(times...)
	if err := domain.ValidateService(&s14); err != nil {
		t.Errorf("14 times: %v", err)
	}
	s15 := mk(append(times, st(1, "11:00"))...)
	if err := domain.ValidateService(&s15); !errors.As(err, &bad) || bad.Reason != domain.ReasonLimit {
		t.Errorf("15 times: %v", err)
	}
}

// TC-P-005 (data part): the seeded sets are complete and valid in every language.
func TestSeedSets(t *testing.T) {
	for _, lang := range domain.ContentLanguages {
		set := domain.SeedFor(lang)
		if len(set.Duties) != 7 || len(set.SingingParts) != 6 || len(set.Items) != 7 {
			t.Errorf("%s: %d duties, %d parts, %d items", lang, len(set.Duties), len(set.SingingParts), len(set.Items))
		}
		keys := map[string]bool{}
		for _, d := range set.Duties {
			name := d
			key, err := domain.ValidateEntryName(domain.KindDuty, &name)
			if err != nil || keys[key] {
				t.Errorf("%s duty %q: %v (duplicate %v)", lang, d, err, keys[key])
			}
			keys[key] = true
		}
		keys = map[string]bool{}
		for _, p := range set.SingingParts {
			name := p
			key, err := domain.ValidateEntryName(domain.KindSingingPart, &name)
			if err != nil || keys[key] {
				t.Errorf("%s part %q: %v", lang, p, err)
			}
			keys[key] = true
		}
		tpl := domain.Template{Name: set.TemplateName, Language: lang}
		for _, it := range set.Items {
			if it.Duty < 0 || it.Duty >= len(set.Duties) {
				t.Fatalf("%s: duty index %d", lang, it.Duty)
			}
			tpl.Items = append(tpl.Items, domain.TemplateItem{Title: it.Title, Type: it.Type})
		}
		if err := domain.ValidateTemplate(&tpl); err != nil {
			t.Errorf("%s template: %v", lang, err)
		}
	}
	if domain.SeedFor("xx").TemplateName != "Ibadah Minggu" {
		t.Error("unknown language falls back to Indonesian")
	}
}
