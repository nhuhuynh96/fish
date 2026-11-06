package fishuc

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

const (
	waterDeviceID        = "water"
	sensorDeviceID       = "sensor"
	phDeviceID           = "ph"
	floatWaitLimit       = 30 * time.Second
	waterWaitSlack       = 30 * time.Second
	sensorWaitLimit      = 60 * time.Second
	sensorPowerWaitLimit = 10 * time.Second
	sensorMaxOnS         = 300
)

// Đơn vị của SettleS; test đổi sang mili giây để chạy nhanh.
var settleUnit = time.Second

type sensorTiming struct {
	Warmup     time.Duration
	Samples    int
	IntervalMs int
}

// pH và TDS cùng ở bo ph. pH RS485 cấp điện liên tục nên không cần làm ấm; TDS chỉ có điện lúc đọc.
var sensorTimings = map[string]sensorTiming{
	"ph":  {Samples: 10, IntervalMs: 500},
	"tds": {Warmup: 5 * time.Second, Samples: 20, IntervalMs: 100},
}

type waterSig struct {
	Stage string
	Full  bool
}

type sensorSig struct {
	Device  string
	Stage   string
	Names   []string
	Sensor  string
	Power   bool
	Failed  bool
	Message string
}

func (s *Service) jobPondID() string {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if !s.jobRunning {
		return ""
	}
	return s.jobPond
}

func (s *Service) armWater() chan waterSig {
	ch := make(chan waterSig, 8)
	s.jobMu.Lock()
	s.waterWait = ch
	s.jobMu.Unlock()
	return ch
}

func (s *Service) armSensor() chan sensorSig {
	ch := make(chan sensorSig, 8)
	s.jobMu.Lock()
	s.sensorWait = ch
	s.jobMu.Unlock()
	return ch
}

func (s *Service) noteWater(topicID, bodyID, stage string, full bool) {
	id := bodyID
	if id == "" {
		id = topicID
	}
	if id != waterDeviceID && topicID != waterDeviceID {
		return
	}
	s.jobMu.Lock()
	ch := s.waterWait
	s.jobMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- waterSig{Stage: stage, Full: full}:
	default:
	}
}

func (s *Service) noteSensorEvent(topicID, bodyID, stage, sensor string, power bool, message string) {
	switch stage {
	case "sensor_error", "command_error":
		s.noteSensor(topicID, bodyID, sensorSig{Stage: stage, Failed: true, Message: message})
	case "power", "power_timeout":
		s.noteSensor(topicID, bodyID, sensorSig{Stage: stage, Sensor: sensor, Power: power, Message: message})
	}
}

func (s *Service) noteSensor(topicID, bodyID string, sig sensorSig) {
	id := bodyID
	if id == "" {
		id = topicID
	}
	if id != sensorDeviceID && id != phDeviceID {
		return
	}
	sig.Device = id
	s.jobMu.Lock()
	ch := s.sensorWait
	s.jobMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- sig:
	default:
	}
}

func (s *Service) finishJob() {
	s.jobMu.Lock()
	s.jobRunning = false
	s.jobPond = ""
	s.waterWait = nil
	s.sensorWait = nil
	s.jobMu.Unlock()
}

// Lượt đo tự động: xả nước cũ, bơm nước mới tới khi phao đầy, chờ ổn định, đo.
// Đo xong không xả: đầu dò pH phải luôn ngập để bầu thủy tinh không khô.
func (s *Service) runMeasure(sensors []string, timing fish.CycleTiming) {
	defer s.finishJob()
	ctx := context.Background()
	want := normalizeSensors(sensors)
	if len(want) == 0 {
		s.recordJob(ctx, sensorDeviceID, "job_failed", "Không có chỉ số đo được hỗ trợ.")
		return
	}
	log.Printf("[MeasureJob] đo %v: xả %ds, bơm tối đa %ds, chờ %ds", want, timing.DrainS, timing.FillMaxS, timing.SettleS)

	measureErr := s.resetSensorBoard(ctx, want)
	if measureErr == nil {
		measureErr = s.refreshWater(ctx, timing)
	}
	for _, action := range want {
		if measureErr != nil {
			break
		}
		measureErr = s.measureSensor(ctx, action)
	}
	if measureErr != nil {
		s.recordJob(ctx, sensorDeviceID, "job_failed", measureErr.Error())
		return
	}
	s.recordJob(ctx, sensorDeviceID, "job_done", "Đo xong, giữ nước trong bình để đầu dò pH luôn ngập.")
}

// Xả hết nước cũ, kiểm tra phao đã tắt, bơm tới khi phao đầy, chờ nước ổn định.
// Phao vẫn đầy sau khi xả nghĩa là bơm xả hoặc phao hỏng; dừng để không đo nước cũ.
func (s *Service) refreshWater(ctx context.Context, timing fish.CycleTiming) error {
	s.recordJob(ctx, waterDeviceID, "drain", fmt.Sprintf("Xả nước cũ %d giây.", timing.DrainS))
	stage, err := s.runWater(ctx, "drain", timing.DrainS)
	if err != nil {
		return fmt.Errorf("xả nước: %w", err)
	}
	if stage != "time_up" {
		return fmt.Errorf("xả nước: bơm xả bị tắt giữa chừng (%s)", stage)
	}

	full, err := s.queryFloat(ctx)
	if err != nil {
		return fmt.Errorf("hỏi phao: %w", err)
	}
	if full {
		return fmt.Errorf("xả %d giây xong phao vẫn đầy, kiểm tra bơm xả hoặc phao", timing.DrainS)
	}

	s.recordJob(ctx, waterDeviceID, "fill", fmt.Sprintf("Bơm nạp tới khi phao đầy, tối đa %d giây.", timing.FillMaxS))
	stage, err = s.runWater(ctx, "inlet", timing.FillMaxS)
	if err != nil {
		return fmt.Errorf("bơm nạp: %w", err)
	}
	switch stage {
	case "filled", "already_full":
	case "time_up":
		return fmt.Errorf("bơm nạp: phao chưa đầy sau %d giây", timing.FillMaxS)
	default:
		return fmt.Errorf("bơm nạp bị tắt giữa chừng (%s)", stage)
	}

	if timing.SettleS > 0 {
		s.recordJob(ctx, waterDeviceID, "settle", fmt.Sprintf("Chờ %d giây cho nước ổn định rồi đo.", timing.SettleS))
		if err := sleepCtx(ctx, time.Duration(timing.SettleS)*settleUnit); err != nil {
			return err
		}
	}
	return nil
}

// Đo ngay từ dashboard: bật nguồn, chờ, đọc, tắt từng sensor; không bơm hay xả.
func (s *Service) runManualMeasure(sensors []string) {
	defer s.finishJob()
	ctx := context.Background()
	want := normalizeSensors(sensors)
	log.Printf("[MeasureJob] đo ngay %v, không chỉnh mực nước", want)

	measureErr := s.resetSensorBoard(ctx, want)
	for _, action := range want {
		if measureErr != nil {
			break
		}
		measureErr = s.measureSensor(ctx, action)
	}
	if measureErr != nil {
		s.recordJob(ctx, sensorDeviceID, "job_failed", measureErr.Error())
		return
	}
	s.recordJob(ctx, sensorDeviceID, "job_done", "Đo xong.")
}

// TDS: bật nguồn, chờ làm ấm, đọc, rồi luôn tắt nguồn. pH không bật tắt nguồn.
func (s *Service) measureSensor(ctx context.Context, action string) (err error) {
	t := sensorTimings[action]
	label := sensorLabel(action)
	if action == "ph" {
		s.recordJob(ctx, sensorDeviceID, "measure", "Đo pH (RS485).")
		return s.measureOne(ctx, action, t)
	}
	// Tắt cả khi lệnh bật lỗi: mất phản hồi không có nghĩa là mạch chưa bật.
	defer func() {
		if offErr := s.sensorPower(context.Background(), action, false); offErr != nil && err == nil {
			err = fmt.Errorf("tắt nguồn %s: %w", label, offErr)
		}
	}()
	if err := s.sensorPower(ctx, action, true); err != nil {
		return fmt.Errorf("bật nguồn %s: %w", label, err)
	}

	if t.Warmup > 0 {
		s.recordJob(ctx, sensorDeviceID, "warmup", fmt.Sprintf("Bật nguồn %s, chờ %d giây rồi đo.", label, int(t.Warmup.Seconds())))
		if err := sleepCtx(ctx, t.Warmup); err != nil {
			return err
		}
	}

	s.recordJob(ctx, sensorDeviceID, "measure", fmt.Sprintf("Đo %s.", label))
	return s.measureOne(ctx, action, t)
}

// Lượt có TDS thì tắt nguồn TDS trước để pH đọc khi que TDS không có điện; lượt chỉ đo pH thì bỏ qua.
func (s *Service) resetSensorBoard(ctx context.Context, want []string) error {
	if !contains(want, "tds") {
		return nil
	}
	if err := s.sensorPower(ctx, "all", false); err != nil {
		return fmt.Errorf("tắt nguồn cảm biến: %w", err)
	}
	return nil
}

// sensor "all" chỉ dùng để tắt.
func (s *Service) sensorPower(ctx context.Context, sensor string, on bool) error {
	ch := s.armSensor()
	maxOnS := 0
	if on {
		maxOnS = sensorMaxOnS
	}
	if err := s.pub.PublishSensorPower(ctx, phDeviceID, sensor, on, maxOnS); err != nil {
		return err
	}
	_, err := waitSensor(ctx, ch, phDeviceID, sensorPowerWaitLimit, func(sig sensorSig) bool {
		return sig.Stage == "power" && sig.Sensor == sensor && sig.Power == on
	})
	return err
}

func (s *Service) queryFloat(ctx context.Context) (bool, error) {
	ch := s.armWater()
	if err := s.pub.PublishLevel(ctx, waterDeviceID); err != nil {
		return false, err
	}
	sig, err := waitWater(ctx, ch, floatWaitLimit, func(sig waterSig) bool {
		return sig.Stage == "water_level"
	})
	if err != nil {
		return false, err
	}
	return sig.Full, nil
}

// Trả stage làm bơm dừng: filled/already_full (bơm nạp), time_up, hoặc lệnh tắt.
func (s *Service) runWater(ctx context.Context, target string, durationS int) (string, error) {
	ch := s.armWater()
	if err := s.pub.PublishPump(ctx, waterDeviceID, target, true, durationS); err != nil {
		return "", err
	}
	limit := time.Duration(durationS)*time.Second + waterWaitSlack
	sig, err := waitWater(ctx, ch, limit, func(sig waterSig) bool {
		switch sig.Stage {
		case "time_up", "pump_off", "drain_off", "stopped", "queue_cleared":
			return true
		case "filled", "already_full":
			return target == "inlet"
		}
		return false
	})
	if err != nil {
		return "", err
	}
	return sig.Stage, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) measureOne(ctx context.Context, action string, t sensorTiming) error {
	ch := s.armSensor()
	if err := s.pub.PublishRead(ctx, phDeviceID, action, t.Samples, t.IntervalMs); err != nil {
		return err
	}
	_, err := waitSensor(ctx, ch, phDeviceID, sensorWaitLimit, func(sig sensorSig) bool {
		return sig.Stage == "data" && contains(sig.Names, action)
	})
	if err != nil {
		return fmt.Errorf("đọc %s: %w", sensorLabel(action), err)
	}
	return nil
}

func waitWater(ctx context.Context, ch <-chan waterSig, limit time.Duration, accept func(waterSig) bool) (waterSig, error) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case sig := <-ch:
			if sig.Stage == "command_error" {
				return sig, fmt.Errorf("water %s", sig.Stage)
			}
			if accept(sig) {
				return sig, nil
			}
		case <-timer.C:
			return waterSig{}, fmt.Errorf("hết thời gian chờ water")
		case <-ctx.Done():
			return waterSig{}, ctx.Err()
		}
	}
}

// Chỉ xét tín hiệu từ đúng bo đang được hỏi.
func waitSensor(ctx context.Context, ch <-chan sensorSig, device string, limit time.Duration, accept func(sensorSig) bool) (sensorSig, error) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case sig := <-ch:
			if sig.Device != device {
				continue
			}
			if sig.Failed {
				msg := sig.Message
				if msg == "" {
					msg = "sensor lỗi"
				}
				return sig, fmt.Errorf("%s", msg)
			}
			if accept(sig) {
				return sig, nil
			}
		case <-timer.C:
			return sensorSig{}, fmt.Errorf("hết thời gian chờ bo %s", device)
		case <-ctx.Done():
			return sensorSig{}, ctx.Err()
		}
	}
}

func (s *Service) recordJob(ctx context.Context, pondID, stage, message string) {
	log.Printf("[MeasureJob] %s %s: %s", pondID, stage, message)
	e := &fish.SamplingEvent{
		ID:        uuid.New().String(),
		DeviceID:  pondID,
		Stage:     stage,
		State:     stage,
		Message:   message,
		CreatedAt: time.Now(),
	}
	if s.eventRepo != nil {
		if err := s.eventRepo.SaveEvent(ctx, e); err != nil {
			log.Printf("[MeasureJob] không lưu sự kiện: %v", err)
		}
	}
	if s.hub != nil {
		s.hub.Broadcast("sampling_event", e)
	}
}

func sensorLabel(action string) string {
	switch action {
	case "ph":
		return "pH"
	case "tds":
		return "TDS"
	default:
		return action
	}
}

// Thứ tự cố định pH rồi TDS: pH phải đọc khi que TDS chưa có điện.
// Nhiệt độ đo kèm pH trong cảm biến RS485, nên yêu cầu nhiệt độ được đọc bằng lệnh pH.
func normalizeSensors(sensors []string) []string {
	seen := map[string]bool{}
	add := func(name string) { seen[name] = true }
	for _, raw := range sensors {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "all", "tat_ca", "measure_all":
			add("ph")
			add("tds")
		case "ph", "temp", "temperature", "nhiet_do":
			add("ph")
		case "tds":
			add("tds")
		}
	}
	var out []string
	for _, name := range []string{"ph", "tds"} {
		if seen[name] {
			out = append(out, name)
		}
	}
	return out
}

func contains(list []string, name string) bool {
	for _, item := range list {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}
