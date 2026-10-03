// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/sqlstore/sqlstoretest"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
)

// fakeProvider answers from a table, or misbehaves in one chosen way.
type fakeProvider struct {
	id    string
	text  app.BibleText
	err   error
	block bool // ignores the deadline and never answers
	mu    sync.Mutex
	asked int
}

func (p *fakeProvider) ID() string { return p.id }

func (p *fakeProvider) Lookup(_ context.Context, _ domain.Reference, _ string) (app.BibleText, error) {
	p.mu.Lock()
	p.asked++
	p.mu.Unlock()
	if p.block {
		select {} // a provider that ignores its context
	}
	return p.text, p.err
}

func (p *fakeProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.asked
}

func good(id string, mayStore bool) *fakeProvider {
	return &fakeProvider{id: id, text: app.BibleText{Text: "Karena begitu besar kasih Allah", Attribution: "Dari " + id, Source: id, MayStore: mayStore}}
}

// withProviders returns the environment's use case with providers registered.
func (e cenv) withProviders(timeout time.Duration, ps ...app.BibleTextProvider) *app.Readings {
	r := *e.readings
	r.Providers, r.ProviderTimeout = ps, timeout
	return &r
}

// IT-R-001: who can do what, and one reading however it was typed.
func TestReadingPermissionsAndForms(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	team, _ := e.member("team@example.org")
	editor, _ := e.member("editor@example.org", e.role(domain.OriginEditor).ID)

	v, err := e.readings.Create(e.ctx, editor, app.ReadingInput{Reference: "Yoh 3:16-21", Translation: "TB", Text: "Karena begitu besar  \r\n\r\n", Attribution: " LAI "})
	if err != nil {
		t.Fatal(err)
	}
	r := v.Reading
	if r.Reference != "JHN 3:16-21" || r.ReferenceDisplay != "Yoh 3:16-21" || r.Text != "Karena begitu besar" || r.Attribution != "LAI" ||
		r.SourceProvider != "manual" || r.Version != 1 || r.Translation.Code != "TB" || !v.Actions.Edit || !v.Actions.Delete {
		t.Errorf("created: %+v", v)
	}

	// The same passage typed another way is the same reading.
	_, err = e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Yohanes  3:16 – 21", Translation: "TB", Text: "lain"})
	var exists *app.ReadingExistsError
	if !errors.As(err, &exists) || exists.ID != r.ID {
		t.Errorf("same passage, other spelling: %v", err)
	}
	// Another translation is a separate reading.
	if _, err := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Yoh 3:16-21", Translation: "BIS", Text: "BIS teks"}); err != nil {
		t.Errorf("another translation: %v", err)
	}
	// Without a translation, the church's default is used.
	if d, err := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Mzm 23", Text: "TUHAN adalah gembalaku"}); err != nil || d.Reading.Translation.Code != "TB" {
		t.Errorf("default translation: %+v %v", d.Reading.Translation, err)
	}

	// A team member can look but not change.
	if got, err := e.readings.Get(e.ctx, team, r.ID); err != nil || got.Actions.Edit || got.Actions.Delete {
		t.Errorf("team get: %+v %v", got, err)
	}
	if _, err := e.readings.Create(e.ctx, team, app.ReadingInput{Reference: "Mzm 1", Text: "x"}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team create: %v", err)
	}
	text := "x"
	if _, err := e.readings.Update(e.ctx, team, r.ID, app.ReadingChange{Version: 1, Text: &text}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team update: %v", err)
	}
	if err := e.readings.Delete(e.ctx, team, r.ID); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("team delete: %v", err)
	}
	if _, err := e.readings.FromProvider(e.ctx, team, "Mzm 1", "TB", "p"); !errors.Is(err, app.ErrForbidden) && invalidField(err) != "provider" {
		t.Errorf("team from-provider: %v", err)
	}
	if _, err := e.readings.Get(e.ctx, nil, r.ID); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("no session: %v", err)
	}

	// Validation.
	for name, in := range map[string]app.ReadingInput{
		"text":        {Reference: "Mzm 2", Translation: "TB", Text: "  "},
		"attribution": {Reference: "Mzm 2", Translation: "TB", Text: "x", Attribution: strings.Repeat("a", 301)},
		"translation": {Reference: "Mzm 2", Translation: "NOPE", Text: "x"},
	} {
		if _, err := e.readings.Create(e.ctx, e.admin, in); invalidField(err) != name {
			t.Errorf("%s: %v", name, err)
		}
	}
	var badRef *domain.ReferenceError
	if _, err := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Foo 1", Text: "x"}); !errors.As(err, &badRef) {
		t.Errorf("bad reference: %v", err)
	}
}

// Optimistic versions, display changes, delete and the usage hook (IT-R-001, IT-R-006).
func TestReadingUpdateDelete(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	v, err := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Yoh 3:16", Translation: "TB", Text: "satu"})
	if err != nil {
		t.Fatal(err)
	}
	id := v.Reading.ID
	text, attr := "dua", "LAI"
	up, err := e.readings.Update(e.ctx, e.admin, id, app.ReadingChange{Version: 1, Text: &text, Attribution: &attr})
	if err != nil || up.Reading.Version != 2 || up.Reading.Text != "dua" || up.Reading.Attribution != "LAI" || up.Reading.Reference != "JHN 3:16" {
		t.Fatalf("update: %+v %v", up, err)
	}
	if _, err := e.readings.Update(e.ctx, e.admin, id, app.ReadingChange{Version: 1, Text: &text}); !errors.Is(err, app.ErrVersionConflict) {
		t.Errorf("stale update: %v", err)
	}
	// The display may change but must still name the same passage.
	disp := "Yohanes 3:16"
	if up, err := e.readings.Update(e.ctx, e.admin, id, app.ReadingChange{Version: 2, ReferenceDisplay: &disp}); err != nil || up.Reading.ReferenceDisplay != "Yohanes 3:16" {
		t.Errorf("display: %+v %v", up, err)
	}
	other := "Yohanes 3:17"
	if _, err := e.readings.Update(e.ctx, e.admin, id, app.ReadingChange{Version: 3, ReferenceDisplay: &other}); invalidField(err) != "reference_display" {
		t.Errorf("display of another passage: %v", err)
	}
	empty := " "
	if _, err := e.readings.Update(e.ctx, e.admin, id, app.ReadingChange{Version: 3, Text: &empty}); invalidField(err) != "text" {
		t.Errorf("empty text: %v", err)
	}
	if got, _ := e.readings.Get(e.ctx, e.admin, id); got.Reading.Version != 3 || got.Reading.Text != "dua" {
		t.Errorf("failed updates changed the reading: %+v", got.Reading)
	}

	lid := e.draft()
	e.useReading(lid, id)
	if err := e.readings.Delete(e.ctx, e.admin, id); !errors.Is(err, app.ErrReadingInUse) {
		t.Errorf("delete in use: %v", err)
	}
	if _, err := e.readings.Get(e.ctx, e.admin, id); err != nil {
		t.Errorf("a reading in use must stay: %v", err)
	}
	if err := e.liturgies.Delete(e.ctx, e.admin, lid); err != nil {
		t.Fatal(err)
	}
	if err := e.readings.Delete(e.ctx, e.admin, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readings.Get(e.ctx, e.admin, id); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if err := e.readings.Delete(e.ctx, e.admin, id); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

// IT-R-003: stored reading first, then providers; storing is a server decision.
func TestReadingLookupAndFromProvider(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	storing, readOnly := good("licensed", true), good("readonly", false)
	u := e.withProviders(time.Second, readOnly, storing)

	// Nothing stored: the first provider that has text wins, may_store included.
	res, err := u.Lookup(e.ctx, e.admin, "Yoh 3:16", "TB")
	if err != nil || res.Reading != nil || res.Provider == nil || res.Provider.Source != "readonly" || res.Provider.MayStore || res.ProviderError {
		t.Fatalf("provider lookup: %+v %v", res, err)
	}
	if res.Reference.String() != "JHN 3:16" || res.Translation.Code != "TB" || res.Display != "Yoh 3:16" {
		t.Errorf("lookup result: %+v", res)
	}

	// A provider that forbids storing: nothing is saved.
	if _, err := u.FromProvider(e.ctx, e.admin, "Yoh 3:16", "TB", "readonly"); invalidField(err) != "provider" {
		t.Errorf("may_store false: %v", err)
	}
	if _, err := u.FromProvider(e.ctx, e.admin, "Yoh 3:16", "TB", "nobody"); invalidField(err) != "provider" {
		t.Errorf("unknown provider: %v", err)
	}
	if list, _ := u.List(e.ctx, e.admin, app.ReadingQuery{}); list.Total != 0 {
		t.Fatalf("nothing should be saved yet: %+v", list)
	}

	// A provider that allows it: the server saves the provider's text.
	v, err := u.FromProvider(e.ctx, e.admin, "Yoh 3:16", "TB", "licensed")
	if err != nil || v.Reading.SourceProvider != "licensed" || v.Reading.Text != "Karena begitu besar kasih Allah" || v.Reading.Attribution != "Dari licensed" {
		t.Fatalf("from provider: %+v %v", v, err)
	}
	// Now the stored reading is found first and the providers are not asked.
	before := storing.calls()
	res, err = u.Lookup(e.ctx, e.admin, "Yohanes 3:16", "TB")
	if err != nil || res.Reading == nil || res.Reading.ID != v.Reading.ID || res.Provider != nil || storing.calls() != before {
		t.Errorf("stored first: %+v %v", res, err)
	}
	if _, err := u.FromProvider(e.ctx, e.admin, "Yoh 3:16", "TB", "licensed"); !errors.As(err, new(*app.ReadingExistsError)) {
		t.Errorf("saving twice: %v", err)
	}

	// Typed text is always "manual".
	m, err := u.Create(e.ctx, e.admin, app.ReadingInput{Reference: "Yoh 3:17", Translation: "TB", Text: "teks"})
	if err != nil || m.Reading.SourceProvider != "manual" {
		t.Errorf("manual: %+v %v", m, err)
	}

	// Providers that have nothing: reading null, provider null, no error flag.
	none := &fakeProvider{id: "none", err: app.ErrNotAvailable}
	res, err = e.withProviders(time.Second, none).Lookup(e.ctx, e.admin, "Mzm 23", "TB")
	if err != nil || res.Reading != nil || res.Provider != nil || res.ProviderError {
		t.Errorf("not available: %+v %v", res, err)
	}
	res, err = e.readings.Lookup(e.ctx, e.admin, "Mzm 23", "") // no provider registered at all
	if err != nil || res.Provider != nil || res.Translation.Code != "TB" {
		t.Errorf("no providers: %+v %v", res, err)
	}
}

// IT-R-008: a provider that fails is skipped; a later one is still asked.
func TestReadingProviderFailures(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	hang := &fakeProvider{id: "hang", block: true}
	broken := &fakeProvider{id: "broken", err: errors.New("connection refused")}
	wrongSource := good("wrong", true)
	wrongSource.text.Source = "somebody-else"
	tooLong := good("long", true)
	tooLong.text.Text = strings.Repeat("a", domain.MaxReadingText+1)
	longAttr := good("attr", true)
	longAttr.text.Attribution = strings.Repeat("a", domain.MaxReadingAttribution+1)
	ok := good("ok", true)

	start := time.Now()
	u := e.withProviders(100*time.Millisecond, hang, broken, wrongSource, tooLong, longAttr)
	res, err := u.Lookup(e.ctx, e.admin, "Yoh 3:16", "TB")
	if err != nil || res.Provider != nil || !res.ProviderError {
		t.Errorf("all failed: %+v %v", res, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("a hanging provider held the lookup for %v", d)
	}
	for _, p := range []*fakeProvider{broken, wrongSource, tooLong, longAttr} {
		if p.calls() != 1 {
			t.Errorf("%s asked %d times", p.id, p.calls())
		}
	}

	// A later provider with text clears the error flag.
	u = e.withProviders(100*time.Millisecond, hang, broken, ok)
	res, err = u.Lookup(e.ctx, e.admin, "Yoh 3:16", "TB")
	if err != nil || res.Provider == nil || res.Provider.Source != "ok" || res.ProviderError {
		t.Errorf("a later provider: %+v %v", res, err)
	}
	// Saving from a failing provider is refused, nothing stored.
	if _, err := u.FromProvider(e.ctx, e.admin, "Yoh 3:16", "TB", "broken"); invalidField(err) != "provider" {
		t.Errorf("from a failing provider: %v", err)
	}
	if list, _ := u.List(e.ctx, e.admin, app.ReadingQuery{}); list.Total != 0 {
		t.Errorf("saved from a failing provider: %+v", list)
	}
	// A canceled request stops the wait.
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	if res, err := e.withProviders(5*time.Second, hang).Lookup(ctx, e.admin, "Yoh 3:16", "TB"); err == nil && res.Provider != nil {
		t.Errorf("canceled lookup returned text: %+v", res)
	}
}

// IT-R-004: the attribution typed last is suggested again, per translation.
func TestSuggestedAttribution(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	for i, ref := range []string{"Mzm 1", "Mzm 2"} {
		e.clock.now = e.clock.now.Add(time.Minute)
		if _, err := e.readings.Create(e.ctx, e.admin, app.ReadingInput{Reference: ref, Translation: "TB", Text: "t", Attribution: []string{"Lama", "Baru"}[i]}); err != nil {
			t.Fatal(err)
		}
	}
	if res, _ := e.readings.Lookup(e.ctx, e.admin, "Mzm 3", "TB"); res.SuggestedAttribution != "Baru" {
		t.Errorf("TB suggestion %q", res.SuggestedAttribution)
	}
	if res, _ := e.readings.Lookup(e.ctx, e.admin, "Mzm 3", "BIS"); res.SuggestedAttribution != "" {
		t.Errorf("BIS suggestion %q", res.SuggestedAttribution)
	}
	// Editing the older reading makes it the most recent.
	list, _ := e.readings.List(e.ctx, e.admin, app.ReadingQuery{})
	first := list.Items[0] // Mzm 1
	e.clock.now = e.clock.now.Add(time.Minute)
	attr := "Terbaru"
	if _, err := e.readings.Update(e.ctx, e.admin, first.ID, app.ReadingChange{Version: 1, Attribution: &attr}); err != nil {
		t.Fatal(err)
	}
	if res, _ := e.readings.Lookup(e.ctx, e.admin, "Mzm 3", "TB"); res.SuggestedAttribution != "Terbaru" {
		t.Errorf("after edit %q", res.SuggestedAttribution)
	}
}

// IT-R-005: order by book, chapter, verse; search; filter; paging.
func TestReadingList(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	for _, in := range []app.ReadingInput{
		{Reference: "Why 22:20", Translation: "TB", Text: "Ya, datanglah"},
		{Reference: "Mzm 23", Translation: "TB", Text: "TUHAN adalah gembalaku"},
		{Reference: "Yoh 3:16", Translation: "TB", Text: "Karena begitu besar kasih Allah akan dunia ini"},
		{Reference: "Yoh 3:16", Translation: "BIS", Text: "Allah sangat mengasihi manusia"},
		{Reference: "Yoh 3:2", Translation: "TB", Text: "orang itu datang"},
		{Reference: "Mzm 9:10", Translation: "TB", Text: "Orang yang mengenal nama-Mu"},
		{Reference: "Kej 1:1", Translation: "TB", Text: "Pada mulanya Allah menciptakan"},
		{Reference: "Yoh 1", Translation: "TB", Text: "Pada mulanya adalah Firman"},
	} {
		if _, err := e.readings.Create(e.ctx, e.admin, in); err != nil {
			t.Fatal(err)
		}
	}
	const all = "GEN 1:1/TB PSA 9:10/TB PSA 23/TB JHN 1/TB JHN 3:2/TB JHN 3:16/BIS JHN 3:16/TB REV 22:20/TB"
	refs := func(q app.ReadingQuery) string {
		res, err := e.readings.List(e.ctx, e.admin, q)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range res.Items {
			out = append(out, it.Reference+"/"+it.Translation.Code)
		}
		return strings.Join(out, " ")
	}
	if got, want := refs(app.ReadingQuery{}), all; got != want {
		t.Errorf("order:\n got %s\nwant %s", got, want)
	}
	for q, want := range map[string]string{
		"yohanes":          "JHN 1/TB JHN 3:2/TB JHN 3:16/BIS JHN 3:16/TB", // the Indonesian book name
		"yoh 3 16":         "JHN 3:16/BIS JHN 3:16/TB",
		"GEMBALAKU":        "PSA 23/TB",
		"mulanya":          "GEN 1:1/TB JHN 1/TB",
		"nama mu":          "PSA 9:10/TB", // punctuation folds to a space
		"tidak ada":        "",
		"%":                all, // folds to nothing, like any query without letters or digits: lists everything
		"_":                all,
		"100%":             "", // a digit and a percent sign: only the digits are searched, and no text has them
		"kasih allah akan": "JHN 3:16/TB",
	} {
		if got := refs(app.ReadingQuery{Q: q}); got != want {
			t.Errorf("q=%q: got %q, want %q", q, got, want)
		}
	}
	if got := refs(app.ReadingQuery{Translation: "BIS"}); got != "JHN 3:16/BIS" {
		t.Errorf("BIS filter: %q", got)
	}
	page, _ := e.readings.List(e.ctx, e.admin, app.ReadingQuery{Limit: 3, Offset: 2})
	if page.Total != 8 || len(page.Items) != 3 || page.Items[0].Reference != "PSA 23" {
		t.Errorf("paging: %+v", page)
	}
	if page.Items[0].Canonical != "Mazmur 23" || page.Items[0].Snippet != "TUHAN adalah gembalaku" || !page.Items[0].Actions.Edit {
		t.Errorf("summary: %+v", page.Items[0])
	}
	if _, err := e.readings.List(e.ctx, e.admin, app.ReadingQuery{Q: strings.Repeat("a", 201)}); invalidField(err) != "q" {
		t.Errorf("long query: %v", err)
	}
}

// Chinese text has no word boundaries: it is folded and searched without spaces.
func TestReadingChineseSearch(t *testing.T) {
	e := newChurch(t, sqlstoretest.NewSQLite(t), nil)
	for _, in := range []app.ReadingInput{
		{Reference: "Yoh 3:16", Translation: "CUV", Text: "神爱世人，甚至将他的独生子赐给他们"},
		{Reference: "Yoh 3:16", Translation: "TB", Text: "Karena begitu besar kasih Allah"},
	} {
		if _, err := e.readings.Create(e.ctx, e.admin, in); err != nil {
			t.Fatal(err)
		}
	}
	for q, want := range map[string]int{"爱世人": 1, "爱 世 人": 1, "独生子": 1, "神": 1, "kasih": 1, "yoh 3 16": 2, "yohanes": 2, "爱人": 0} {
		res, err := e.readings.List(e.ctx, e.admin, app.ReadingQuery{Q: q})
		if err != nil || res.Total != want {
			t.Errorf("q=%q: total %d (%v), want %d", q, res.Total, err, want)
		}
	}
}
