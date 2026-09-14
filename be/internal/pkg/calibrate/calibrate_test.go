package calibrate

import "testing"

func TestCalcPH_Neutral(t *testing.T) {
	ph := CalcPH(2.50, 2.50, 0.18)
	if ph < 6.99 || ph > 7.01 {
		t.Fatalf("expected pH ~7.0 at neutral, got %.2f", ph)
	}
}

func TestCalcPH_Alkaline(t *testing.T) {
	ph := CalcPH(2.334, 2.50, 0.18)
	if ph < 7.8 || ph > 8.0 {
		t.Fatalf("expected pH ~7.92, got %.2f", ph)
	}
}

func TestCalcPH_Clamp(t *testing.T) {
	if CalcPH(5.0, 2.5, 0.18) != 0 {
		t.Fatal("expected clamp to 0")
	}
	if CalcPH(-1.0, 2.5, 0.18) != 14 {
		t.Fatal("expected clamp to 14")
	}
}

func TestCalcTDS_LowVoltage(t *testing.T) {
	tds := CalcTDS(0.144, 25.0)
	if tds < 50 || tds > 70 {
		t.Fatalf("expected TDS ~59 ppm, got %.0f", tds)
	}
}

func TestCalcTurbidity_ClearWater(t *testing.T) {
	ntu := CalcTurbidity(2.164, 2.15, 1.0, 1000)
	if ntu > 5 {
		t.Fatalf("expected clear water NTU near 0, got %.1f", ntu)
	}
}

func TestCalcTurbidity_DirtyWater(t *testing.T) {
	ntu := CalcTurbidity(0.5, 2.15, 1.0, 1000)
	if ntu != 1000 {
		t.Fatalf("expected max NTU at dirty voltage, got %.1f", ntu)
	}
}
