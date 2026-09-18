package fishuc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/pkg/calibrate"
)

type ScheduleRegistrar interface {
	EnsureDevice(deviceID string)
}

type Service struct {
	repo      fish.MeasurementRepository
	eventRepo fish.EventRepository
	devRepo   fish.DeviceRepository
	calRepo   fish.CalibrationRepository
	pub       fish.CommandPublisher
	hub       fish.EventHub
	sched     ScheduleRegistrar
}

func NewService(
	repo fish.MeasurementRepository,
	eventRepo fish.EventRepository,
	devRepo fish.DeviceRepository,
	calRepo fish.CalibrationRepository,
	pub fish.CommandPublisher,
	hub fish.EventHub,
) *Service {
	return &Service{
		repo:      repo,
		eventRepo: eventRepo,
		devRepo:   devRepo,
		calRepo:   calRepo,
		pub:       pub,
		hub:       hub,
	}
}

func (s *Service) SetScheduleRegistrar(sched ScheduleRegistrar) {
	s.sched = sched
}

// 1. Kích hoạt đo lường từ Backend
func (s *Service) TriggerMeasurement(ctx context.Context, deviceID string, sensors []string) error {
	if len(sensors) == 0 {
		sensors = []string{"all"}
	}
	log.Printf("[Usecase] Gửi lệnh đo tới device [%s] với cảm biến: %v", deviceID, sensors)
	return s.pub.PublishMeasure(ctx, deviceID, sensors)
}

// 2. Điều khiển bơm nạp / xả thủ công
func (s *Service) SetPump(ctx context.Context, deviceID string, target string, state bool) error {
	t := strings.ToLower(strings.TrimSpace(target))
	if t != "inlet" && t != "drain" {
		return fmt.Errorf("pump target must be \"inlet\" or \"drain\", got %q", target)
	}
	log.Printf("[Usecase] Điều khiển bơm [%s] trên device [%s]: state=%v", t, deviceID, state)
	return s.pub.PublishPump(ctx, deviceID, t, state)
}

func (s *Service) ClearQueue(ctx context.Context, deviceID string) error {
	log.Printf("[Usecase] Xóa hàng đợi trên device [%s]", deviceID)
	return s.pub.PublishClearQueue(ctx, deviceID)
}

// 2b. Cấu hình lịch đo tự động
func (s *Service) SetSchedule(ctx context.Context, deviceID string, enabled bool, temp, ph, turb, tds int) error {
	log.Printf("[Usecase] Cấu hình lịch đo trên device [%s]: auto=%v, temp=%ds, ph=%ds, turb=%ds, tds=%ds",
		deviceID, enabled, temp, ph, turb, tds)
	return s.pub.PublishSchedule(ctx, deviceID, enabled, temp, ph, turb, tds)
}

// 2c. Bật/tắt chế độ tự động đo
func (s *Service) ToggleAuto(ctx context.Context, deviceID string, enabled bool) error {
	log.Printf("[Usecase] Bật/tắt tự động đo trên device [%s]: enabled=%v", deviceID, enabled)
	return s.pub.PublishAutoToggle(ctx, deviceID, enabled)
}

type rawReading struct {
	ADC         int     `json:"adc"`
	Voltage     float64 `json:"voltage"`
	SampleCount int     `json:"sample_count"`
}

// 3. Xử lý nhận dữ liệu cảm biến đo xong từ MQTT
func (s *Service) HandleSensorData(ctx context.Context, deviceID string, payload []byte) error {
	log.Printf("[Usecase] Nhận sensor_data từ [%s]: %s", deviceID, string(payload))
	var body struct {
		DeviceID   string   `json:"device_id"`
		Timestamp  int64    `json:"timestamp"`
		Status     string   `json:"status"`
		DurationMs int64    `json:"duration_ms"`
		Sensors    []string `json:"sensors_measured"`
		Raw        struct {
			PH        *rawReading `json:"ph"`
			TDS       *rawReading `json:"tds"`
			Turbidity *rawReading `json:"turbidity"`
		} `json:"raw"`
		Data struct {
			Temperature *float64 `json:"temperature"`
			PH          *float64 `json:"ph"`
			Turbidity   *float64 `json:"turbidity"`
			TDS         *float64 `json:"tds"`
		} `json:"data"`
	}

	if err := json.Unmarshal(payload, &body); err != nil {
		return fmt.Errorf("unmarshal sensor data: %w", err)
	}

	if body.DeviceID == "" {
		body.DeviceID = deviceID
	}

	m := &fish.Measurement{
		ID:         uuid.New().String(),
		DeviceID:   body.DeviceID,
		Timestamp:  body.Timestamp,
		Status:     body.Status,
		DurationMs: body.DurationMs,
		Sensors:    body.Sensors,
		CreatedAt:  time.Now(),
	}

	hasRaw := body.Raw.PH != nil || body.Raw.TDS != nil || body.Raw.Turbidity != nil
	if hasRaw {
		cal, err := s.getCalibration(ctx, body.DeviceID)
		if err != nil {
			return err
		}
		s.applyRawReadings(m, body.Raw.PH, body.Raw.TDS, body.Raw.Turbidity, cal)
	} else {
		m.Temperature = body.Data.Temperature
		m.PH = body.Data.PH
		m.Turbidity = body.Data.Turbidity
		m.TDS = body.Data.TDS
	}

	if err := s.repo.SaveMeasurement(ctx, m); err != nil {
		return fmt.Errorf("save measurement: %w", err)
	}

	s.updateDeviceSeen(ctx, body.DeviceID, "IDLE")
	if s.hub != nil {
		s.hub.Broadcast("sensor_data", m)
	}
	log.Printf("[Usecase] Đã lưu và broadcast kết quả đo từ [%s]", body.DeviceID)
	return nil
}

func (s *Service) getCalibration(ctx context.Context, deviceID string) (*fish.DeviceCalibration, error) {
	if s.calRepo == nil {
		return fish.DefaultCalibration(deviceID), nil
	}
	cal, err := s.calRepo.GetCalibration(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return fish.DefaultCalibration(deviceID), nil
	}
	return cal, nil
}

func (s *Service) applyRawReadings(
	m *fish.Measurement,
	phRaw, tdsRaw, turbRaw *rawReading,
	cal *fish.DeviceCalibration,
) {
	if phRaw != nil {
		m.PHAdc = &phRaw.ADC
		m.PHVoltage = &phRaw.Voltage
		ph := calibrate.CalcPH(phRaw.Voltage, cal.PHNeutralV, cal.PHSlope)
		m.PH = &ph
	}
	if tdsRaw != nil {
		m.TDSAdc = &tdsRaw.ADC
		m.TDSVoltage = &tdsRaw.Voltage
		tds := calibrate.CalcTDS(tdsRaw.Voltage, cal.TDSTempC, cal.TDSRefV, cal.TDSRefPPM, cal.TDSMaxPPM)
		m.TDS = &tds
	}
	if turbRaw != nil {
		m.TurbidityAdc = &turbRaw.ADC
		m.TurbidityVoltage = &turbRaw.Voltage
		ntu := calibrate.CalcTurbidity(turbRaw.Voltage, cal.TurbVClear, cal.TurbVDirty, cal.TurbNTUMax)
		m.Turbidity = &ntu
	}
}

func (s *Service) GetCalibration(ctx context.Context, deviceID string) (*fish.DeviceCalibration, error) {
	return s.getCalibration(ctx, deviceID)
}

func (s *Service) UpdateCalibration(ctx context.Context, cal *fish.DeviceCalibration) error {
	if cal.DeviceID == "" {
		return fmt.Errorf("device_id required")
	}
	if s.calRepo == nil {
		return fmt.Errorf("calibration storage not configured")
	}
	cal.UpdatedAt = time.Now()
	if err := s.calRepo.SaveCalibration(ctx, cal); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.Broadcast("calibration_updated", cal)
	}
	return nil
}

// 4. Xử lý nhận sự kiện tiến trình (Event Stream) từ MQTT
func (s *Service) HandleEvent(ctx context.Context, deviceID string, payload []byte) error {
	var body struct {
		DeviceID string `json:"device_id"`
		Stage    string `json:"stage"`
		State    string `json:"state"`
		Message  string `json:"message"`
	}

	if err := json.Unmarshal(payload, &body); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	if body.DeviceID == "" {
		body.DeviceID = deviceID
	}

	e := &fish.SamplingEvent{
		ID:        uuid.New().String(),
		DeviceID:  body.DeviceID,
		Stage:     body.Stage,
		State:     body.State,
		Message:   body.Message,
		CreatedAt: time.Now(),
	}

	if err := s.eventRepo.SaveEvent(ctx, e); err != nil {
		return fmt.Errorf("save event: %w", err)
	}

	// Cập nhật state device
	s.updateDeviceSeen(ctx, body.DeviceID, body.State)

	// Broadcast realtime event qua WebSocket
	s.hub.Broadcast("sampling_event", e)
	return nil
}

// 5. Xử lý trạng thái online/offline từ MQTT
func (s *Service) HandleStatus(ctx context.Context, deviceID string, payload []byte) error {
	var body struct {
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
		IP       string `json:"ip"`
	}

	online := false
	ip := ""

	if err := json.Unmarshal(payload, &body); err == nil && body.Status != "" {
		online = strings.ToLower(body.Status) == "online"
		ip = body.IP
	} else {
		online = strings.ToLower(strings.TrimSpace(string(payload))) == "online"
	}

	d, _ := s.devRepo.GetDevice(ctx, deviceID)
	if d == nil {
		d = &fish.Device{ID: deviceID}
	}
	d.Online = online
	d.LastSeen = time.Now()
	if ip != "" {
		d.IP = ip
	}

	if err := s.devRepo.UpsertDevice(ctx, d); err != nil {
		return err
	}

	if s.sched != nil {
		s.sched.EnsureDevice(deviceID)
	}

	s.hub.Broadcast("device_status", d)
	log.Printf("[Usecase] Device [%s] status: online=%v", deviceID, online)
	return nil
}

// 6. Xử lý telemetry định kỳ
func (s *Service) HandleTelemetry(ctx context.Context, deviceID string, payload []byte) error {
	var body struct {
		RSSI   int    `json:"rssi"`
		Uptime int64  `json:"uptime"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return err
	}

	d, _ := s.devRepo.GetDevice(ctx, deviceID)
	if d == nil {
		d = &fish.Device{ID: deviceID, Online: true}
	}
	d.LastSeen = time.Now()
	d.RSSI = body.RSSI
	d.Uptime = body.Uptime
	if body.State != "" {
		d.State = body.State
	}

	if err := s.devRepo.UpsertDevice(ctx, d); err != nil {
		return err
	}

	if s.sched != nil {
		s.sched.EnsureDevice(deviceID)
	}

	s.hub.Broadcast("device_status", d)
	return nil
}

// 7. Xử lý Serial log từ ESP32 (topic fish/+/log) — chỉ broadcast realtime, không lưu DB
func (s *Service) HandleDeviceLog(ctx context.Context, deviceID string, payload []byte) error {
	var body struct {
		DeviceID string `json:"device_id"`
		UptimeMs int64  `json:"uptime_ms"`
		Msg      string `json:"msg"`
	}

	if err := json.Unmarshal(payload, &body); err != nil {
		// Fallback: payload plain text
		body.Msg = strings.TrimSpace(string(payload))
	}
	if body.DeviceID == "" {
		body.DeviceID = deviceID
	}
	if body.Msg == "" {
		return nil
	}

	entry := map[string]any{
		"device_id":  body.DeviceID,
		"uptime_ms":  body.UptimeMs,
		"msg":        body.Msg,
		"created_at": time.Now(),
	}

	if s.hub != nil {
		s.hub.Broadcast("device_log", entry)
	}
	return nil
}

func (s *Service) updateDeviceSeen(ctx context.Context, deviceID string, state string) {
	d, _ := s.devRepo.GetDevice(ctx, deviceID)
	if d == nil {
		d = &fish.Device{ID: deviceID, Online: true}
	}
	d.LastSeen = time.Now()
	d.Online = true
	if state != "" {
		d.State = state
	}
	_ = s.devRepo.UpsertDevice(ctx, d)
	if s.sched != nil {
		s.sched.EnsureDevice(deviceID)
	}
}

func (s *Service) GetLatest(ctx context.Context, deviceID string) (*fish.Measurement, error) {
	return s.repo.GetLatestMeasurement(ctx, deviceID)
}

func (s *Service) ListHistory(ctx context.Context, deviceID string, limit int) ([]fish.Measurement, error) {
	return s.repo.ListMeasurements(ctx, deviceID, limit)
}

func (s *Service) ListEvents(ctx context.Context, deviceID string, limit int) ([]fish.SamplingEvent, error) {
	return s.eventRepo.ListEvents(ctx, deviceID, limit)
}

func (s *Service) ListDevices(ctx context.Context) ([]fish.Device, error) {
	return s.devRepo.ListDevices(ctx)
}
