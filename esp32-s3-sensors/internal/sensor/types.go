package sensor

// SensorType định danh các loại cảm biến
type SensorType string

const (
	SensorTemp SensorType = "temperature"
	SensorPH   SensorType = "ph"
	SensorTDS  SensorType = "tds"
)

// RawReading chứa giá trị ADC thô, điện áp quy đổi và số mẫu đọc
type RawReading struct {
	ADC         int     `json:"adc"`
	Voltage     float32 `json:"voltage"`
	SampleCount int     `json:"sample_count"`
}

// SensorPayload là cấu trúc JSON bắn lên MQTT topic fish/<device_id>/sensor_data
type SensorPayload struct {
	DeviceID        string                 `json:"device_id"`
	Timestamp       int64                  `json:"timestamp"`
	Status          string                 `json:"status"`
	DurationMs      uint32                 `json:"duration_ms"`
	SensorsMeasured []string               `json:"sensors_measured"`
	Raw             map[string]RawReading  `json:"raw,omitempty"`
	Data            map[string]float32     `json:"data,omitempty"`
}

// PinConfig định nghĩa chân kết nối cảm biến trên ESP32-S3
type PinConfig struct {
	// DS18B20 Nhiệt độ (OneWire)
	TempDataPin  uint8
	TempPowerPin uint8

	// pH Sensor (ADC + VCC Control)
	PHADCPin   uint8
	PHPowerPin uint8

	// TDS Sensor (ADC + VCC Control)
	TDSADCPin   uint8
	TDSPowerPin uint8

	// Nút BOOT & Đèn báo trạng thái
	BootButtonPin uint8
	StatusLEDPin  uint8
}

// DefaultPinConfig trả về cấu hình chân mặc định tối ưu cho ESP32-S3
func DefaultPinConfig() PinConfig {
	return PinConfig{
		TempDataPin:   4,  // GPIO 4 (OneWire Data)
		TempPowerPin:  5,  // GPIO 5 (VCC Enable)
		PHADCPin:      6,  // GPIO 6 (ADC1_CH5)
		PHPowerPin:    7,  // GPIO 7 (VCC Enable)
		TDSADCPin:     8,  // GPIO 8 (ADC1_CH7)
		TDSPowerPin:   9,  // GPIO 9 (VCC Enable)
		BootButtonPin: 0,  // GPIO 0 (Nút BOOT)
		StatusLEDPin:  48, // GPIO 48 (RGB / WS2812 hoặc Status LED)
	}
}
