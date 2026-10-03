package memory

import (
	"slices"
	"testing"
	"time"
)

// recallTimeZone stands in for the owner's time zone: four hours behind UTC.
var recallTimeZone = time.FixedZone("UTC-4", -4*60*60)

// Thursday 1 October 2026, 18:00 in the owner's zone.
var recallTimeNow = time.Date(2026, time.October, 1, 18, 0, 0, 0, recallTimeZone)

func recallDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, recallTimeZone)
}

func TestRecallTimeWindowsForSupportedExpressions(t *testing.T) {
	for _, c := range []struct {
		text, expression string
		from, to         time.Time
	}{
		{"What did I book yesterday?", "yesterday", recallDay(2026, 9, 30), recallDay(2026, 10, 1)},
		{"Yesterday morning I said something about the boiler.", "yesterday", recallDay(2026, 9, 30), recallDay(2026, 10, 1)},
		{"what did we decide the day before yesterday", "the day before yesterday", recallDay(2026, 9, 29), recallDay(2026, 9, 30)},
		{"What did I tell you last night?", "last night", recallDay(2026, 9, 30).Add(18 * time.Hour), recallDay(2026, 10, 1).Add(6 * time.Hour)},
		{"Where did I leave the parcel earlier today?", "earlier today", recallDay(2026, 10, 1), recallDay(2026, 10, 2)},
		{"What did I say this morning", "this morning", recallDay(2026, 10, 1), recallDay(2026, 10, 2)},
		{"What did I say today?", "today", recallDay(2026, 10, 1), recallDay(2026, 10, 2)},
		{"What did I send the landlord three days ago?", "three days ago", recallDay(2026, 9, 28), recallDay(2026, 9, 29)},
		{"the thing from 3 days ago", "3 days ago", recallDay(2026, 9, 28), recallDay(2026, 9, 29)},
		{"what I mentioned a day ago", "a day ago", recallDay(2026, 9, 30), recallDay(2026, 10, 1)},
		{"a couple of days ago I asked about tyres", "a couple of days ago", recallDay(2026, 9, 28), recallDay(2026, 9, 30)},
		{"a few days ago", "a few days ago", recallDay(2026, 9, 26), recallDay(2026, 9, 30)},
		{"Which paint colour did I pick last week?", "last week", recallDay(2026, 9, 21), recallDay(2026, 9, 28)},
		{"the previous week", "previous week", recallDay(2026, 9, 21), recallDay(2026, 9, 28)},
		{"two weeks ago", "two weeks ago", recallDay(2026, 9, 14), recallDay(2026, 9, 21)},
		{"a week ago", "a week ago", recallDay(2026, 9, 21), recallDay(2026, 9, 28)},
		{"What did I do last weekend?", "last weekend", recallDay(2026, 9, 26), recallDay(2026, 9, 28)},
		{"What did I sign up for last month?", "last month", recallDay(2026, 9, 1), recallDay(2026, 10, 1)},
		{"two months ago", "two months ago", recallDay(2026, 8, 1), recallDay(2026, 9, 1)},
		{"last year", "last year", recallDay(2025, 1, 1), recallDay(2026, 1, 1)},
		{"earlier this week", "earlier this week", recallDay(2026, 9, 28), recallDay(2026, 10, 5)},
		{"earlier this month", "earlier this month", recallDay(2026, 10, 1), recallDay(2026, 11, 1)},
		{"what did I buy this week", "this week", recallDay(2026, 9, 28), recallDay(2026, 10, 5)},
		{"Which restaurant did Priya recommend on Friday?", "on friday", recallDay(2026, 9, 25), recallDay(2026, 9, 26)},
		{"What did I eat on Wednesday?", "on wednesday", recallDay(2026, 9, 30), recallDay(2026, 10, 1)},
		{"last Friday", "last friday", recallDay(2026, 9, 25), recallDay(2026, 9, 26)},
		{"last Wednesday", "last wednesday", recallDay(2026, 9, 23), recallDay(2026, 9, 24)},
		{"What did I decide in March?", "in march", recallDay(2026, 3, 1), recallDay(2026, 4, 1)},
		{"what was the plan in December", "in december", recallDay(2025, 12, 1), recallDay(2026, 1, 1)},
		{"what did I say in October", "in october", recallDay(2026, 10, 1), recallDay(2026, 11, 1)},
		{"last March", "last march", recallDay(2026, 3, 1), recallDay(2026, 4, 1)},
		{"in May 2025", "in may 2025", recallDay(2025, 5, 1), recallDay(2025, 6, 1)},
		{"employer March 2026", "march 2026", recallDay(2026, 3, 1), recallDay(2026, 4, 1)},
		{"How much was the bill I paid on 2026-09-14?", "on 2026-09-14", recallDay(2026, 9, 14), recallDay(2026, 9, 15)},
		{"What did I say on September 14?", "on september 14", recallDay(2026, 9, 14), recallDay(2026, 9, 15)},
		{"What happened on 14 September 2025", "on 14 september 2025", recallDay(2025, 9, 14), recallDay(2025, 9, 15)},
		{"What did I say on Sept 14th?", "on sept 14th", recallDay(2026, 9, 14), recallDay(2026, 9, 15)},
		{"What did we decide on December 25?", "on december 25", recallDay(2025, 12, 25), recallDay(2025, 12, 26)},
		{"What did I buy in 2025?", "in 2025", recallDay(2025, 1, 1), recallDay(2026, 1, 1)},
	} {
		reference, found := FindRecallTimeReference(c.text)
		if !found {
			t.Errorf("%q: no time reference", c.text)
			continue
		}
		if reference.Expression != c.expression {
			t.Errorf("%q: expression %q, want %q", c.text, reference.Expression, c.expression)
		}
		window, ok := reference.Window(recallTimeNow, recallTimeZone)
		if !ok {
			t.Errorf("%q: %q did not resolve to a past window", c.text, reference.Expression)
			continue
		}
		if !window.From.Equal(c.from) || !window.To.Equal(c.to) {
			t.Errorf("%q: window [%s, %s), want [%s, %s)", c.text, window.From.In(recallTimeZone), window.To.In(recallTimeZone), c.from, c.to)
		}
		if window.From.Location() != time.UTC || window.To.Location() != time.UTC || window.Expression != c.expression {
			t.Errorf("%q: window %+v is not UTC or lost its expression", c.text, window)
		}
	}
}

// Ambiguous names (a weekday, a month, a date without a year, "today", "this
// week") are read as past only in a request that speaks in the past and does
// not mark them as future. Expressions that refer to the future, to the
// present, or to nothing in time are not windows.
func TestRecallTimeIgnoresFutureAndNonTemporalWording(t *testing.T) {
	for _, text := range []string{
		"What should I cook on Friday?",
		"What did I plan for Friday?",
		"Book the car service before Friday.",
		"What's happening in March?",
		"When does my passport expire in March 2029?",
		"Plan a grocery list for lentil soup on Friday.",
		"Summarize the test results from today's CI run for the payments service.",
		"What's on this week?",
		"I may have mentioned the boiler.",
		"Jan said the boiler is fine.",
		"I'll see you next week.",
		"What's planned for tomorrow?",
		"Where do I work these days?",
		"What do I use nowadays?",
		"The last time I checked it was fine.",
		"At last the parcel arrived.",
		"Who came last in the race?",
		"What's 17 times 23?",
		"Remind me of my half-marathon goal pace.",
		"What did I book for next Friday?",
		"What did I book on 2026-10-05?",
		"Write a haiku about autumn.",
		"in two days",
		"since March",
		"it costs 2026 euros",
	} {
		reference, found := FindRecallTimeReference(text)
		if !found {
			continue
		}
		if window, ok := reference.Window(recallTimeNow, recallTimeZone); ok {
			t.Errorf("%q: read %q as the past window [%s, %s)", text, reference.Expression, window.From, window.To)
		}
	}
}

func TestRecallTimeWordsAndKindAreContentFree(t *testing.T) {
	reference, found := FindRecallTimeReference("How much was the electricity bill I paid on 2026-09-14?")
	if !found || reference.Kind != "date" || !slices.Equal(reference.Words, []string{"on", "2026", "09", "14"}) {
		t.Fatalf("ISO date reference = %+v %t", reference, found)
	}
	reference, found = FindRecallTimeReference("What did I send the landlord THREE days ago?")
	if !found || reference.Kind != "days_ago" || !slices.Equal(reference.Words, []string{"three", "days", "ago"}) {
		t.Fatalf("days ago reference = %+v %t", reference, found)
	}
	reference, found = FindRecallTimeReference("Which paint colour did I pick last week? Or was it yesterday?")
	if !found || reference.Expression != "last week" || reference.Kind != "last_week" {
		t.Fatalf("first reference = %+v %t", reference, found)
	}
	var none RecallTimeReference
	if _, ok := none.Window(recallTimeNow, recallTimeZone); ok {
		t.Fatal("the zero reference resolved to a window")
	}
}

// The owner's day boundaries come from the owner's zone, not UTC.
func TestRecallTimeUsesTheOwnersZone(t *testing.T) {
	now := time.Date(2026, time.October, 2, 2, 0, 0, 0, time.UTC) // 1 October, 22:00 at UTC-4
	reference, _ := FindRecallTimeReference("What did I book yesterday?")
	window, ok := reference.Window(now, recallTimeZone)
	if !ok || !window.From.Equal(time.Date(2026, 9, 30, 4, 0, 0, 0, time.UTC)) || !window.To.Equal(time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)) {
		t.Fatalf("yesterday at UTC-4 = %+v %t", window, ok)
	}
	window, ok = reference.Window(now, time.UTC)
	if !ok || !window.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("yesterday at UTC = %+v %t", window, ok)
	}
	if window, ok = reference.Window(now, nil); !ok || !window.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("a missing zone must read as UTC: %+v %t", window, ok)
	}
}

func TestRecallTimeMonthResolutionRelativeToNow(t *testing.T) {
	reference, _ := FindRecallTimeReference("last March")
	march := time.Date(2026, time.March, 10, 12, 0, 0, 0, recallTimeZone)
	if window, ok := reference.Window(march, recallTimeZone); !ok || !window.From.Equal(recallDay(2025, 3, 1)) {
		t.Fatalf("last March during March = %+v %t", window, ok)
	}
	reference, _ = FindRecallTimeReference("what did I decide in March")
	if window, ok := reference.Window(march, recallTimeZone); !ok || !window.From.Equal(recallDay(2026, 3, 1)) {
		t.Fatalf("in March during March = %+v %t", window, ok)
	}
	february := time.Date(2026, time.February, 10, 12, 0, 0, 0, recallTimeZone)
	if window, ok := reference.Window(february, recallTimeZone); !ok || !window.From.Equal(recallDay(2025, 3, 1)) {
		t.Fatalf("in March during February = %+v %t", window, ok)
	}
}
