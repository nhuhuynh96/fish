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
// Optional single-point scale: when refV > 0 and refPPM > 0, result is scaled so that
// voltage=refV maps to refPPM (after temperature compensation).
func CalcTDS(voltage, tempC, refV, refPPM, maxPPM float64) float64 {
	if maxPPM <= 0 {
		maxPPM = 2000
	}
	raw := tdsFromVoltage(voltage, tempC)
	if refV > 0 && refPPM > 0 {
		refRaw := tdsFromVoltage(refV, tempC)
		if refRaw > 1 {
			raw = raw * (refPPM / refRaw)
		}
	}
	return clamp(raw, 0, maxPPM)
}

func tdsFromVoltage(voltage, tempC float64) float64 {
	compensationCoefficient := 1.0 + 0.02*(tempC-25.0)
	if compensationCoefficient < 0.1 {
		compensationCoefficient = 0.1
	}
	compensationVoltage := voltage / compensationCoefficient

	return (133.42*compensationVoltage*compensationVoltage*compensationVoltage -
		255.86*compensationVoltage*compensationVoltage +
		857.39*compensationVoltage) * 0.5
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
