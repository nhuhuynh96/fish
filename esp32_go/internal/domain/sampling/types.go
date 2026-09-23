package sampling

import "time"

type SensorType int

const (
	SensorTemp SensorType = iota
	SensorPH
	SensorTurbidity
	SensorTDS
)

func (s SensorType) Name() string {
	switch s {
	case SensorTemp:
		return "temperature"
	case SensorPH:
		return "ph"
	case SensorTurbidity:
		return "turbidity"
	case SensorTDS:
		return "tds"
	default:
		return "unknown"
	}
}

type ActionType string

const (
	ActionInlet      ActionType = "inlet"
	ActionDrain      ActionType = "drain"
	ActionPH         ActionType = "ph"
	ActionTurbidity  ActionType = "turbidity"
	ActionTDS        ActionType = "tds"
	ActionStatus     ActionType = "status"
	ActionStop       ActionType = "stop"
	ActionClearQueue ActionType = "clear_queue"
)

type FillLevel int

const (
	FillLevelLow  FillLevel = iota // 0 — phao 1 (pH / turb)
	FillLevelHigh                  // 1 — phao 2 (TDS / bơm thủ công)
)

func NormalizeFillLevel(v FillLevel) FillLevel {
	if v >= FillLevelHigh {
		return FillLevelHigh
	}
	return FillLevelLow
}

func (l FillLevel) Name() string {
	if l == FillLevelHigh {
		return "mức 2/TDS"
	}
	return "mức 1/pH+turb"
}

// Command is the only MQTT command shape: {"action":"...","fillLevel":0|1}
type Command struct {
	Action    ActionType `json:"action"`
	FillLevel FillLevel  `json:"fillLevel"`
}

type RawReading struct {
	ADC         int     `json:"adc"`
	Voltage     float64 `json:"voltage"`
	SampleCount int     `json:"sample_count"`
}

const (
	FloatDebounce     = 800 * time.Millisecond
	PumpFloatGrace    = 1 * time.Second
	ManualPumpTime    = 30 * time.Second
	DrainFixedTime    = 30 * time.Second
	MaxManualAttempts = 3
)
