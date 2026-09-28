package sensor

import (
	"encoding/json"
	"fmt"
	"machine"
	"time"
)

// Manager điều phối toàn bộ chu trình cấp nguồn và đọc cảm biến
type Manager struct {
	pins       PinConfig
	tempDriver *DS18B20Driver
	phDriver   *PHSensorDriver
	tdsDriver  *TDSSensorDriver
}

// NewManager khởi tạo Sensor Manager với cấu hình chân
func NewManager(pins PinConfig) *Manager {
	return &Manager{
		pins:       pins,
		tempDriver: NewDS18B20Driver(machine.Pin(pins.TempDataPin), machine.Pin(pins.TempPowerPin)),
		phDriver:   NewPHSensorDriver(machine.Pin(pins.PHADCPin), machine.Pin(pins.PHPowerPin)),
		tdsDriver:  NewTDSSensorDriver(machine.Pin(pins.TDSADCPin), machine.Pin(pins.TDSPowerPin)),
	}
}

// PowerOffAll tắt nguồn toàn bộ cảm biến
func (m *Manager) PowerOffAll() {
	m.tempDriver.SetPower(false)
	m.phDriver.SetPower(false)
	m.tdsDriver.SetPower(false)
}

// MeasureTemperature đọc riêng nhiệt độ
func (m *Manager) MeasureTemperature(deviceID string) (*SensorPayload, error) {
	start := time.Now()
	temp, err := m.tempDriver.ReadTemperatureC()
	duration := uint32(time.Since(start).Milliseconds())

	if err != nil {
		return nil, err
	}

	payload := &SensorPayload{
		DeviceID:        deviceID,
		Timestamp:       time.Now().Unix(),
		Status:          "success",
		DurationMs:      duration,
		SensorsMeasured: []string{"temperature"},
		Data: map[string]float32{
			"temperature": temp,
		},
	}
	return payload, nil
}

// MeasurePH đọc riêng pH (đã qua lọc trung vị)
func (m *Manager) MeasurePH(deviceID string) (*SensorPayload, error) {
	start := time.Now()
	raw, err := m.phDriver.ReadRaw()
	duration := uint32(time.Since(start).Milliseconds())

	if err != nil {
		return nil, err
	}

	payload := &SensorPayload{
		DeviceID:        deviceID,
		Timestamp:       time.Now().Unix(),
		Status:          "success",
		DurationMs:      duration,
		SensorsMeasured: []string{"ph"},
		Raw: map[string]RawReading{
			"ph": raw,
		},
	}
	return payload, nil
}

// MeasureTDS đọc riêng TDS (đã qua lọc trung vị)
func (m *Manager) MeasureTDS(deviceID string) (*SensorPayload, error) {
	start := time.Now()
	raw, err := m.tdsDriver.ReadRaw()
	duration := uint32(time.Since(start).Milliseconds())

	if err != nil {
		return nil, err
	}

	payload := &SensorPayload{
		DeviceID:        deviceID,
		Timestamp:       time.Now().Unix(),
		Status:          "success",
		DurationMs:      duration,
		SensorsMeasured: []string{"tds"},
		Raw: map[string]RawReading{
			"tds": raw,
		},
	}
	return payload, nil
}

// MeasureAll thực hiện quy trình đo tuần tự an toàn:
// 1. Nhiệt độ -> 2. pH -> 3. TDS (không cấp nguồn cùng lúc để tránh dòng rò trong nước)
func (m *Manager) MeasureAll(deviceID string) (*SensorPayload, error) {
	start := time.Now()
	measured := make([]string, 0, 3)
	rawMap := make(map[string]RawReading)
	dataMap := make(map[string]float32)

	// 1. Đo nhiệt độ
	fmt.Println("[Sensor] Bắt đầu đo nhiệt độ DS18B20...")
	temp, err := m.tempDriver.ReadTemperatureC()
	if err == nil {
		dataMap["temperature"] = temp
		measured = append(measured, "temperature")
	} else {
		fmt.Printf("[Sensor] Cảnh báo lỗi DS18B20: %v\n", err)
	}

	// Nghỉ 200ms giữa các lần chuyển mạch
	time.Sleep(200 * time.Millisecond)

	// 2. Đo pH
	fmt.Println("[Sensor] Bắt đầu đo pH...")
	phRaw, err := m.phDriver.ReadRaw()
	if err == nil {
		rawMap["ph"] = phRaw
		measured = append(measured, "ph")
	} else {
		fmt.Printf("[Sensor] Cảnh báo lỗi pH: %v\n", err)
	}

	time.Sleep(200 * time.Millisecond)

	// 3. Đo TDS
	fmt.Println("[Sensor] Bắt đầu đo TDS...")
	tdsRaw, err := m.tdsDriver.ReadRaw()
	if err == nil {
		rawMap["tds"] = tdsRaw
		measured = append(measured, "tds")
	} else {
		fmt.Printf("[Sensor] Cảnh báo lỗi TDS: %v\n", err)
	}

	duration := uint32(time.Since(start).Milliseconds())

	payload := &SensorPayload{
		DeviceID:        deviceID,
		Timestamp:       time.Now().Unix(),
		Status:          "success",
		DurationMs:      duration,
		SensorsMeasured: measured,
		Raw:             rawMap,
		Data:            dataMap,
	}

	return payload, nil
}

// FormatJSON chuyển payload thành chuỗi JSON để publish MQTT
func (m *Manager) FormatJSON(payload *SensorPayload) (string, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
