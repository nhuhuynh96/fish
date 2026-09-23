package advice

import "testing"

func TestUnionizedAmmoniaFraction(t *testing.T) {
	// ~25°C pH 7.0 → very little free NH3 (~0.4%)
	f70 := UnionizedAmmoniaFraction(7.0, 25)
	if f70 < 0.003 || f70 > 0.008 {
		t.Fatalf("pH7 25C fraction=%v", f70)
	}
	// pH 8.0 25°C ~4%
	f80 := UnionizedAmmoniaFraction(8.0, 25)
	if f80 < 0.03 || f80 > 0.06 {
		t.Fatalf("pH8 25C fraction=%v", f80)
	}
	if FreeAmmonia(1.0, 8.0, 25) < 0.03 {
		t.Fatal("expected ~0.04 mg/L free NH3 from 1 mg/L TAN")
	}
}
