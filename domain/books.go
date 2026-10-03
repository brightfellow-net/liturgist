// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import "strings"

// Book is one of the 66 books of the Bible (07 §2.4).
type Book struct {
	Ordinal int    // 1..66, canonical order
	Code    string // USFM code
	Name    string // Indonesian name (LAI, Terjemahan Baru)
	Abbr    string // LAI abbreviation
}

// Books lists the books in canonical order. The table was confirmed by the
// owner on 2026-10-02 (07 §2.4).
var Books = []Book{
	{1, "GEN", "Kejadian", "Kej"}, {2, "EXO", "Keluaran", "Kel"}, {3, "LEV", "Imamat", "Im"},
	{4, "NUM", "Bilangan", "Bil"}, {5, "DEU", "Ulangan", "Ul"}, {6, "JOS", "Yosua", "Yos"},
	{7, "JDG", "Hakim-hakim", "Hak"}, {8, "RUT", "Rut", "Rut"}, {9, "1SA", "1 Samuel", "1Sam"},
	{10, "2SA", "2 Samuel", "2Sam"}, {11, "1KI", "1 Raja-raja", "1Raj"}, {12, "2KI", "2 Raja-raja", "2Raj"},
	{13, "1CH", "1 Tawarikh", "1Taw"}, {14, "2CH", "2 Tawarikh", "2Taw"}, {15, "EZR", "Ezra", "Ezr"},
	{16, "NEH", "Nehemia", "Neh"}, {17, "EST", "Ester", "Est"}, {18, "JOB", "Ayub", "Ayb"},
	{19, "PSA", "Mazmur", "Mzm"}, {20, "PRO", "Amsal", "Ams"}, {21, "ECC", "Pengkhotbah", "Pkh"},
	{22, "SNG", "Kidung Agung", "Kid"}, {23, "ISA", "Yesaya", "Yes"}, {24, "JER", "Yeremia", "Yer"},
	{25, "LAM", "Ratapan", "Rat"}, {26, "EZK", "Yehezkiel", "Yeh"}, {27, "DAN", "Daniel", "Dan"},
	{28, "HOS", "Hosea", "Hos"}, {29, "JOL", "Yoel", "Yl"}, {30, "AMO", "Amos", "Am"},
	{31, "OBA", "Obaja", "Ob"}, {32, "JON", "Yunus", "Yun"}, {33, "MIC", "Mikha", "Mi"},
	{34, "NAM", "Nahum", "Nah"}, {35, "HAB", "Habakuk", "Hab"}, {36, "ZEP", "Zefanya", "Zef"},
	{37, "HAG", "Hagai", "Hag"}, {38, "ZEC", "Zakharia", "Za"}, {39, "MAL", "Maleakhi", "Mal"},
	{40, "MAT", "Matius", "Mat"}, {41, "MRK", "Markus", "Mrk"}, {42, "LUK", "Lukas", "Luk"},
	{43, "JHN", "Yohanes", "Yoh"}, {44, "ACT", "Kisah Para Rasul", "Kis"}, {45, "ROM", "Roma", "Rm"},
	{46, "1CO", "1 Korintus", "1Kor"}, {47, "2CO", "2 Korintus", "2Kor"}, {48, "GAL", "Galatia", "Gal"},
	{49, "EPH", "Efesus", "Ef"}, {50, "PHP", "Filipi", "Flp"}, {51, "COL", "Kolose", "Kol"},
	{52, "1TH", "1 Tesalonika", "1Tes"}, {53, "2TH", "2 Tesalonika", "2Tes"}, {54, "1TI", "1 Timotius", "1Tim"},
	{55, "2TI", "2 Timotius", "2Tim"}, {56, "TIT", "Titus", "Tit"}, {57, "PHM", "Filemon", "Flm"},
	{58, "HEB", "Ibrani", "Ibr"}, {59, "JAS", "Yakobus", "Yak"}, {60, "1PE", "1 Petrus", "1Ptr"},
	{61, "2PE", "2 Petrus", "2Ptr"}, {62, "1JN", "1 Yohanes", "1Yoh"}, {63, "2JN", "2 Yohanes", "2Yoh"},
	{64, "3JN", "3 Yohanes", "3Yoh"}, {65, "JUD", "Yudas", "Yud"}, {66, "REV", "Wahyu", "Why"},
}

// extraAliases are further spellings of a book. Spellings found in the pilot
// church's documents are added here (07 §2.1); each must stay unique.
var extraAliases = map[string]string{
	"Kidung": "SNG",
	"Kisah":  "ACT",
	"Hakim":  "JDG",
}

// oneChapterBooks have a single chapter, so a passage without ":" names
// verses (07 §2.1).
var oneChapterBooks = map[string]bool{"OBA": true, "PHM": true, "2JN": true, "3JN": true, "JUD": true}

var (
	bookByCode  = map[string]*Book{}
	bookByAlias = map[string]*Book{}
)

func init() {
	for i := range Books {
		b := &Books[i]
		bookByCode[b.Code] = b
		for _, s := range []string{b.Code, b.Name, b.Abbr} {
			bookByAlias[bookKey(s)] = b
		}
	}
	for alias, code := range extraAliases {
		bookByAlias[bookKey(alias)] = bookByCode[code]
	}
}

// bookKey is how book spellings are compared: lower-case, without dots,
// spaces and hyphens (07 §2.1).
func bookKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r == '.' || r == ' ' || r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// BookByCode returns the book with a USFM code.
func BookByCode(code string) (Book, bool) {
	b, ok := bookByCode[code]
	if !ok {
		return Book{}, false
	}
	return *b, true
}

// BookAliases returns every spelling that names a book, in comparison form
// (for the uniqueness test).
func BookAliases() map[string]string {
	out := make(map[string]string, len(bookByAlias))
	for k, b := range bookByAlias {
		out[k] = b.Code
	}
	return out
}
