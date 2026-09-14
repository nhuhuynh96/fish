package calibrate

import "math"

// CalcPH converts probe voltage to pH using linear calibration around neutral point.
func CalcPH(voltage, neutralV, slope float64) float64 {
	if slope == 0 {
		slope = 0.18
	}
	ph := 7.0 + ((neutralV - voltage) / slope)
	return clamp(ph, 0, 14)
}

// CalcTDS converts probe voltage to ppm using DFRobot-style polynomial with temp compensation.
func CalcTDS(voltage, tempC float64) float64 {
	compensationCoefficient := 1.0 + 0.02*(tempC-25.0)
	if compensationCoefficient < 0.1 {
		compensationCoefficient = 0.1
	}
	compensationVoltage := voltage / compensationCoefficient

	tdsValue := (133.42*compensationVoltage*compensationVoltage*compensationVoltage -
		255.86*compensationVoltage*compensationVoltage +
		857.39*compensationVoltage) * 0.5

	return clamp(tdsValue, 0, 1000)
}

// CalcTurbidity maps voltage to NTU using piecewise linear calibration.
func CalcTurbidity(voltage, vClear, vDirty, ntuMax float64) float64 {
	if vClear <= vDirty {
		vClear, vDirty = 2.15, 1.0
	}
	if ntuMax <= 0 {
		ntuMax = 1000
	}

	var ntu float64
	switch {
	case voltage >= vClear:
		ntu = 0
	case voltage <= vDirty:
		ntu = ntuMax
	default:
		ntu = (vClear - voltage) / (vClear - vDirty) * ntuMax
	}
	return clamp(ntu, 0, ntuMax)
}

func clamp(v, min, max float64) float64 {
	return math.Max(min, math.Min(max, v))
}
