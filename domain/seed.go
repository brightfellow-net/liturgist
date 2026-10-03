// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

// SeedKey marks the churches whose planning defaults were created (09 §3).
const SeedKey = "step3"

// SeedItem is an item of the starter template; Duty indexes SeedSet.Duties.
type SeedItem struct {
	Title string
	Type  ItemType
	Duty  int
}

// SeedSet is the editable defaults of one content language (09 §3). The
// words are a draft until the owner's review (Q-3.3).
type SeedSet struct {
	Duties       []string
	SingingParts []string
	TemplateName string
	Items        []SeedItem
}

// Indexes into SeedSet.Duties of the ready-made duties.
const (
	seedLiturgist = iota
	seedWorshipLeader
	seedMusician
	seedReader
	seedPreacher
	seedMultimedia
	seedCollector
)

func starterItems(titles [7]string) []SeedItem {
	return []SeedItem{
		{titles[0], ItemFreeText, seedLiturgist},
		{titles[1], ItemSong, seedWorshipLeader},
		{titles[2], ItemReading, seedReader},
		{titles[3], ItemPrayer, seedLiturgist},
		{titles[4], ItemSermon, seedPreacher},
		{titles[5], ItemOther, seedCollector},
		{titles[6], ItemFreeText, seedLiturgist},
	}
}

var seedSets = map[string]SeedSet{
	"id": {
		Duties:       []string{"Liturgis", "Pemandu Pujian", "Pemusik", "Pembaca Alkitab", "Pengkhotbah", "Multimedia", "Kolektan"},
		SingingParts: []string{"Semua", "Pemandu", "Jemaat", "Pria", "Wanita", "Paduan Suara"},
		TemplateName: "Ibadah Minggu",
		Items: starterItems([7]string{"Votum dan Salam", "Pujian", "Pembacaan Alkitab", "Doa Syafaat",
			"Khotbah", "Persembahan", "Berkat"}),
	},
	"en": {
		Duties:       []string{"Liturgist", "Worship leader", "Musician", "Scripture reader", "Preacher", "Multimedia", "Offering collector"},
		SingingParts: []string{"All", "Leader", "Congregation", "Men", "Women", "Choir"},
		TemplateName: "Sunday service",
		Items: starterItems([7]string{"Votum and greeting", "Praise", "Scripture reading", "Intercessory prayer",
			"Sermon", "Offering", "Blessing"}),
	},
	"zh-Hans": {
		Duties:       []string{"主礼", "领唱", "乐手", "读经者", "讲道者", "多媒体", "司献"},
		SingingParts: []string{"全体", "领唱", "会众", "男声", "女声", "诗班"},
		TemplateName: "主日崇拜",
		Items:        starterItems([7]string{"宣召与问安", "赞美", "读经", "代祷", "讲道", "奉献", "祝福"}),
	},
	"zh-Hant": {
		Duties:       []string{"主禮", "領唱", "樂手", "讀經者", "講道者", "多媒體", "司獻"},
		SingingParts: []string{"全體", "領唱", "會眾", "男聲", "女聲", "詩班"},
		TemplateName: "主日崇拜",
		Items:        starterItems([7]string{"宣召與問安", "讚美", "讀經", "代禱", "講道", "奉獻", "祝福"}),
	},
}

// SeedFor returns the defaults for a content language; an unknown language
// gets the Indonesian set.
func SeedFor(language string) SeedSet {
	if s, ok := seedSets[language]; ok {
		return s
	}
	return seedSets["id"]
}
