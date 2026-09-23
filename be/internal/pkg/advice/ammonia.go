package advice

import "math"

// UnionizedAmmoniaFraction is the Emerson (1975) fraction of TAN that is free NH3.
func UnionizedAmmoniaFraction(ph, tempC float64) float64 {
	if ph <= 0 {
		return 0
	}
	if tempC < 0 || tempC > 45 {
		tempC = 25
	}
	tk := 273.15 + tempC
	pKa := 0.09018 + 2729.92/tk
	return 1.0 / (1.0 + math.Pow(10, pKa-ph))
}

func FreeAmmonia(tanMGL, ph, tempC float64) float64 {
	if tanMGL < 0 {
		return 0
	}
	return tanMGL * UnionizedAmmoniaFraction(ph, tempC)
}
