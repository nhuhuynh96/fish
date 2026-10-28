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
	waterDeviceID         = "water"
	sensorDeviceID        = "sensor"
	floatWaitLimit        = 30 * time.Second
	waterWaitSlack        = 30 * time.Second
	sensorWaitLimit       = 60 * time.Second
	sensorPowerWaitLimit  = 10 * time.Second
	sensorMaxOnS          = 300
	fillDurationS         = 180
	drainToFloatDurationS = 120
	drainExtraS           = 20
)

// Đo pH ngay sau khi bơm cho kết quả cao hơn khoảng 0,5 do nước chưa ổn định.
var waterSettleDelay = 45 * time.Second

type sensorTiming struct {
	Warmup     time.Duration
	Samples    int
	IntervalMs int
}

// Mạch pH cần 10-30 giây sau khi cấp nguồn mới ổn định; đọc sau 1 giây mỗi lần lệch tới ±0,2 pH.
var sensorTimings = map[string]sensorTiming{
	"ph":   {Warmup: 30 * time.Second, Samples: 40, IntervalMs: 100},
	"temp": {Warmup: 1 * time.Second},
	"tds":  {Warmup: 5 * time.Second, Samples: 20, IntervalMs: 100},
}

type waterSig struct {
	Stage string
	Full  bool
}

type sensorSig struct {
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
	if id != sensorDeviceID && topicID != sensorDeviceID {
		return
	}
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

func (s *Service) runMeasure(_ string, sensors []string) {
	defer s.finishJob()
	ctx := context.Background()
	want := normalizeSensors(sensors)
	if len(want) == 0 {
		s.recordJob(ctx, sensorDeviceID, "job_failed", "Không có chỉ số đo được hỗ trợ.")
		return
	}
	log.Printf("[MeasureJob] đo %v: bơm tới khi phao đầy, đo, rồi xả", want)

	var measureErr error
	if err := s.sensorPower(ctx, "all", false); err != nil {
		measureErr = fmt.Errorf("tắt nguồn cảm biến: %w", err)
	}
	var settle time.Duration
	if measureErr == nil {
		pumped, err := s.fillToFloat(ctx)
		if err != nil {
			measureErr = fmt.Errorf("bơm nạp: %w", err)
		} else if pumped {
			settle = waterSettleDelay
		}
	}
	for _, action := range want {
		if measureErr != nil {
			break
		}
		// Chỉ sensor đầu chờ nước ổn định; sensor sau đã có thêm thời gian của sensor trước.
		measureErr = s.measureSensor(ctx, action, settle)
		settle = 0
	}

	if err := s.drainBin(ctx); err != nil && measureErr == nil {
		measureErr = fmt.Errorf("xả nước: %w", err)
	}
	if measureErr != nil {
		s.recordJob(ctx, sensorDeviceID, "job_failed", measureErr.Error())
		return
	}
	s.recordJob(ctx, sensorDeviceID, "job_done", "Đo xong.")
}

// Đo ngay từ dashboard: bật nguồn, chờ, đọc, tắt từng sensor; không bơm hay xả.
func (s *Service) runManualMeasure(sensors []string) {
	defer s.finishJob()
	ctx := context.Background()
	want := normalizeSensors(sensors)
	log.Printf("[MeasureJob] đo ngay %v, không chỉnh mực nước", want)

	var measureErr error
	if err := s.sensorPower(ctx, "all", false); err != nil {
		measureErr = fmt.Errorf("tắt nguồn cảm biến: %w", err)
	}
	for _, action := range want {
		if measureErr != nil {
			break
		}
		measureErr = s.measureSensor(ctx, action, 0)
	}
	if measureErr != nil {
		s.recordJob(ctx, sensorDeviceID, "job_failed", measureErr.Error())
		return
	}
	s.recordJob(ctx, sensorDeviceID, "job_done", "Đo xong.")
}

// Bật nguồn, chờ đủ lâu hơn giữa thời gian làm ấm và thời gian nước ổn định, đọc, rồi luôn tắt nguồn.
func (s *Service) measureSensor(ctx context.Context, action string, settle time.Duration) (err error) {
	t := sensorTimings[action]
	label := sensorLabel(action)
	// Tắt cả khi lệnh bật lỗi: mất phản hồi không có nghĩa là mạch chưa bật.
	defer func() {
		if offErr := s.sensorPower(context.Background(), action, false); offErr != nil && err == nil {
			err = fmt.Errorf("tắt nguồn %s: %w", label, offErr)
		}
	}()
	if err := s.sensorPower(ctx, action, true); err != nil {
		return fmt.Errorf("bật nguồn %s: %w", label, err)
	}

	wait := t.Warmup
	if settle > wait {
		wait = settle
	}
	if wait > 0 {
		msg := fmt.Sprintf("Bật nguồn %s, chờ %d giây rồi đo.", label, int(wait.Seconds()))
		if settle > 0 {
			msg = fmt.Sprintf("Bật nguồn %s, chờ %d giây cho nước ổn định và cảm biến làm ấm rồi đo.", label, int(wait.Seconds()))
		}
		s.recordJob(ctx, sensorDeviceID, "warmup", msg)
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
	}

	s.recordJob(ctx, sensorDeviceID, "measure", fmt.Sprintf("Đo %s.", label))
	return s.measureOne(ctx, action, t)
}

// sensor "all" chỉ dùng để tắt.
func (s *Service) sensorPower(ctx context.Context, sensor string, on bool) error {
	ch := s.armSensor()
	maxOnS := 0
	if on {
		maxOnS = sensorMaxOnS
	}
	if err := s.pub.PublishSensorPower(ctx, sensorDeviceID, sensor, on, maxOnS); err != nil {
		return err
	}
	_, err := waitSensor(ctx, ch, sensorPowerWaitLimit, func(sig sensorSig) bool {
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

// Trả stage làm bơm dừng: filled/drained/already_full/already_empty, time_up, hoặc lệnh tắt.
func (s *Service) runWater(ctx context.Context, target string, untilFloat bool, durationS int) (string, error) {
	ch := s.armWater()
	if err := s.pub.PublishPump(ctx, waterDeviceID, target, true, untilFloat, durationS); err != nil {
		return "", err
	}
	limit := time.Duration(durationS)*time.Second + waterWaitSlack
	sig, err := waitWater(ctx, ch, limit, func(sig waterSig) bool {
		switch sig.Stage {
		case "time_up", "pump_off", "drain_off", "stopped", "queue_cleared":
			return true
		case "filled", "already_full":
			return target == "inlet"
		case "drained", "already_empty":
			return target == "drain"
		}
		return false
	})
	if err != nil {
		return "", err
	}
	return sig.Stage, nil
}

// pumped = vừa bơm, nước cần thời gian ổn định trước khi đo.
func (s *Service) fillToFloat(ctx context.Context) (pumped bool, err error) {
	full, err := s.queryFloat(ctx)
	if err != nil {
		return false, err
	}
	if full {
		log.Printf("[MeasureJob] phao đã đầy, không bơm")
		return false, nil
	}
	s.recordJob(ctx, waterDeviceID, "fill", fmt.Sprintf("Bơm nạp tới khi phao đầy, tối đa %d giây.", fillDurationS))
	stage, err := s.runWater(ctx, "inlet", true, fillDurationS)
	if err != nil {
		return true, err
	}
	switch stage {
	case "filled", "already_full":
		return true, nil
	case "time_up":
		return true, fmt.Errorf("phao chưa đầy sau %d giây", fillDurationS)
	default:
		return true, fmt.Errorf("bơm nạp bị tắt giữa chừng (%s)", stage)
	}
}

// Xả tới khi phao tắt, rồi xả thêm drainExtraS cho cạn. Phao không tắt kịp chỉ ghi cảnh báo, vẫn xả thêm.
func (s *Service) drainBin(ctx context.Context) error {
	s.recordJob(ctx, waterDeviceID, "final_drain",
		fmt.Sprintf("Xả tới khi phao tắt (tối đa %d giây), rồi xả thêm %d giây.", drainToFloatDurationS, drainExtraS))
	stage, err := s.runWater(ctx, "drain", true, drainToFloatDurationS)
	if err != nil {
		return err
	}
	switch stage {
	case "drained", "already_empty":
	case "time_up":
		s.recordJob(ctx, waterDeviceID, "drain_warning",
			fmt.Sprintf("Xả %d giây mà phao chưa tắt. Kiểm tra phao hoặc bơm xả.", drainToFloatDurationS))
	default:
		return fmt.Errorf("bơm xả bị tắt giữa chừng (%s)", stage)
	}
	stage, err = s.runWater(ctx, "drain", false, drainExtraS)
	if err != nil {
		return err
	}
	if stage != "time_up" {
		return fmt.Errorf("bơm xả bị tắt giữa chừng (%s)", stage)
	}
	return nil
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
	if err := s.pub.PublishRead(ctx, sensorDeviceID, action, t.Samples, t.IntervalMs); err != nil {
		return err
	}
	want := action
	if action == "temp" {
		want = "temperature"
	}
	_, err := waitSensor(ctx, ch, sensorWaitLimit, func(sig sensorSig) bool {
		return sig.Stage == "data" && contains(sig.Names, want)
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

func waitSensor(ctx context.Context, ch <-chan sensorSig, limit time.Duration, accept func(sensorSig) bool) (sensorSig, error) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case sig := <-ch:
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
			return sensorSig{}, fmt.Errorf("hết thời gian chờ sensor")
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
	case "temp":
		return "nhiệt độ"
	case "tds":
		return "TDS"
	default:
		return action
	}
}

// Thứ tự cố định pH, temp, TDS.
// pH phải đo trước nhiệt độ: đo ngay sau DS18B20 làm pH cao hơn khoảng 0,5.
func normalizeSensors(sensors []string) []string {
	seen := map[string]bool{}
	add := func(name string) { seen[name] = true }
	for _, raw := range sensors {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "all", "tat_ca", "measure_all":
			add("temp")
			add("ph")
			add("tds")
		case "ph":
			add("ph")
		case "temp", "temperature", "nhiet_do":
			add("temp")
		case "tds":
			add("tds")
		}
	}
	var out []string
	for _, name := range []string{"ph", "temp", "tds"} {
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
