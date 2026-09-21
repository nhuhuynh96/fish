package advice

import (
	"context"
	"time"
)

type Profile struct {
	VolumeL   float64 `json:"volume_l"`
	Species   string  `json:"species"`
	HasFilter *bool   `json:"has_filter,omitempty"`
}

type Thresholds struct {
	TempMin       float64 `json:"temp_min"`
	TempMax       float64 `json:"temp_max"`
	PHMin         float64 `json:"ph_min"`
	PHMax         float64 `json:"ph_max"`
	TurbidityWarn float64 `json:"turbidity_warn"`
	TurbidityMax  float64 `json:"turbidity_max"`
	TDSMin        float64 `json:"tds_min"`
	TDSMax        float64 `json:"tds_max"`
}

type Snapshot struct {
	Time        time.Time
	Temperature *float64
	PH          *float64
	Turbidity   *float64
	TDS         *float64
}

type MetricTrend struct {
	Current   *float64 `json:"current,omitempty"`
	Delta1h   *float64 `json:"delta_1h,omitempty"`
	Delta6h   *float64 `json:"delta_6h,omitempty"`
	Delta24h  *float64 `json:"delta_24h,omitempty"`
	Slope     string   `json:"slope"`
	N         int      `json:"n"`
	SpanHours float64  `json:"span_hours"`
}

type Finding struct {
	Metric string     `json:"metric"`
	Value  *float64   `json:"value,omitempty"`
	Target [2]float64 `json:"target"`
	Status string     `json:"status"`
	Slope  string     `json:"slope"`
	Label  string     `json:"label"`
}

type Action struct {
	Priority  int            `json:"priority"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Detail    string         `json:"detail"`
	AmountPct *int           `json:"amount_pct,omitempty"`
	Hardware  map[string]any `json:"hardware,omitempty"`
}

type SeriesPoint struct {
	Time        string   `json:"t"`
	Temperature *float64 `json:"temperature,omitempty"`
	PH          *float64 `json:"ph,omitempty"`
	Turbidity   *float64 `json:"turbidity,omitempty"`
	TDS         *float64 `json:"tds,omitempty"`
}

type Result struct {
	Overall      string                 `json:"overall"`
	Ready        bool                   `json:"ready"`
	Reason       string                 `json:"reason,omitempty"`
	Summary      string                 `json:"summary"`
	UsedLLM      bool                   `json:"used_llm"`
	Profile      Profile                `json:"profile"`
	Thresholds   Thresholds             `json:"thresholds"`
	Current      map[string]*float64    `json:"current"`
	Trend        map[string]MetricTrend `json:"trend"`
	Series6h     []SeriesPoint          `json:"series_6h,omitempty"`
	Findings     []Finding              `json:"findings"`
	Actions      []Action               `json:"actions"`
	DoNot        []string               `json:"do_not"`
	Limitations  string                 `json:"limitations"`
	SampleWindow string                 `json:"sample_window"`
	LatestAt     *time.Time             `json:"latest_at,omitempty"`
	Stale        bool                   `json:"stale"`
}

type Narration struct {
	Summary string            `json:"summary"`
	Details map[string]string `json:"action_details"`
	DoNot   []string          `json:"do_not"`
}

type Narrator interface {
	Narrate(ctx context.Context, result *Result) (*Narration, error)
}
