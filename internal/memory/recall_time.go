package memory

import (
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// RecallTimeReference is a deterministic reading of one explicit past time
// reference in a request ("yesterday", "last week", "three days ago", "on
// Friday", "in March", "2026-09-14"). It names a calendar range relative to
// the request time; Window resolves it in the owner's time zone. No model
// proposes it, and it never widens what a reader may see: retrieval uses the
// window only to prefer evidence recorded (or, for accepted Claims, valid)
// inside it (memory architecture Stage 6).
//
// The zero value names no time.
type RecallTimeReference struct {
	// Kind names the expression family ("yesterday", "last_week", "weekday",
	// "date", ...). It is content-free and safe for diagnostics.
	Kind string `json:"kind,omitempty"`
	// Expression is the matched request text, lower-cased.
	Expression string `json:"expression,omitempty"`
	// Words are the expression's lower-cased words. They say when, not what,
	// so a search inside the window does not also require them to match.
	Words []string `json:"words,omitempty"`

	unit        recallTimeUnit
	near, far   int // units back from the current one: the window is [start-far, start-near+1)
	weekday     time.Weekday
	weekdayMode recallWeekdayMode
	month       time.Month
	monthMode   recallMonthMode
	day, year   int
}

// RecallTimeWindow is a resolved reference: the half-open UTC range
// [From, To) that the expression names.
type RecallTimeWindow struct {
	Expression string    `json:"expression"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
}

type recallTimeUnit int

const (
	recallUnitNone recallTimeUnit = iota
	recallUnitDay
	recallUnitWeek
	recallUnitWeekend
	recallUnitMonth
	recallUnitYear
	recallUnitNight
	recallUnitWeekday
	recallUnitNamedMonth
	recallUnitDate
	recallUnitNamedYear
)

type recallWeekdayMode int

const (
	recallWeekdayMostRecent recallWeekdayMode = iota // "on Friday": the latest one before today
	recallWeekdayLastWeek                            // "last Friday": the one in the previous week
	recallWeekdayThisWeek                            // "this Friday": this week's, if already past
)

type recallMonthMode int

const (
	recallMonthLatest recallMonthMode = iota // "in March": the latest March that has begun
	recallMonthBefore                        // "last March": the latest March before this month
	recallMonthYear                          // "March 2026": that year's
)

type recallToken struct {
	start, end int
	word       string
}

// Words that say a request is about the past. A weekday, a month, a date
// without a year, "today" and "this week/month/year" can name the future
// ("What should I cook on Friday?"); they are read as past only in a request
// that also uses one of these, or a regular past form (a word of five or more
// letters ending in "ed", other than "-eed").
const recallPastCues = " did didn didnt was wasn wasnt were weren werent had hadn hadnt said told mentioned talked discussed asked answered decided agreed went made sent wrote bought paid met saw ate came took gave found picked booked ordered signed renewed arranged chose tried called emailed texted finished started visited cooked ran spent received recommended suggested happened noted remembered shared posted fixed moved changed cancelled canceled returned lost broke "

// Words ending in "ed" that are not past forms.
const recallNotPastEd = " hundred embed sacred naked wicked beloved kindred rugged ragged crooked "

// A word just before an ambiguous name that points it at the future.
const recallFutureMarkers = " next coming upcoming for by until till before after from "

var recallNumberWords = map[string]int{"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12}

var recallWeekdays = map[string]time.Weekday{"monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday, "sunday": time.Sunday}

// Full month names. Abbreviations are months only beside a day or a year
// ("Sept 14", "Jan 2026"); alone they are too often names or other words.
var recallMonths = map[string]time.Month{"january": time.January, "february": time.February, "march": time.March, "april": time.April,
	"may": time.May, "june": time.June, "july": time.July, "august": time.August, "september": time.September,
	"october": time.October, "november": time.November, "december": time.December}

var recallMonthAbbreviations = map[string]time.Month{"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"jun": time.June, "jul": time.July, "aug": time.August, "sep": time.September, "sept": time.September,
	"oct": time.October, "nov": time.November, "dec": time.December}

// Prepositions that make a full month name a time ("in March", "last May").
const recallMonthPrepositions = " in during last early late mid "

// FindRecallTimeReference returns the first supported past time reference in
// text. It reads words only; Window decides whether the reference names a
// past range at a given request time.
func FindRecallTimeReference(text string) (RecallTimeReference, bool) {
	if !utf8.ValidString(text) {
		return RecallTimeReference{}, false
	}
	tokens := recallTokens(text)
	past := false
	for _, token := range tokens {
		if strings.Contains(recallPastCues, " "+token.word+" ") ||
			len(token.word) >= 5 && strings.HasSuffix(token.word, "ed") && !strings.HasSuffix(token.word, "eed") && !strings.Contains(recallNotPastEd, " "+token.word+" ") {
			past = true
			break
		}
	}
	word := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return tokens[i].word
	}
	for i := range tokens {
		reference, last, ok := matchRecallTime(text, tokens, i, word, past)
		if !ok {
			continue
		}
		reference.Expression = strings.ToLower(text[tokens[i].start:tokens[last].end])
		for _, token := range tokens[i : last+1] {
			reference.Words = append(reference.Words, token.word)
		}
		return reference, true
	}
	return RecallTimeReference{}, false
}

func recallTokens(text string) []recallToken {
	var tokens []recallToken
	start := -1
	for offset, r := range text {
		alnum := unicode.IsLetter(r) || unicode.IsDigit(r)
		if alnum && start < 0 {
			start = offset
		}
		if !alnum && start >= 0 {
			tokens = append(tokens, recallToken{start, offset, strings.ToLower(text[start:offset])})
			start = -1
		}
	}
	if start >= 0 {
		tokens = append(tokens, recallToken{start, len(text), strings.ToLower(text[start:])})
	}
	return tokens
}

// matchRecallTime tries every supported pattern that starts at token i and
// returns the reference and the index of its last token.
func matchRecallTime(text string, tokens []recallToken, i int, word func(int) string, past bool) (RecallTimeReference, int, bool) {
	w := word(i)
	future := strings.Contains(recallFutureMarkers, " "+word(i-1)+" ")
	relative := func(kind string, unit recallTimeUnit, near, far, last int) (RecallTimeReference, int, bool) {
		return RecallTimeReference{Kind: kind, unit: unit, near: near, far: far}, last, true
	}
	// Dates: an optional "on", then ISO, "<month> <day>[, <year>]" or
	// "<day> <month> [<year>]".
	dateStart := i
	if w == "on" {
		dateStart = i + 1
	}
	if reference, last, ok := matchRecallDate(text, tokens, dateStart, word); ok && (reference.year != 0 || past && !future) {
		return reference, last, true
	}
	switch w {
	case "the":
		if word(i+1) == "day" && word(i+2) == "before" && word(i+3) == "yesterday" {
			return relative("day_before_yesterday", recallUnitDay, 2, 2, i+3)
		}
	case "yesterday":
		return relative("yesterday", recallUnitDay, 1, 1, i)
	case "today":
		if past && !future {
			return relative("today", recallUnitDay, 0, 0, i)
		}
	case "earlier":
		switch {
		case word(i+1) == "today":
			return relative("today", recallUnitDay, 0, 0, i+1)
		case word(i+1) == "this" && word(i+2) == "week":
			return relative("this_week", recallUnitWeek, 0, 0, i+2)
		case word(i+1) == "this" && word(i+2) == "month":
			return relative("this_month", recallUnitMonth, 0, 0, i+2)
		case word(i+1) == "this" && word(i+2) == "year":
			return relative("this_year", recallUnitYear, 0, 0, i+2)
		}
	case "this":
		switch word(i + 1) {
		case "morning":
			return relative("today", recallUnitDay, 0, 0, i+1)
		case "week":
			if past && !future {
				return relative("this_week", recallUnitWeek, 0, 0, i+1)
			}
		case "month":
			if past && !future {
				return relative("this_month", recallUnitMonth, 0, 0, i+1)
			}
		case "year":
			if past && !future {
				return relative("this_year", recallUnitYear, 0, 0, i+1)
			}
		}
		if weekday, ok := recallWeekdays[word(i+1)]; ok && past && !future {
			return RecallTimeReference{Kind: "weekday", unit: recallUnitWeekday, weekday: weekday, weekdayMode: recallWeekdayThisWeek}, i + 1, true
		}
	case "last", "previous":
		switch word(i + 1) {
		case "night":
			if w == "last" {
				return RecallTimeReference{Kind: "last_night", unit: recallUnitNight}, i + 1, true
			}
		case "week":
			return relative("last_week", recallUnitWeek, 1, 1, i+1)
		case "weekend":
			return relative("last_weekend", recallUnitWeekend, 1, 1, i+1)
		case "month":
			return relative("last_month", recallUnitMonth, 1, 1, i+1)
		case "year":
			return relative("last_year", recallUnitYear, 1, 1, i+1)
		}
		if weekday, ok := recallWeekdays[word(i+1)]; ok {
			return RecallTimeReference{Kind: "weekday", unit: recallUnitWeekday, weekday: weekday, weekdayMode: recallWeekdayLastWeek}, i + 1, true
		}
		if month, ok := recallMonths[word(i+1)]; ok && w == "last" {
			return RecallTimeReference{Kind: "month", unit: recallUnitNamedMonth, month: month, monthMode: recallMonthBefore}, i + 1, true
		}
	case "on":
		if weekday, ok := recallWeekdays[word(i+1)]; ok && past && !future {
			return RecallTimeReference{Kind: "weekday", unit: recallUnitWeekday, weekday: weekday, weekdayMode: recallWeekdayMostRecent}, i + 1, true
		}
	case "in", "during":
		if year, ok := recallYear(word(i + 1)); ok {
			return RecallTimeReference{Kind: "year", unit: recallUnitNamedYear, year: year}, i + 1, true
		}
	}
	// "<count> <unit>(s) ago", with "a couple of" and "a few" as ranges.
	if reference, last, ok := matchRecallAgo(i, word); ok {
		return reference, last, true
	}
	// "in|during|early|late|mid <month> [<year>]" and "<month> <year>".
	monthAt, monthPrefixed := i, false
	if strings.Contains(recallMonthPrepositions, " "+w+" ") && w != "last" {
		monthAt, monthPrefixed = i+1, true
	}
	if month, ok := recallMonths[word(monthAt)]; ok && (monthPrefixed || monthAt == i) {
		if year, ok := recallYear(word(monthAt + 1)); ok {
			return RecallTimeReference{Kind: "month", unit: recallUnitNamedMonth, month: month, year: year, monthMode: recallMonthYear}, monthAt + 1, true
		}
		if monthPrefixed && past && !future {
			return RecallTimeReference{Kind: "month", unit: recallUnitNamedMonth, month: month, monthMode: recallMonthLatest}, monthAt, true
		}
	}
	if month, ok := recallMonthAbbreviations[w]; ok && monthAt == i {
		if year, ok := recallYear(word(i + 1)); ok {
			return RecallTimeReference{Kind: "month", unit: recallUnitNamedMonth, month: month, year: year, monthMode: recallMonthYear}, i + 1, true
		}
	}
	return RecallTimeReference{}, 0, false
}

func matchRecallAgo(i int, word func(int) string) (RecallTimeReference, int, bool) {
	near, far, at := 0, 0, i
	switch {
	case word(i) == "a" && word(i+1) == "couple" && word(i+2) == "of":
		near, far, at = 2, 3, i+3
	case word(i) == "a" && word(i+1) == "few":
		near, far, at = 2, 5, i+2
	default:
		n, ok := recallNumberWords[word(i)]
		if !ok {
			value, err := strconv.Atoi(word(i))
			if err != nil || value < 1 || value > 99 || strconv.Itoa(value) != word(i) {
				return RecallTimeReference{}, 0, false
			}
			n = value
		}
		near, far, at = n, n, i+1
	}
	if word(at+1) != "ago" {
		return RecallTimeReference{}, 0, false
	}
	unit := strings.TrimSuffix(word(at), "s")
	kinds := map[string]recallTimeUnit{"day": recallUnitDay, "week": recallUnitWeek, "month": recallUnitMonth, "year": recallUnitYear}
	u, ok := kinds[unit]
	if !ok || (near != far && u != recallUnitDay && u != recallUnitWeek) {
		return RecallTimeReference{}, 0, false
	}
	return RecallTimeReference{Kind: unit + "s_ago", unit: u, near: near, far: far}, at + 1, true
}

// matchRecallDate reads an ISO date ("2026-09-14"), "<month> <day>[,]
// [<year>]" or "<day> <month> [<year>]" starting at token i. A year of zero
// means the date named none.
func matchRecallDate(text string, tokens []recallToken, i int, word func(int) string) (RecallTimeReference, int, bool) {
	if i >= len(tokens) {
		return RecallTimeReference{}, 0, false
	}
	separated := func(a int, separator string) bool {
		return a+1 < len(tokens) && text[tokens[a].end:tokens[a+1].start] == separator
	}
	if year, ok := recallYear(word(i)); ok && len(word(i+1)) == 2 && len(word(i+2)) == 2 && separated(i, "-") && separated(i+1, "-") {
		month, errMonth := strconv.Atoi(word(i + 1))
		day, errDay := strconv.Atoi(word(i + 2))
		if errMonth == nil && errDay == nil && validRecallDate(year, time.Month(month), day) {
			return RecallTimeReference{Kind: "date", unit: recallUnitDate, year: year, month: time.Month(month), day: day}, i + 2, true
		}
		return RecallTimeReference{}, 0, false
	}
	month := func(w string) (time.Month, bool) {
		if m, ok := recallMonths[w]; ok {
			return m, true
		}
		m, ok := recallMonthAbbreviations[w]
		return m, ok
	}
	if m, ok := month(word(i)); ok {
		if day, ok := recallDayOfMonth(word(i + 1)); ok {
			last := i + 1
			year, hasYear := recallYear(word(i + 2))
			if hasYear {
				last = i + 2
			} else {
				year = 0
			}
			if year == 0 || validRecallDate(year, m, day) {
				return RecallTimeReference{Kind: "date", unit: recallUnitDate, year: year, month: m, day: day}, last, true
			}
		}
	}
	if day, ok := recallDayOfMonth(word(i)); ok {
		if m, ok := month(word(i + 1)); ok {
			last := i + 1
			year, hasYear := recallYear(word(i + 2))
			if hasYear {
				last = i + 2
			} else {
				year = 0
			}
			if year == 0 || validRecallDate(year, m, day) {
				return RecallTimeReference{Kind: "date", unit: recallUnitDate, year: year, month: m, day: day}, last, true
			}
		}
	}
	return RecallTimeReference{}, 0, false
}

func recallYear(w string) (int, bool) {
	if len(w) != 4 {
		return 0, false
	}
	year, err := strconv.Atoi(w)
	return year, err == nil && year >= 1900 && year <= 2999
}

func recallDayOfMonth(w string) (int, bool) {
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if trimmed, ok := strings.CutSuffix(w, suffix); ok && trimmed != "" {
			w = trimmed
			break
		}
	}
	if len(w) == 0 || len(w) > 2 {
		return 0, false
	}
	day, err := strconv.Atoi(w)
	return day, err == nil && day >= 1 && day <= 31
}

func validRecallDate(year int, month time.Month, day int) bool {
	if month < time.January || month > time.December || day < 1 {
		return false
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Day() == day
}

// Window resolves the reference at the request time now in the owner's
// zone (UTC when loc is nil). It reports false for the zero reference and
// for a range that has not begun, since only the past has been recorded.
// Weeks begin on Monday.
func (r RecallTimeReference) Window(now time.Time, loc *time.Location) (RecallTimeWindow, bool) {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	weekStart := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	yearStart := time.Date(local.Year(), time.January, 1, 0, 0, 0, 0, loc)
	var from, to time.Time
	switch r.unit {
	case recallUnitDay:
		from, to = today.AddDate(0, 0, -r.far), today.AddDate(0, 0, -r.near+1)
	case recallUnitWeek:
		from, to = weekStart.AddDate(0, 0, -7*r.far), weekStart.AddDate(0, 0, -7*(r.near-1))
	case recallUnitWeekend:
		from = weekStart.AddDate(0, 0, 5-7*r.far)
		to = from.AddDate(0, 0, 2)
	case recallUnitMonth:
		from, to = monthStart.AddDate(0, -r.far, 0), monthStart.AddDate(0, -(r.near-1), 0)
	case recallUnitYear:
		from, to = yearStart.AddDate(-r.far, 0, 0), yearStart.AddDate(-(r.near-1), 0, 0)
	case recallUnitNight:
		from, to = today.AddDate(0, 0, -1).Add(18*time.Hour), today.Add(6*time.Hour)
	case recallUnitWeekday:
		offset := (int(r.weekday) + 6) % 7 // days after Monday
		switch r.weekdayMode {
		case recallWeekdayLastWeek:
			from = weekStart.AddDate(0, 0, offset-7)
		case recallWeekdayThisWeek:
			from = weekStart.AddDate(0, 0, offset)
			if !from.Before(today) {
				return RecallTimeWindow{}, false
			}
		default:
			back := (int(today.Weekday()) - int(r.weekday) + 7) % 7
			if back == 0 {
				back = 7
			}
			from = today.AddDate(0, 0, -back)
		}
		to = from.AddDate(0, 0, 1)
	case recallUnitNamedMonth:
		year := r.year
		switch r.monthMode {
		case recallMonthLatest:
			year = local.Year()
			if r.month > local.Month() {
				year--
			}
		case recallMonthBefore:
			year = local.Year()
			if r.month >= local.Month() {
				year--
			}
		}
		from = time.Date(year, r.month, 1, 0, 0, 0, 0, loc)
		to = from.AddDate(0, 1, 0)
	case recallUnitDate:
		year := r.year
		if year == 0 {
			year = local.Year()
			if time.Date(year, r.month, r.day, 0, 0, 0, 0, loc).After(today) {
				year--
			}
			if !validRecallDate(year, r.month, r.day) {
				return RecallTimeWindow{}, false
			}
		}
		from = time.Date(year, r.month, r.day, 0, 0, 0, 0, loc)
		to = from.AddDate(0, 0, 1)
	case recallUnitNamedYear:
		from = time.Date(r.year, time.January, 1, 0, 0, 0, 0, loc)
		to = from.AddDate(1, 0, 0)
	default:
		return RecallTimeWindow{}, false
	}
	if !from.Before(to) || from.After(now) {
		return RecallTimeWindow{}, false
	}
	return RecallTimeWindow{Expression: r.Expression, From: from.UTC(), To: to.UTC()}, true
}
