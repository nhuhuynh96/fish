package advice

import (
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

func hasType(actions []Action, ty string) bool {
	for _, a := range actions {
		if a.Type == ty {
			return true
		}
	}
	return false
}

func lowPHHighTDS(now time.Time) []fish.Measurement {
	return staggeredHistory(now, 6, time.Hour, func(elapsed time.Duration) (ph, tds, turb, temp *float64) {
		h := elapsed.Hours()
		return f64(6.6 + h*0.02), f64(900 - h*10), nil, f64(28)
	})
}

func TestRAS_LowPHLowKH_AddsBicarbonateNotWaterChange(t *testing.T) {
	now := time.Now()
	kits := []fish.KitReading{{KHDKH: f64(3), MeasuredAt: now}}
	res := AnalyzeWithKit(lowPHHighTDS(now), kits, Profile{Species: "ca_chinh", VolumeL: 9000, SystemType: "ras"}, nil)

	if !hasType(res.Actions, "add_alkalinity") {
		t.Fatalf("thiếu add_alkalinity: %v", typesOf(res.Actions))
	}
	if hasType(res.Actions, "water_change_percent") {
		t.Fatalf("RAS không nên thay nước gấp khi chỉ pH/TDS lệch: %v", typesOf(res.Actions))
	}
	if !hasType(res.Actions, "increase_daily_exchange") {
		t.Fatalf("thiếu increase_daily_exchange: %v", typesOf(res.Actions))
	}
	for _, d := range res.DoNot {
		if d == "Không đổ baking soda / pH Up khi TDS đang cao." {
			t.Fatal("RAS không được cấm baking soda")
		}
	}
}

func TestRAS_LowPHEnoughKH_DegasCO2(t *testing.T) {
	now := time.Now()
	kits := []fish.KitReading{{KHDKH: f64(8), MeasuredAt: now}}
	res := AnalyzeWithKit(lowPHHighTDS(now), kits, Profile{SystemType: "ras"}, nil)
	if !hasType(res.Actions, "degas_co2") || hasType(res.Actions, "add_alkalinity") {
		t.Fatalf("muốn degas_co2, có %v", typesOf(res.Actions))
	}
}

func TestRAS_NitriteHigh_ChecksBiofilter(t *testing.T) {
	now := time.Now()
	history := staggeredHistory(now, 6, time.Hour, func(time.Duration) (ph, tds, turb, temp *float64) {
		return f64(7.6), f64(400), nil, f64(28)
	})
	kits := []fish.KitReading{
		{NO2MGL: f64(0.6), MeasuredAt: now},
		{DOMGL: f64(6.5), MeasuredAt: now.Add(-2 * time.Hour)},
	}
	res := AnalyzeWithKit(history, kits, Profile{SystemType: "ras"}, nil)
	if !hasType(res.Actions, "check_biofilter") || !hasType(res.Actions, "reduce_feeding") {
		t.Fatalf("thiếu check_biofilter / reduce_feeding: %v", typesOf(res.Actions))
	}
	if res.Current["do"] == nil || *res.Current["do"] != 6.5 {
		t.Fatalf("DO từ lần nhập trước phải còn, do=%v", res.Current["do"])
	}
}

func TestPond_LowPHHighTDS_StillChangesWater(t *testing.T) {
	now := time.Now()
	res := AnalyzeWithKit(lowPHHighTDS(now), nil, Profile{SystemType: "pond"}, nil)
	if !hasType(res.Actions, "water_change_percent") || hasType(res.Actions, "add_alkalinity") {
		t.Fatalf("ao thường giữ cách cũ: %v", typesOf(res.Actions))
	}
}
