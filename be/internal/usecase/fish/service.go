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
	"github.com/nhuhuynh/iot-fish/internal/pkg/advice"
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
	narrator  advice.Narrator
	pondRepo  fish.PondConfigRepository
	kitRepo   fish.KitReadingRepository
}

func NewService(
	repo fish.MeasurementRepository,
	eventRepo fish.EventRepository,
	devRepo fish.DeviceRepository,
	calRepo fish.CalibrationRepository,
	pub fish.CommandPublisher,
	hub fish.EventHub,
) *Service {
	s := &Service{
		repo:      repo,
		eventRepo: eventRepo,
		devRepo:   devRepo,
		calRepo:   calRepo,
		pub:       pub,
		hub:       hub,
	}
	if p, ok := calRepo.(fish.PondConfigRepository); ok {
		s.pondRepo = p
	} else if p, ok := repo.(fish.PondConfigRepository); ok {
		s.pondRepo = p
	}
	if k, ok := repo.(fish.KitReadingRepository); ok {
		s.kitRepo = k
	} else if k, ok := calRepo.(fish.KitReadingRepository); ok {
		s.kitRepo = k
	}
	return s
}

func (s *Service) SetScheduleRegistrar(sched ScheduleRegistrar) {
	s.sched = sched
}

func (s *Service) SetNarrator(n advice.Narrator) {
	s.narrator = n
}

// 1. Gửi từng action đo (ph / temp / tds). pH và TDS tự bơm tới phao; nhiệt độ đo thẳng.
func (s *Service) TriggerMeasurement(ctx context.Context, deviceID string, sensors []string) error {
	if len(sensors) == 0 {
		sensors = []string{"all"}
	}
	log.Printf("[Usecase] Gửi lệnh đo tới device [%s] với cảm biến: %v", deviceID, sensors)
	return s.pub.PublishMeasure(ctx, deviceID, sensors)
}

// 2. Điều khiển bơm nạp / xả thủ công
func (s *Service) SetPump(ctx context.Context, deviceID string, target string, state bool, level int) error {
	t := strings.ToLower(strings.TrimSpace(target))
	if t != "inlet" && t != "drain" {
		return fmt.Errorf("pump target must be \"inlet\" or \"drain\", got %q", target)
	}
	if t == "inlet" && state && level != 1 {
		level = 2
	}
	log.Printf("[Usecase] Điều khiển bơm [%s] trên device [%s]: state=%v level=%d", t, deviceID, state, level)
	return s.pub.PublishPump(ctx, deviceID, t, state, level)
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
		DeviceID   string `json:"device_id"`
		Stage      string `json:"stage"`
		State      string `json:"state"`
		Message    string `json:"message"`
		InletOn    *bool  `json:"inlet_on"`
		DrainOn    *bool  `json:"drain_on"`
		FloatFull  *bool  `json:"float_full"`
		FloatEmpty *bool  `json:"float_empty"`
	}

	if err := json.Unmarshal(payload, &body); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	if body.DeviceID == "" {
		body.DeviceID = deviceID
	}

	e := &fish.SamplingEvent{
		ID:         uuid.New().String(),
		DeviceID:   body.DeviceID,
		Stage:      body.Stage,
		State:      body.State,
		Message:    body.Message,
		InletOn:    body.InletOn,
		DrainOn:    body.DrainOn,
		FloatFull:  body.FloatFull,
		FloatEmpty: body.FloatEmpty,
		CreatedAt:  time.Now(),
	}

	if err := s.eventRepo.SaveEvent(ctx, e); err != nil {
		return fmt.Errorf("save event: %w", err)
	}

	// Cập nhật state device
	s.updateDeviceSeen(ctx, body.DeviceID, body.State)

	// Broadcast realtime event qua WebSocket
	if s.hub != nil {
		s.hub.Broadcast("sampling_event", e)
	}

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

func (s *Service) GetAdvice(ctx context.Context, deviceID string, profile advice.Profile) (*advice.Result, error) {
	if s.narrator == nil {
		return nil, advice.ErrLLMNotConfigured
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	history, err := s.repo.ListMeasurementsSince(ctx, deviceID, since, 4000)
	if err != nil {
		return nil, err
	}
	cfg := s.loadPondConfig(ctx)
	merged := advice.Profile{
		VolumeL:   cfg.VolumeL,
		Species:   cfg.Species,
		HasFilter: &cfg.HasFilter,
	}
	if profile.VolumeL > 0 {
		merged.VolumeL = profile.VolumeL
	}
	if strings.TrimSpace(profile.Species) != "" {
		merged.Species = profile.Species
	}
	if profile.HasFilter != nil {
		merged.HasFilter = profile.HasFilter
	}
	th := toAdviceThresholds(cfg.Thresholds)
	var kits []fish.KitReading
	if s.kitRepo != nil {
		kits, err = s.kitRepo.ListKitReadingsSince(ctx, deviceID, since, 400)
		if err != nil {
			return nil, err
		}
	}
	res := advice.AnalyzeWithKit(history, kits, merged, &th)
	if err := advice.Enrich(ctx, res, s.narrator); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) SaveKitReading(ctx context.Context, r *fish.KitReading) (*fish.KitReading, error) {
	if s.kitRepo == nil {
		return nil, fmt.Errorf("kit reading storage not configured")
	}
	if r == nil || strings.TrimSpace(r.DeviceID) == "" {
		return nil, fmt.Errorf("missing device id")
	}
	if r.DOMGL == nil && r.TANMGL == nil {
		return nil, fmt.Errorf("cần ít nhất oxy (do_mg_l) hoặc amonia (tan_mg_l)")
	}
	if r.DOMGL != nil && (*r.DOMGL < 0 || *r.DOMGL > 20) {
		return nil, fmt.Errorf("do_mg_l phải từ 0–20")
	}
	if r.TANMGL != nil && (*r.TANMGL < 0 || *r.TANMGL > 20) {
		return nil, fmt.Errorf("tan_mg_l phải từ 0–20")
	}
	if latest, err := s.repo.GetLatestMeasurement(ctx, r.DeviceID); err == nil && latest != nil {
		r.PHUsed = latest.PH
		r.TempUsed = latest.Temperature
	}
	tempC := 25.0
	if r.TempUsed != nil {
		tempC = *r.TempUsed
	}
	phV := 7.0
	if r.PHUsed != nil {
		phV = *r.PHUsed
	}
	if r.TANMGL != nil {
		v := advice.FreeAmmonia(*r.TANMGL, phV, tempC)
		r.NH3FreeMGL = &v
	}
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.Source == "" {
		r.Source = "kit"
	}
	now := time.Now()
	if r.MeasuredAt.IsZero() {
		r.MeasuredAt = now
	}
	r.CreatedAt = now
	if err := s.kitRepo.SaveKitReading(ctx, r); err != nil {
		return nil, err
	}
	if s.hub != nil {
		s.hub.Broadcast("kit_reading", r)
	}
	return r, nil
}

func (s *Service) ListKitReadings(ctx context.Context, deviceID string, limit int) ([]fish.KitReading, error) {
	if s.kitRepo == nil {
		return []fish.KitReading{}, nil
	}
	return s.kitRepo.ListKitReadings(ctx, deviceID, limit)
}

func (s *Service) GetPondConfig(ctx context.Context) (*fish.PondConfig, error) {
	return s.loadPondConfig(ctx), nil
}

func (s *Service) UpdatePondConfig(ctx context.Context, patch *fish.PondConfig) (*fish.PondConfig, error) {
	if s.pondRepo == nil {
		return nil, fmt.Errorf("pond config storage not configured")
	}
	cur := s.loadPondConfig(ctx)
	if patch == nil {
		return cur, nil
	}
	speciesChanged := false
	if sp := strings.ToLower(strings.TrimSpace(patch.Species)); sp != "" && sp != cur.Species {
		cur.Species = sp
		speciesChanged = true
	}
	if patch.VolumeL > 0 {
		cur.VolumeL = patch.VolumeL
	}
	if patch.VolumeM3 > 0 {
		cur.VolumeL = patch.VolumeM3 * 1000
	}
	cur.HasFilter = patch.HasFilter
	if speciesChanged && patch.Thresholds == (fish.WaterThresholds{}) {
		cur.Thresholds = fromAdviceThresholds(advice.ThresholdsFor(cur.Species))
	}
	if patch.Thresholds != (fish.WaterThresholds{}) {
		cur.Thresholds = patch.Thresholds
	}
	cur.UpdatedAt = time.Now()
	cur = cur.Normalize()
	if err := s.pondRepo.SavePondConfig(ctx, cur); err != nil {
		return nil, err
	}
	if s.hub != nil {
		s.hub.Broadcast("pond_config_updated", cur)
	}
	return cur, nil
}

func (s *Service) loadPondConfig(ctx context.Context) *fish.PondConfig {
	if s.pondRepo == nil {
		return fish.DefaultPondConfig()
	}
	c, err := s.pondRepo.GetPondConfig(ctx)
	if err != nil || c == nil {
		return fish.DefaultPondConfig()
	}
	return c.Normalize()
}

func toAdviceThresholds(t fish.WaterThresholds) advice.Thresholds {
	return advice.Thresholds{
		TempMin: t.TempMin, TempMax: t.TempMax,
		PHMin: t.PHMin, PHMax: t.PHMax,
		TurbidityWarn: t.TurbidityWarn, TurbidityMax: t.TurbidityMax,
		TDSMin: t.TDSMin, TDSMax: t.TDSMax,
	}
}

func fromAdviceThresholds(t advice.Thresholds) fish.WaterThresholds {
	return fish.WaterThresholds{
		TempMin: t.TempMin, TempMax: t.TempMax,
		PHMin: t.PHMin, PHMax: t.PHMax,
		TurbidityWarn: t.TurbidityWarn, TurbidityMax: t.TurbidityMax,
		TDSMin: t.TDSMin, TDSMax: t.TDSMax,
	}
}

func (s *Service) ListEvents(ctx context.Context, deviceID string, limit int) ([]fish.SamplingEvent, error) {
	return s.eventRepo.ListEvents(ctx, deviceID, limit)
}

func (s *Service) ListDevices(ctx context.Context) ([]fish.Device, error) {
	return s.devRepo.ListDevices(ctx)
}
