package advice

import (
	"math"
	"sort"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

const carryMaxAge = 2 * time.Hour

type hold struct {
	v  *float64
	at time.Time
}

func (h hold) valueAt(t time.Time) *float64 {
	if h.v == nil || t.Sub(h.at) > carryMaxAge {
		return nil
	}
	return cloneF(h.v)
}

type sample struct {
	t time.Time
	v float64
}

func cloneF(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}

func roundPtr(p *float64, digits int) *float64 {
	if p == nil {
		return nil
	}
	v := round(*p, digits)
	return &v
}

// BuildSeries carry-forwards staggered sensor rows (newest-first input).
// A metric expires if it was not refreshed within carryMaxAge.
func BuildSeries(history []fish.Measurement) []Snapshot {
	if len(history) == 0 {
		return nil
	}
	chrono := make([]fish.Measurement, len(history))
	for i := range history {
		chrono[len(history)-1-i] = history[i]
	}

	var temp, ph, turb, tds hold
	out := make([]Snapshot, 0, len(chrono))
	for _, m := range chrono {
		t := m.CreatedAt
		if m.Temperature != nil {
			temp = hold{v: cloneF(m.Temperature), at: t}
		}
		if m.PH != nil {
			ph = hold{v: cloneF(m.PH), at: t}
		}
		if m.Turbidity != nil {
			turb = hold{v: cloneF(m.Turbidity), at: t}
		}
		if m.TDS != nil {
			tds = hold{v: cloneF(m.TDS), at: t}
		}
		if m.Temperature == nil && m.PH == nil && m.Turbidity == nil && m.TDS == nil {
			continue
		}
		out = append(out, Snapshot{
			Time:        t,
			Temperature: temp.valueAt(t),
			PH:          ph.valueAt(t),
			Turbidity:   turb.valueAt(t),
			TDS:         tds.valueAt(t),
		})
	}
	return out
}

func metricValue(s Snapshot, metric string) *float64 {
	switch metric {
	case "temperature":
		return s.Temperature
	case "ph":
		return s.PH
	case "turbidity":
		return s.Turbidity
	case "tds":
		return s.TDS
	default:
		return nil
	}
}

func rawValue(m fish.Measurement, metric string) *float64 {
	switch metric {
	case "temperature":
		return m.Temperature
	case "ph":
		return m.PH
	case "turbidity":
		return m.Turbidity
	case "tds":
		return m.TDS
	default:
		return nil
	}
}

func countRaw(history []fish.Measurement, metric string, since time.Time) (n int, spanHours float64) {
	var first, last time.Time
	for _, m := range history {
		if rawValue(m, metric) == nil {
			continue
		}
		if m.CreatedAt.Before(since) {
			continue
		}
		n++
		if first.IsZero() || m.CreatedAt.Before(first) {
			first = m.CreatedAt
		}
		if last.IsZero() || m.CreatedAt.After(last) {
			last = m.CreatedAt
		}
	}
	if n >= 2 {
		spanHours = last.Sub(first).Hours()
	}
	return n, spanHours
}

func collectSamples(history []fish.Measurement, metric string) []sample {
	tmp := make([]sample, 0, len(history))
	for _, m := range history {
		v := rawValue(m, metric)
		if v == nil {
			continue
		}
		tmp = append(tmp, sample{t: m.CreatedAt, v: *v})
	}
	sort.Slice(tmp, func(i, j int) bool { return tmp[i].t.Before(tmp[j].t) })
	return tmp
}

func median(vals []float64) *float64 {
	if len(vals) == 0 {
		return nil
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	var v float64
	if len(cp)%2 == 0 {
		v = (cp[mid-1] + cp[mid]) / 2
	} else {
		v = cp[mid]
	}
	return &v
}

func medianAround(samples []sample, center time.Time, radius time.Duration) *float64 {
	vals := make([]float64, 0, 8)
	for _, s := range samples {
		d := s.t.Sub(center)
		if d < 0 {
			d = -d
		}
		if d <= radius {
			vals = append(vals, s.v)
		}
	}
	return median(vals)
}

func deltaMedian(samples []sample, now time.Time, window time.Duration) *float64 {
	if len(samples) == 0 {
		return nil
	}
	current := medianAround(samples, now, 25*time.Minute)
	if current == nil {
		last := samples[len(samples)-1]
		if now.Sub(last.t) <= 25*time.Minute {
			current = &last.v
		}
	}
	if current == nil {
		return nil
	}
	radius := 45 * time.Minute
	if window <= time.Hour {
		radius = 20 * time.Minute
	}
	past := medianAround(samples, now.Add(-window), radius)
	if past == nil {
		return nil
	}
	d := *current - *past
	return &d
}

func slopeOf(delta *float64, noise float64) string {
	if delta == nil {
		return "unknown"
	}
	if math.Abs(*delta) <= noise {
		return "stable"
	}
	if *delta > 0 {
		return "rising"
	}
	return "falling"
}

func downsample(series []Snapshot, since time.Time, bucket time.Duration, maxPoints int) []SeriesPoint {
	if len(series) == 0 {
		return nil
	}
	var buckets []Snapshot
	var cur *Snapshot
	var bucketStart time.Time
	for _, s := range series {
		if s.Time.Before(since) {
			continue
		}
		if cur == nil {
			cp := s
			cur = &cp
			bucketStart = s.Time.Truncate(bucket)
			continue
		}
		if s.Time.Truncate(bucket).Equal(bucketStart) {
			cp := s
			cur = &cp
			continue
		}
		buckets = append(buckets, *cur)
		cp := s
		cur = &cp
		bucketStart = s.Time.Truncate(bucket)
	}
	if cur != nil {
		buckets = append(buckets, *cur)
	}
	if maxPoints > 0 && len(buckets) > maxPoints {
		step := float64(len(buckets)) / float64(maxPoints)
		trimmed := make([]Snapshot, 0, maxPoints)
		for i := 0; i < maxPoints; i++ {
			idx := int(math.Round(float64(i) * step))
			if idx >= len(buckets) {
				idx = len(buckets) - 1
			}
			if len(trimmed) > 0 && trimmed[len(trimmed)-1].Time.Equal(buckets[idx].Time) {
				continue
			}
			trimmed = append(trimmed, buckets[idx])
		}
		if trimmed[len(trimmed)-1].Time != buckets[len(buckets)-1].Time {
			trimmed[len(trimmed)-1] = buckets[len(buckets)-1]
		}
		buckets = trimmed
	}
	out := make([]SeriesPoint, 0, len(buckets))
	for _, s := range buckets {
		out = append(out, SeriesPoint{
			Time:        s.Time.Format("15:04"),
			Temperature: roundPtr(s.Temperature, 1),
			PH:          roundPtr(s.PH, 2),
			Turbidity:   roundPtr(s.Turbidity, 1),
			TDS:         roundPtr(s.TDS, 0),
		})
	}
	return out
}

func buildTrend(history []fish.Measurement, series []Snapshot, metric string, noise float64, digits int) MetricTrend {
	since24 := time.Now().Add(-24 * time.Hour)
	if len(series) > 0 {
		since24 = series[len(series)-1].Time.Add(-24 * time.Hour)
	}
	n, span := countRaw(history, metric, since24)
	tr := MetricTrend{N: n, SpanHours: round(span, 2), Slope: "unknown"}
	if len(series) == 0 {
		return tr
	}
	now := series[len(series)-1].Time
	samples := collectSamples(history, metric)
	tr.Current = roundPtr(metricValue(series[len(series)-1], metric), digits)
	tr.Delta1h = roundPtr(deltaMedian(samples, now, time.Hour), digits)
	tr.Delta6h = roundPtr(deltaMedian(samples, now, 6*time.Hour), digits)
	tr.Delta24h = roundPtr(deltaMedian(samples, now, 24*time.Hour), digits)
	primary := tr.Delta6h
	if primary == nil {
		primary = tr.Delta1h
	}
	tr.Slope = slopeOf(primary, noise)
	return tr
}
