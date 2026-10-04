package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Workflow Schedules (Stage 14 sub-stage 11, 2026-10-04; ADR 0020 and
// 0023). A reviewed schedule names a five-field cron and an IANA timezone.
// Its slots are computed on that timezone's wall clock, never the host's:
//
//   - A slot is one nominal local date and time the cron matches (minute,
//     hour, day of month, month, day of week; when both day fields are
//     restricted, a day matching either runs, and a day field starting with
//     "*" restricts nothing beyond the other, as in cron). "N/S" steps from
//     N to the field's end. Its key is that nominal local time,
//     "2006-01-02T15:04", which with the timezone identifies it for good.
//   - Its due time is the instant that wall time happens in the timezone.
//     A wall time that does not exist (the hour skipped when daylight saving
//     starts) is due when the gap ends, keeping its nominal key and marked
//     Shifted, so a daily or weekly slot is never lost. A wall time that
//     happens twice (the hour repeated when daylight saving ends) is due
//     once, at its first occurrence.
//   - $schedule.date and $schedule.time are the slot's nominal local date
//     and time, so a run key that uses them is the same however the slot's
//     instant moved.
//
// Nothing here stores, starts or catches up runs: the runtime records each
// slot it decides and applies the definition's Catch-up Policy.

// Timetable is a validated cron in an IANA timezone.
type Timetable struct {
	Cron     string
	Timezone string
	cron     cronSpec
	loc      *time.Location
}

// Slot is one scheduled time.
type Slot struct {
	// Key is the nominal local date and time, "2006-01-02T15:04".
	Key string `json:"key"`
	// Date and Time are $schedule.date and $schedule.time.
	Date string `json:"date"`
	Time string `json:"time"`
	// Due is the instant the slot comes due.
	Due time.Time `json:"due"`
	// Shifted marks a nominal time that did not exist (a daylight-saving
	// gap): it is due when the gap ends.
	Shifted bool `json:"shifted,omitempty"`
}

// SlotCursor is a position on a timetable: the slots after it are those
// due later than At, or, when Key names the slot decided last, due at At
// with a later key (slots shifted out of a gap share one due time). An
// empty Key is a plain instant: only slots due strictly later follow it.
type SlotCursor struct {
	At  time.Time `json:"at"`
	Key string    `json:"key,omitempty"`
}

// Follows reports whether slot s comes after the cursor.
func (c SlotCursor) Follows(s Slot) bool {
	return s.Due.After(c.At) || c.Key != "" && s.Due.Equal(c.At) && s.Key > c.Key
}

// SlotWindow is the slots after a cursor up to a time: the first, the
// latest ones (oldest first) and how many there are. Truncated says the
// cursor lay further back than the scan bound, so First and Total cover
// only the bound.
type SlotWindow struct {
	First     *Slot  `json:"first,omitempty"`
	Latest    []Slot `json:"latest"`
	Total     int    `json:"total"`
	Truncated bool   `json:"truncated,omitempty"`
}

const (
	// maxScheduleScanDays bounds how far back Due looks for missed slots.
	maxScheduleScanDays = 400
	// maxScheduleHorizonDays bounds how far ahead Next looks (eight years
	// cover 29 February).
	maxScheduleHorizonDays = 8 * 366
	slotKeyLayout          = "2006-01-02T15:04"
)

// NewTimetable validates a cron and an IANA timezone.
func NewTimetable(cron, timezone string) (Timetable, error) {
	spec, err := parseCron(cron)
	if err != nil {
		return Timetable{}, err
	}
	if timezone == "" || timezone == "Local" {
		return Timetable{}, fmt.Errorf("name an IANA timezone such as America/New_York, not %q", timezone)
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Timetable{}, fmt.Errorf("unknown timezone %s", timezone)
	}
	return Timetable{Cron: strings.Join(strings.Fields(cron), " "), Timezone: timezone, cron: spec, loc: loc}, nil
}

// Next is the first slot after the cursor, or false when none comes within
// eight years (a date such as 31 February never comes).
func (t Timetable) Next(c SlotCursor) (Slot, bool) {
	day := civilDate(c.At.In(t.loc)).AddDate(0, 0, -1)
	for range maxScheduleHorizonDays {
		for _, s := range t.slotsOn(day) {
			if c.Follows(s) {
				return s, true
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return Slot{}, false
}

// Due is every slot after the cursor that is due by until: the first, the
// latest keep (oldest first) and the total. It scans at most
// maxScheduleScanDays back from until.
func (t Timetable) Due(c SlotCursor, until time.Time, keep int) SlotWindow {
	w := SlotWindow{Latest: []Slot{}}
	start := civilDate(c.At.In(t.loc)).AddDate(0, 0, -1)
	end := civilDate(until.In(t.loc)).AddDate(0, 0, 1)
	if bound := end.AddDate(0, 0, -maxScheduleScanDays); start.Before(bound) {
		start, w.Truncated = bound, true
	}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		for _, s := range t.slotsOn(day) {
			if !c.Follows(s) || s.Due.After(until) {
				continue
			}
			if w.First == nil {
				first := s
				w.First = &first
			}
			w.Total++
			if keep > 0 {
				if len(w.Latest) == keep {
					w.Latest = append(w.Latest[:0:0], w.Latest[1:]...)
				}
				w.Latest = append(w.Latest, s)
			}
		}
	}
	return w
}

// civilDate is midnight UTC of t's local calendar date, used only as a
// calendar counter.
func civilDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// slotsOn lists the slots of one local calendar date in nominal order,
// which is also due order.
func (t Timetable) slotsOn(day time.Time) []Slot {
	if !t.cron.dayMatches(day) {
		return nil
	}
	var out []Slot
	for h := range 24 {
		if !t.cron.hours[h] {
			continue
		}
		for m := range 60 {
			if !t.cron.minutes[m] {
				continue
			}
			nominal := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.UTC)
			due, shifted := t.instant(nominal)
			out = append(out, Slot{Key: nominal.Format(slotKeyLayout), Date: nominal.Format(time.DateOnly), Time: nominal.Format("15:04"), Due: due, Shifted: shifted})
		}
	}
	return out
}

// instant is when the nominal wall time (given as UTC fields) happens in
// the timezone: its first occurrence, or the end of the gap it falls in.
func (t Timetable) instant(nominal time.Time) (time.Time, bool) {
	var best, latest time.Time
	found := false
	for _, probe := range []time.Duration{-36 * time.Hour, -12 * time.Hour, 0, 12 * time.Hour, 36 * time.Hour} {
		_, offset := nominal.Add(probe).In(t.loc).Zone()
		c := nominal.Add(-time.Duration(offset) * time.Second)
		if latest.IsZero() || c.After(latest) {
			latest = c
		}
		if sameWall(c.In(t.loc), nominal) && (!found || c.Before(best)) {
			best, found = c, true
		}
	}
	if found {
		return best.UTC(), false
	}
	// A gap: the candidate under the offset before it lies after the
	// transition, which started the zone it falls in.
	start, _ := latest.In(t.loc).ZoneBounds()
	if start.IsZero() || start.After(latest) {
		return latest.UTC(), true
	}
	return start.UTC(), true
}

func sameWall(local, nominal time.Time) bool {
	y, mo, d := local.Date()
	return y == nominal.Year() && mo == nominal.Month() && d == nominal.Day() && local.Hour() == nominal.Hour() && local.Minute() == nominal.Minute()
}

// cronSpec is a parsed five-field cron.
type cronSpec struct {
	minutes  [60]bool
	hours    [24]bool
	days     [32]bool
	months   [13]bool
	weekdays [7]bool
	// dayStar and weekdayStar: the day-of-month or day-of-week field starts
	// with "*", so a day must match both fields rather than either.
	dayStar, weekdayStar bool
}

func parseCron(expr string) (cronSpec, error) {
	if err := validateCron(expr); err != nil {
		return cronSpec{}, err
	}
	fields := strings.Fields(expr)
	var c cronSpec
	sets := [][]bool{c.minutes[:], c.hours[:], c.days[:], c.months[:], c.weekdays[:]}
	for i, field := range fields {
		spec := cronFields[i]
		for _, part := range strings.Split(field, ",") {
			base, stepText, stepped := strings.Cut(part, "/")
			step := 1
			if stepped {
				step, _ = strconv.Atoi(stepText)
			}
			lo, hi := spec.min, spec.max
			if base != "*" {
				a, b, ranged := strings.Cut(base, "-")
				lo, _ = strconv.Atoi(a)
				hi = lo
				switch {
				case ranged:
					hi, _ = strconv.Atoi(b)
				case stepped:
					hi = spec.max
				}
			}
			for v := lo; v <= hi; v += step {
				sets[i][v] = true
			}
		}
	}
	c.dayStar, c.weekdayStar = strings.HasPrefix(fields[2], "*"), strings.HasPrefix(fields[4], "*")
	return c, nil
}

func (c cronSpec) dayMatches(day time.Time) bool {
	if !c.months[day.Month()] {
		return false
	}
	dom, dow := c.days[day.Day()], c.weekdays[day.Weekday()]
	if c.dayStar || c.weekdayStar {
		return dom && dow
	}
	return dom || dow
}

// ScheduleSpec is a definition's reviewed schedule: its timetable and its
// Catch-up Policy (skip, or each_missed with at most MaxRuns recovery
// runs).
type ScheduleSpec struct {
	Timetable Timetable
	CatchUp   string
	MaxRuns   int
}

// Catch-up Policies (ADR 0023).
const (
	CatchUpSkip       = "skip"
	CatchUpEachMissed = "each_missed"
)

// Schedule is the definition's reviewed schedule, if it declares one.
func (v *Validated) Schedule() (ScheduleSpec, bool) {
	s := v.definition.Schedule
	if s == nil || s.CatchUp == nil {
		return ScheduleSpec{}, false
	}
	tt, err := NewTimetable(s.Cron, s.Timezone)
	if err != nil {
		return ScheduleSpec{}, false
	}
	return ScheduleSpec{Timetable: tt, CatchUp: s.CatchUp.Policy, MaxRuns: s.CatchUp.MaxRuns}, true
}

// ScheduleInputs are a scheduled run's inputs for a slot: the schedule's
// literals as reviewed, and $schedule.date and $schedule.time from the
// slot. Inputs it leaves out take their defaults when accepted.
func (v *Validated) ScheduleInputs(slot Slot) (map[string]json.RawMessage, error) {
	s := v.definition.Schedule
	if s == nil {
		return nil, fmt.Errorf("workflow %s has no schedule", v.Name())
	}
	out := map[string]json.RawMessage{}
	names := make([]string, 0, len(s.Inputs))
	for name := range s.Inputs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		node := s.Inputs[name]
		value, err := nodeValue(&node)
		if err != nil {
			return nil, fmt.Errorf("schedule.inputs.%s: %v", name, err)
		}
		if text, ok := value.(string); ok && isRefText(text) {
			r, err := parseRef(text)
			if err != nil || r.Root != refSchedule {
				return nil, fmt.Errorf("schedule.inputs.%s: %s is not a schedule value", name, text)
			}
			value = slot.Date
			if r.Name == "time" {
				value = slot.Time
			}
		}
		encoded, err := EncodeValue(value)
		if err != nil {
			return nil, fmt.Errorf("schedule.inputs.%s: %v", name, err)
		}
		out[name] = encoded
	}
	return out, nil
}
