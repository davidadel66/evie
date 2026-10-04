package workflow

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Workflow Schedules (Stage 14 sub-stage 11, ADR 0020 and 0023): due times
// are computed on the wall clock of the schedule's declared IANA timezone,
// never the host's, and daylight-saving changes are deterministic: a wall
// time that does not exist (spring forward) is due when the gap ends, and a
// wall time that happens twice (fall back) is due once, at its first
// occurrence. A slot's identity is its nominal local date and time.

func timetable(t *testing.T, cron, tz string) Timetable {
	t.Helper()
	tt, err := NewTimetable(cron, tz)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func after(s string) SlotCursor { return SlotCursor{At: utc(s)} }

func nextSlot(t *testing.T, tt Timetable, c SlotCursor) Slot {
	t.Helper()
	s, ok := tt.Next(c)
	if !ok {
		t.Fatalf("no slot after %v", c)
	}
	return s
}

func TestTimetableRejectsWhatValidationRejects(t *testing.T) {
	for _, bad := range []string{"* * * *", "MON * * * *", "60 * * * *", "0 24 * * *", "0 0 0 * *", "0 0 * 13 *", "0 0 * * 7", "*/0 * * * *"} {
		if _, err := NewTimetable(bad, "UTC"); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	for _, tz := range []string{"", "Local", "Mars/Olympus"} {
		if _, err := NewTimetable("0 7 * * *", tz); err == nil {
			t.Errorf("timezone %q was accepted", tz)
		}
	}
}

// Weekday mornings in New York: the slot is 07:00 local, whatever the host's
// timezone, and its key, date and time are nominal local values.
func TestTimetableDueTimesUseTheDeclaredTimezone(t *testing.T) {
	tt := timetable(t, "0 7 * * 1-5", "America/New_York")
	s := nextSlot(t, tt, after("2026-10-05T00:00:00Z")) // a Monday
	if !s.Due.Equal(utc("2026-10-05T11:00:00Z")) || s.Key != "2026-10-05T07:00" || s.Date != "2026-10-05" || s.Time != "07:00" || s.Shifted {
		t.Fatalf("first slot = %+v", s)
	}
	s = nextSlot(t, tt, SlotCursor{At: s.Due, Key: s.Key})
	if s.Key != "2026-10-06T07:00" {
		t.Fatalf("second slot = %+v", s)
	}
	// After Friday's slot comes Monday's.
	s = nextSlot(t, tt, after("2026-10-09T11:00:00Z"))
	if s.Key != "2026-10-12T07:00" || !s.Due.Equal(utc("2026-10-12T11:00:00Z")) {
		t.Fatalf("after Friday = %+v", s)
	}
	// The same cron in Kolkata (+05:30) and UTC.
	if s := nextSlot(t, timetable(t, "0 9 * * *", "Asia/Kolkata"), after("2026-10-05T00:00:00Z")); !s.Due.Equal(utc("2026-10-05T03:30:00Z")) {
		t.Fatalf("Kolkata slot = %+v", s)
	}
	if s := nextSlot(t, timetable(t, "0 9 * * *", "UTC"), after("2026-10-05T09:00:00Z")); s.Key != "2026-10-06T09:00" {
		t.Fatalf("a slot due exactly at the cursor is not after it: %+v", s)
	}
}

// A daily 07:00 schedule stays at 07:00 local across the end of daylight
// saving time: its UTC instant moves by an hour.
func TestTimetableFollowsTheWallClockAcrossDaylightSaving(t *testing.T) {
	tt := timetable(t, "0 7 * * *", "America/New_York")
	want := map[string]string{"2026-10-31T07:00": "2026-10-31T11:00:00Z", "2026-11-01T07:00": "2026-11-01T12:00:00Z", "2026-11-02T07:00": "2026-11-02T12:00:00Z"}
	c := after("2026-10-31T00:00:00Z")
	for range 3 {
		s := nextSlot(t, tt, c)
		if w, ok := want[s.Key]; !ok || !s.Due.Equal(utc(w)) {
			t.Fatalf("slot %+v; want one of %v", s, want)
		}
		c = SlotCursor{At: s.Due, Key: s.Key}
	}
}

// Spring forward: 02:30 does not exist on 2026-03-08 in New York, so that
// day's slot is due when the gap ends (03:00 EDT), keeps its nominal key,
// and is marked shifted. Two slots in the gap are both due then, in nominal
// order, and neither is lost.
func TestTimetableRunsANonexistentTimeWhenTheGapEnds(t *testing.T) {
	tt := timetable(t, "30 2 * * *", "America/New_York")
	s := nextSlot(t, tt, after("2026-03-07T12:00:00Z"))
	if s.Key != "2026-03-08T02:30" || !s.Due.Equal(utc("2026-03-08T07:00:00Z")) || !s.Shifted || s.Time != "02:30" {
		t.Fatalf("gap slot = %+v", s)
	}
	if s := nextSlot(t, tt, SlotCursor{At: s.Due, Key: s.Key}); s.Key != "2026-03-09T02:30" || !s.Due.Equal(utc("2026-03-09T06:30:00Z")) || s.Shifted {
		t.Fatalf("day after the gap = %+v", s)
	}
	half := timetable(t, "*/30 2 * * *", "America/New_York")
	first := nextSlot(t, half, after("2026-03-08T06:00:00Z"))
	second := nextSlot(t, half, SlotCursor{At: first.Due, Key: first.Key})
	if first.Key != "2026-03-08T02:00" || second.Key != "2026-03-08T02:30" || !first.Due.Equal(second.Due) || !first.Due.Equal(utc("2026-03-08T07:00:00Z")) {
		t.Fatalf("gap slots = %+v, %+v", first, second)
	}
	// A cursor at an instant (an approval at 07:00:00Z) is strictly after
	// every slot due then.
	if s := nextSlot(t, half, after("2026-03-08T07:00:00Z")); s.Key != "2026-03-09T02:00" {
		t.Fatalf("after an instant cursor = %+v", s)
	}
}

// Fall back: 01:30 happens twice on 2026-11-01 in New York; the slot is due
// once, at the first occurrence (EDT).
func TestTimetableRunsARepeatedTimeOnce(t *testing.T) {
	tt := timetable(t, "*/30 1 * * *", "America/New_York")
	var keys []string
	var dues []time.Time
	c := after("2026-11-01T04:00:00Z")
	for range 3 {
		s := nextSlot(t, tt, c)
		keys, dues = append(keys, s.Key), append(dues, s.Due)
		c = SlotCursor{At: s.Due, Key: s.Key}
	}
	if strings.Join(keys, ",") != "2026-11-01T01:00,2026-11-01T01:30,2026-11-02T01:00" {
		t.Fatalf("keys = %v", keys)
	}
	if !dues[0].Equal(utc("2026-11-01T05:00:00Z")) || !dues[1].Equal(utc("2026-11-01T05:30:00Z")) || !dues[2].Equal(utc("2026-11-02T06:00:00Z")) {
		t.Fatalf("dues = %v", dues)
	}
	window := tt.Due(after("2026-11-01T00:00:00Z"), utc("2026-11-01T23:00:00Z"), 10)
	if window.Total != 2 || len(window.Latest) != 2 {
		t.Fatalf("the repeated hour ran twice: %+v", window)
	}
}

// Day of month and day of week follow cron: when both are restricted a day
// matching either runs, but when either field starts with * a day must
// match both. N/S steps from N to the field's end.
func TestTimetableCronFields(t *testing.T) {
	tt := timetable(t, "0 9 1 * 1", "UTC")
	var keys []string
	c := after("2026-09-27T00:00:00Z")
	for range 4 {
		s := nextSlot(t, tt, c)
		keys = append(keys, s.Key)
		c = SlotCursor{At: s.Due, Key: s.Key}
	}
	// Monday 28 September, Thursday 1 October, Monday 5 and 12 October.
	if strings.Join(keys, ",") != "2026-09-28T09:00,2026-10-01T09:00,2026-10-05T09:00,2026-10-12T09:00" {
		t.Fatalf("keys = %v", keys)
	}
	// Days 1, 11, 21 and 31 that are Mondays: the first is 21 December.
	if s := nextSlot(t, timetable(t, "0 9 */10 * 1", "UTC"), after("2026-10-01T10:00:00Z")); s.Key != "2026-12-21T09:00" {
		t.Fatalf("starred day of month with a weekday = %+v", s)
	}
	steps := timetable(t, "5/20 3 * * *", "UTC")
	keys = nil
	c = after("2026-10-05T00:00:00Z")
	for range 3 {
		s := nextSlot(t, steps, c)
		keys = append(keys, s.Time)
		c = SlotCursor{At: s.Due, Key: s.Key}
	}
	if strings.Join(keys, ",") != "03:05,03:25,03:45" {
		t.Fatalf("stepped minutes = %v", keys)
	}
	if _, ok := timetable(t, "0 0 31 2 *", "UTC").Next(after("2026-01-01T00:00:00Z")); ok {
		t.Fatal("31 February has a slot")
	}
	if s := nextSlot(t, timetable(t, "0 0 29 2 *", "UTC"), after("2026-03-01T00:00:00Z")); s.Key != "2028-02-29T00:00" {
		t.Fatalf("29 February = %+v", s)
	}
}

// Due lists the slots after a cursor up to a time: the first, the latest
// few (oldest first) and how many there are; a cursor older than the scan
// bound is reported as truncated.
func TestTimetableDueWindow(t *testing.T) {
	tt := timetable(t, "0 7 * * *", "UTC")
	w := tt.Due(after("2026-10-01T12:00:00Z"), utc("2026-10-05T12:00:00Z"), 2)
	if w.Total != 4 || w.First == nil || w.First.Key != "2026-10-02T07:00" || len(w.Latest) != 2 || w.Latest[0].Key != "2026-10-04T07:00" ||
		w.Latest[1].Key != "2026-10-05T07:00" || w.Truncated {
		t.Fatalf("window = %+v", w)
	}
	if w := tt.Due(SlotCursor{At: utc("2026-10-05T07:00:00Z"), Key: "2026-10-05T07:00"}, utc("2026-10-05T12:00:00Z"), 3); w.Total != 0 || w.First != nil || len(w.Latest) != 0 {
		t.Fatalf("nothing due after the latest slot: %+v", w)
	}
	w = tt.Due(after("2023-01-01T00:00:00Z"), utc("2026-10-05T12:00:00Z"), 1)
	if !w.Truncated || len(w.Latest) != 1 || w.Latest[0].Key != "2026-10-05T07:00" || w.Total < 400 {
		t.Fatalf("long downtime window = %+v", w)
	}
}

// A schedule's inputs: literals as reviewed, $schedule.date and
// $schedule.time from the slot's nominal local values.
func TestWorkflowScheduleInputsComeFromTheSlot(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	spec, ok := v.Schedule()
	if !ok || spec.CatchUp != "each_missed" || spec.MaxRuns != 3 || spec.Timetable.Timezone != "America/New_York" {
		t.Fatalf("schedule = %+v, %v", spec, ok)
	}
	slot := nextSlot(t, spec.Timetable, after("2026-10-05T00:00:00Z"))
	inputs, err := v.ScheduleInputs(slot)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(inputs)
	if string(encoded) != `{"day":"2026-10-05"}` {
		t.Fatalf("inputs = %s", encoded)
	}
	accepted, err := v.AcceptInputs(inputs, nil)
	if err != nil || v.RunKey(accepted) != v.RunKey(map[string]any{"day": "2026-10-05"}) {
		t.Fatalf("run key from $schedule = %v, %v", accepted, err)
	}
	manual := mustValidate(t, filepath.Join("testdata", "split-bill"), testEnvironment())
	if _, ok := manual.Schedule(); ok {
		t.Fatal("a manual workflow has a schedule")
	}
}
