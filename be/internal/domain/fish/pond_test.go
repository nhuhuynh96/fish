package fish

import "testing"

func TestDefaultPondConfig_Eel(t *testing.T) {
	c := DefaultPondConfig()
	if c.Species != "ca_chinh" || c.VolumeL != 9000 {
		t.Fatalf("species/volume: %+v", c)
	}
	if c.Thresholds.PHMin != 7.0 || c.Thresholds.PHMax != 8.5 {
		t.Fatalf("pH: %+v", c.Thresholds)
	}
	if c.Badges["ph"].OK == "" || c.Badges["tds"].OK == "" {
		t.Fatalf("badges: %+v", c.Badges)
	}
}
