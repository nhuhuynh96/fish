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
)

type Service struct {
	repo      fish.MeasurementRepository
	eventRepo fish.EventRepository
	devRepo   fish.DeviceRepository
	pub       fish.CommandPublisher
	hub       fish.EventHub
}

func NewService(
	repo fish.MeasurementRepository,
	eventRepo fish.EventRepository,
	devRepo fish.DeviceRepository,
	pub fish.CommandPublisher,
	hub fish.EventHub,
) *Service {
	return &Service{
		repo:      repo,
		eventRepo: eventRepo,
		devRepo:   devRepo,
		pub:       pub,
		hub:       hub,
	}
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
	log.Printf("[Usecase] Điều khiển bơm [%s] trên device [%s]: state=%v", target, deviceID, state)
	return s.pub.PublishPump(ctx, deviceID, target, state)
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

// 3. Xử lý nhận dữ liệu cảm biến đo xong từ MQTT
func (s *Service) HandleSensorData(ctx context.Context, deviceID string, payload []byte) error {
	log.Printf("[Usecase] Nhận sensor_data thô từ [%s]: %s", deviceID, string(payload))
	var body struct {
		DeviceID   string   `json:"device_id"`
		Timestamp  int64    `json:"timestamp"`
		Status     string   `json:"status"`
		DurationMs int64    `json:"duration_ms"`
		Sensors    []string `json:"sensors_measured"`
		Data       struct {
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
		ID:          uuid.New().String(),
		DeviceID:    body.DeviceID,
		Timestamp:   body.Timestamp,
		Status:      body.Status,
		DurationMs:  body.DurationMs,
		Sensors:     body.Sensors,
		Temperature: body.Data.Temperature,
		PH:          body.Data.PH,
		Turbidity:   body.Data.Turbidity,
		TDS:         body.Data.TDS,
		CreatedAt:   time.Now(),
	}

	if err := s.repo.SaveMeasurement(ctx, m); err != nil {
		return fmt.Errorf("save measurement: %w", err)
	}

	// Cập nhật trạng thái device
	s.updateDeviceSeen(ctx, body.DeviceID, "IDLE")

	// Broadcast dữ liệu mới tới tất cả Client qua WebSocket
	s.hub.Broadcast("sensor_data", m)
	log.Printf("[Usecase] Đã lưu và broadcast kết quả đo từ [%s]", body.DeviceID)
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

	s.hub.Broadcast("device_status", d)
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
