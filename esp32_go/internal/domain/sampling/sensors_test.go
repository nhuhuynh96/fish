package sampling

import "testing"

func TestExpandSensorTokens_PHTurbTDS(t *testing.T) {
	got := ExpandSensorTokens([]string{"ph", "turbidity", "tds"})
	want := []SensorType{SensorPH, SensorTurbidity, SensorTDS}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %s want %s", i, got[i].Name(), want[i].Name())
		}
	}
}

func TestExpandSensorTokens_All(t *testing.T) {
	got := ExpandSensorTokens([]string{"all"})
	if len(got) != 3 || got[2] != SensorTDS {
		t.Fatalf("all should expand to ph,turb,tds got %v", got)
	}
}
