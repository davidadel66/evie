// Package usage provides content-free owner diagnostics. Measurements never
// become conversation evidence, billing authority, or runtime budget state.
package usage

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

var ErrRange = errors.New("usage range must be 1 to 90 calendar days in a valid timezone")
var ErrLimit = errors.New("usage observation limit exceeded; narrow the range")

const MaxObservations = 200000

type Period struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	Timezone string    `json:"timezone"`
	Start    time.Time `json:"-"`
	End      time.Time `json:"-"`
	location *time.Location
}

func ParsePeriod(from, to, zone string) (Period, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		return Period{}, ErrRange
	}
	firstDate, err := time.Parse(time.DateOnly, from)
	if err != nil {
		return Period{}, ErrRange
	}
	lastDate, err := time.Parse(time.DateOnly, to)
	if err != nil || !lastDate.After(firstDate) || lastDate.Sub(firstDate) > 90*24*time.Hour {
		return Period{}, ErrRange
	}
	start, err := time.ParseInLocation(time.DateOnly, from, loc)
	if err != nil || start.Format(time.DateOnly) != from {
		return Period{}, ErrRange
	}
	end, err := time.ParseInLocation(time.DateOnly, to, loc)
	if err != nil || end.Format(time.DateOnly) != to || !end.After(start) {
		return Period{}, ErrRange
	}
	return Period{From: from, To: to, Timezone: zone, Start: start, End: end, location: loc}, nil
}

// Counter uses a decimal JSON string so totals remain exact in the browser.
type Counter struct {
	Value    *int64 `json:"value,string"`
	Reported int    `json:"reported"`
}
type Metrics struct {
	Calls         int      `json:"calls"`
	MeasuredCalls int      `json:"measuredCalls"`
	Input         Counter  `json:"input"`
	Cached        Counter  `json:"cached"`
	Output        Counter  `json:"output"`
	Total         Counter  `json:"total"`
	Reasoning     Counter  `json:"reasoning"`
	CacheWrite    Counter  `json:"cacheWrite"`
	CacheInput    *int64   `json:"cacheInput,string"`
	CacheCalls    int      `json:"cacheCalls"`
	CachePercent  *float64 `json:"cachePercent"`
	Anomalies     int      `json:"anomalies"`
	cacheRead     int64
}
type Tokens struct {
	Input      *int64 `json:"input_tokens"`
	Cached     *int64 `json:"cached_input_tokens"`
	Output     *int64 `json:"output_tokens"`
	Total      *int64 `json:"total_tokens"`
	Reasoning  *int64 `json:"reasoning_output_tokens"`
	CacheWrite *int64 `json:"cache_write_input_tokens"`
}
type Observation struct {
	ID     string
	At     time.Time
	Model  string
	Tokens Tokens
}
type Day struct {
	Date    string  `json:"date"`
	Metrics Metrics `json:"metrics"`
}
type Model struct {
	Name    string  `json:"name"`
	Metrics Metrics `json:"metrics"`
}
type Summary struct {
	Metrics       Metrics    `json:"metrics"`
	Daily         []Day      `json:"daily"`
	Models        []Model    `json:"models"`
	FirstObserved *time.Time `json:"firstObserved"`
	LastObserved  *time.Time `json:"lastObserved"`
}
type Source struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Label       string     `json:"label"`
	State       string     `json:"state"`
	Coverage    string     `json:"coverage"`
	Message     string     `json:"message,omitempty"`
	CollectedAt *time.Time `json:"collectedAt"`
	Summary     *Summary   `json:"summary"`
	Account     *Account   `json:"account"`
}
type AccountDay struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens,string"`
}
type Window struct {
	Name        string    `json:"name"`
	UsedPercent float64   `json:"usedPercent"`
	Minutes     int       `json:"minutes"`
	ResetsAt    time.Time `json:"resetsAt"`
}
type Account struct {
	Plan           string       `json:"plan"`
	LifetimeTokens *int64       `json:"lifetimeTokens,string"`
	Daily          []AccountDay `json:"daily"`
	Windows        []Window     `json:"windows"`
}
type Report struct {
	Period  Period   `json:"period"`
	Sources []Source `json:"sources"`
}

func Summarize(p Period, observations []Observation) (Summary, error) {
	result := Summary{Daily: []Day{}, Models: []Model{}}
	if len(observations) > MaxObservations {
		return result, ErrLimit
	}
	if p.location == nil {
		return result, ErrRange
	}
	days := map[string]*Metrics{}
	models := map[string]*Metrics{}
	firstDate, _ := time.Parse(time.DateOnly, p.From)
	lastDate, _ := time.Parse(time.DateOnly, p.To)
	for date := firstDate; date.Before(lastDate); date = date.AddDate(0, 0, 1) {
		days[date.Format(time.DateOnly)] = &Metrics{}
	}
	for _, o := range observations {
		if o.At.Before(p.Start) || !o.At.Before(p.End) {
			continue
		}
		day := o.At.In(p.location).Format(time.DateOnly)
		name := o.Model
		if name == "" {
			name = "Unknown model"
		}
		if _, ok := models[name]; !ok {
			if len(models) >= 200 {
				return Summary{}, ErrLimit
			}
			models[name] = &Metrics{}
		}
		for _, m := range []*Metrics{&result.Metrics, days[day], models[name]} {
			if err := m.add(o.Tokens); err != nil {
				return Summary{}, err
			}
		}
		at := o.At
		if result.FirstObserved == nil || at.Before(*result.FirstObserved) {
			result.FirstObserved = &at
		}
		if result.LastObserved == nil || at.After(*result.LastObserved) {
			result.LastObserved = &at
		}
	}
	for date, m := range days {
		result.Daily = append(result.Daily, Day{date, *m})
	}
	for name, m := range models {
		result.Models = append(result.Models, Model{name, *m})
	}
	sort.Slice(result.Daily, func(i, j int) bool { return result.Daily[i].Date < result.Daily[j].Date })
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].Name < result.Models[j].Name })
	return result, nil
}
func (m *Metrics) add(t Tokens) error {
	m.Calls++
	measured := false
	for _, pair := range []struct {
		dst   *Counter
		value *int64
	}{{&m.Input, t.Input}, {&m.Cached, t.Cached}, {&m.Output, t.Output}, {&m.Total, t.Total}, {&m.Reasoning, t.Reasoning}, {&m.CacheWrite, t.CacheWrite}} {
		if pair.value == nil || *pair.value < 0 {
			continue
		}
		measured = true
		if err := addValue(&pair.dst.Value, *pair.value); err != nil {
			return err
		}
		pair.dst.Reported++
	}
	if measured {
		m.MeasuredCalls++
	}
	anomaly := false
	if t.Input != nil && *t.Input >= 0 && t.Cached != nil && *t.Cached >= 0 {
		if *t.Cached > *t.Input {
			anomaly = true
		} else {
			if err := addValue(&m.CacheInput, *t.Input); err != nil {
				return err
			}
			if m.cacheRead > math.MaxInt64-*t.Cached {
				return fmt.Errorf("usage counter overflow")
			}
			m.cacheRead += *t.Cached
			m.CacheCalls++
			if *m.CacheInput > 0 {
				percent := 100 * float64(m.cacheRead) / float64(*m.CacheInput)
				m.CachePercent = &percent
			}
		}
	}
	if t.Reasoning != nil && t.Output != nil && *t.Reasoning > *t.Output {
		anomaly = true
	}
	if t.CacheWrite != nil && t.Input != nil && *t.CacheWrite > *t.Input {
		anomaly = true
	}
	if t.Input != nil && t.Output != nil && t.Total != nil && *t.Input >= 0 && *t.Output >= 0 && (*t.Input > math.MaxInt64-*t.Output || *t.Total != *t.Input+*t.Output) {
		anomaly = true
	}
	if anomaly {
		m.Anomalies++
	}
	return nil
}
func addValue(target **int64, value int64) error {
	if *target == nil {
		n := int64(0)
		*target = &n
	}
	if **target > math.MaxInt64-value {
		return fmt.Errorf("usage counter overflow")
	}
	**target += value
	return nil
}
