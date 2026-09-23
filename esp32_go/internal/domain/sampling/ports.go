package sampling

// Hardware is the outbound port for pumps, floats, and ADC (driven adapters).
type Hardware interface {
	SetInlet(on bool)
	SetDrain(on bool)
	InletOn() bool
	DrainOn() bool
	FloatLow() bool  // phao mức 1 (GPIO17)
	FloatHigh() bool // phao mức 2 (GPIO4)
	SetSensorPower(sensor SensorType, on bool)
	ReadADC(sensor SensorType) RawReading
}

// Bus is the outbound port for MQTT telemetry (driven adapter).
type Bus interface {
	Publish(subTopic string, payload []byte, retained bool) error
	Connected() bool
}
