package advice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

func f64(v float64) *float64 { return &v }

func meas(t time.Time, ph, tds, turb, temp *float64) fish.Measurement {
	m := fish.Measurement{CreatedAt: t, Status: "success"}
	m.PH, m.TDS, m.Turbidity, m.Temperature = ph, tds, turb, temp
	return m
}

func staggeredHistory(now time.Time, hours int, step time.Duration, at func(elapsed time.Duration) (ph, tds, turb, temp *float64)) []fish.Measurement {
	var out []fish.Measurement
	for elapsed := time.Duration(hours) * time.Hour; elapsed >= 0; elapsed -= step {
		ph, tds, turb, temp := at(elapsed)
		t := now.Add(-elapsed)
		if ph != nil {
			out = append(out, meas(t, ph, nil, nil, nil))
		}
		if tds != nil {
			out = append(out, meas(t.Add(2*time.Second), nil, tds, nil, nil))
		}
		if turb != nil {
			out = append(out, meas(t.Add(4*time.Second), nil, nil, turb, nil))
		}
		if temp != nil {
			out = append(out, meas(t.Add(6*time.Second), nil, nil, nil, temp))
		}
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func TestAnalyze_NoData(t *testing.T) {
	res := Analyze(nil, Profile{})
	if res.Ready || res.Reason != "no_data" {
		t.Fatalf("expected no_data, got ready=%v reason=%s", res.Ready, res.Reason)
	}
	if res.Profile.Species != "ca_chinh" || res.Profile.VolumeL != 9000 {
		t.Fatalf("default profile: species=%s volume=%v", res.Profile.Species, res.Profile.VolumeL)
	}
}

func TestAnalyze_NeedMoreSamples(t *testing.T) {
	now := time.Now()
	history := []fish.Measurement{
		meas(now, f64(6.4), nil, nil, nil),
		meas(now.Add(-10*time.Minute), nil, f64(400), nil, nil),
	}
	res := Analyze(history, Profile{Species: "general"})
	if res.Ready {
		t.Fatal("expected not ready")
	}
	if res.Reason != "need_more_samples" {
		t.Fatalf("reason=%s", res.Reason)
	}
}

func TestAnalyze_FallingPH_RisingTDS_WaterChangeNoBuffer(t *testing.T) {
	now := time.Now()
	history := staggeredHistory(now, 8, 30*time.Minute, func(elapsed time.Duration) (ph, tds, turb, temp *float64) {
		frac := elapsed.Hours() / 8
		// 8h ago: ph 7.1, tds 240, turb 12 → now: ph 6.35, tds 410, turb 32
		return f64(6.35 + 0.75*frac), f64(410 - 170*frac), f64(32 - 20*frac), f64(27.0)
	})
	res := Analyze(history, Profile{Species: "general", VolumeL: 80})
	if !res.Ready {
		t.Fatalf("expected ready, reason=%s n_ph=%d span=%.2f", res.Reason, res.Trend["ph"].N, res.Trend["ph"].SpanHours)
	}
	if res.Trend["ph"].Slope != "falling" {
		t.Fatalf("ph slope=%s delta6h=%v", res.Trend["ph"].Slope, res.Trend["ph"].Delta6h)
	}
	if res.Trend["tds"].Slope != "rising" {
		t.Fatalf("tds slope=%s", res.Trend["tds"].Slope)
	}
	if res.Overall == "ok" {
		t.Fatal("expected warning/danger")
	}

	hasChange, hasKH := false, false
	for _, a := range res.Actions {
		if a.Type == "water_change_percent" {
			hasChange = true
		}
		if a.Type == "add_kh_buffer" {
			hasKH = true
		}
	}
	if !hasChange {
		t.Fatal("expected water_change_percent")
	}
	if hasKH {
		t.Fatal("must not recommend KH buffer when TDS is high")
	}
}

func TestAnalyze_LowPH_LowTDS_AllowsBuffer(t *testing.T) {
	now := time.Now()
	history := staggeredHistory(now, 6, 30*time.Minute, func(elapsed time.Duration) (ph, tds, turb, temp *float64) {
		frac := elapsed.Hours() / 6
		return f64(6.4 + 0.2*frac), f64(90 + 5*frac), f64(8.0), f64(26.5)
	})
	res := Analyze(history, Profile{Species: "general", VolumeL: 60})
	if !res.Ready {
		t.Fatalf("not ready: %s", res.Reason)
	}
	hasKH := false
	for _, a := range res.Actions {
		if a.Type == "add_kh_buffer" {
			hasKH = true
		}
	}
	if !hasKH {
		t.Fatalf("expected add_kh_buffer, actions=%v", typesOf(res.Actions))
	}
}

func TestAnalyze_StableInRange_Nothing(t *testing.T) {
	now := time.Now()
	history := staggeredHistory(now, 6, 20*time.Minute, func(elapsed time.Duration) (ph, tds, turb, temp *float64) {
		return f64(7.1), f64(180), f64(8), f64(26.5)
	})
	res := Analyze(history, Profile{})
	if !res.Ready {
		t.Fatalf("not ready: %s", res.Reason)
	}
	if res.Overall != "ok" {
		t.Fatalf("overall=%s findings=%v", res.Overall, res.Findings)
	}
	if len(res.Actions) == 0 || res.Actions[0].Type != "nothing" {
		t.Fatalf("expected nothing, got %v", typesOf(res.Actions))
	}
}

func TestBuildSeries_CarryForward(t *testing.T) {
	now := time.Now()
	history := []fish.Measurement{
		meas(now, nil, f64(200), nil, nil), // newest: tds
		meas(now.Add(-2*time.Minute), f64(7.2), nil, nil, nil),
		meas(now.Add(-4*time.Minute), nil, nil, f64(10), nil),
	}
	series := BuildSeries(history)
	if len(series) != 3 {
		t.Fatalf("len=%d", len(series))
	}
	last := series[len(series)-1]
	if last.PH == nil || *last.PH != 7.2 || last.TDS == nil || *last.TDS != 200 || last.Turbidity == nil {
		t.Fatalf("carry-forward failed: %+v", last)
	}
}

func TestBuildSeries_ExpiresStaleCarryForward(t *testing.T) {
	now := time.Now()
	history := []fish.Measurement{
		meas(now, nil, f64(180), nil, nil),
		meas(now.Add(-3*time.Hour), nil, nil, nil, f64(28)),
	}
	series := BuildSeries(history)
	last := series[len(series)-1]
	if last.Temperature != nil {
		t.Fatalf("stale temperature should expire, got %v", *last.Temperature)
	}
	if last.TDS == nil || *last.TDS != 180 {
		t.Fatal("fresh TDS should remain")
	}
}

func TestAnalyze_GapDoesNotInventSixHourDelta(t *testing.T) {
	now := time.Now()
	history := []fish.Measurement{
		meas(now.Add(-18*time.Hour), nil, f64(1400), nil, nil),
		meas(now.Add(-20*time.Minute), nil, f64(158), nil, nil),
		meas(now.Add(-10*time.Minute), nil, f64(155), nil, nil),
		meas(now, nil, f64(160), nil, nil),
	}
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	res := Analyze(history, Profile{})
	if res.Trend["tds"].Delta6h != nil {
		t.Fatalf("expected nil delta_6h across gap, got %v", *res.Trend["tds"].Delta6h)
	}
}

func TestEnrich_RequiresOpenAI(t *testing.T) {
	res := Analyze(nil, Profile{})
	if err := Enrich(context.Background(), res, nil); !errors.Is(err, ErrLLMNotConfigured) {
		t.Fatalf("expected ErrLLMNotConfigured, got %v", err)
	}
}

func TestThresholds_CaChinh(t *testing.T) {
	th := ThresholdsFor("ca_chinh")
	if th.PHMin != 7.0 || th.PHMax != 8.5 || th.TDSMax != 800 || th.TempMin != 26 {
		t.Fatalf("unexpected eel thresholds: %+v", th)
	}
}

func typesOf(actions []Action) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = a.Type
	}
	return out
}

func TestAnalyzeWithKit_AmmoniaAndDO(t *testing.T) {
	now := time.Now()
	history := staggeredHistory(now, 6, time.Hour, func(elapsed time.Duration) (ph, tds, turb, temp *float64) {
		return f64(7.4), f64(400), f64(10), f64(28)
	})
	kits := []fish.KitReading{{
		DOMGL:      f64(3.5),
		TANMGL:     f64(2.0),
		MeasuredAt: now,
	}}
	res := AnalyzeWithKit(history, kits, Profile{Species: "ca_chinh", VolumeL: 9000}, nil)
	if res.Current["do"] == nil || *res.Current["do"] != 3.5 {
		t.Fatalf("do=%v", res.Current["do"])
	}
	if res.Current["tan"] == nil {
		t.Fatal("missing tan")
	}
	if res.Current["nh3_free"] == nil || *res.Current["nh3_free"] <= 0 {
		t.Fatalf("nh3_free=%v", res.Current["nh3_free"])
	}
	if res.Kit.Source != "kit" || res.Kit.Stale {
		t.Fatalf("kit snapshot=%+v", res.Kit)
	}
	types := typesOf(res.Actions)
	hasAeration, hasChange, hasFeed := false, false, false
	for _, ty := range types {
		if ty == "increase_aeration" {
			hasAeration = true
		}
		if ty == "water_change_percent" {
			hasChange = true
		}
		if ty == "reduce_feeding" {
			hasFeed = true
		}
	}
	if !hasAeration || !hasChange || !hasFeed {
		t.Fatalf("actions=%v", types)
	}
	if res.Overall != "danger" {
		t.Fatalf("overall=%s", res.Overall)
	}
}
