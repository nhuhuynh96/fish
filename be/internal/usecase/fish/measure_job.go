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
	levelWaitLimit        = 30 * time.Second
	waterWaitSlack        = 30 * time.Second
	sensorWaitLimit       = 60 * time.Second
	fillDurationS         = 180
	drainToLevelDurationS = 120
	finalDrainDurationS   = 60
)

type waterSig struct {
	Stage string
	Level int
}

type sensorSig struct {
	Names   []string
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
	ch := make(chan sensorSig, 4)
	s.jobMu.Lock()
	s.sensorWait = ch
	s.jobMu.Unlock()
	return ch
}

func (s *Service) noteWater(topicID, bodyID, stage string, level int, message string) {
	id := bodyID
	if id == "" {
		id = topicID
	}
	if id == sensorDeviceID && stage == "sensor_error" {
		s.noteSensor(topicID, bodyID, nil, true, message)
		return
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
	case ch <- waterSig{Stage: stage, Level: level}:
	default:
	}
}

func (s *Service) noteSensor(topicID, bodyID string, names []string, failed bool, message string) {
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
	case ch <- sensorSig{Names: names, Failed: failed, Message: message}:
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
	log.Printf("[MeasureJob] đo %v: chỉnh mực theo từng sensor, đo, rồi xả %ds", want, finalDrainDurationS)

	var measureErr error
	for _, action := range want {
		if err := s.ensureLevel(ctx, sensorFillLevel(action)); err != nil {
			measureErr = fmt.Errorf("%s: %w", sensorLabel(action), err)
			break
		}
		s.recordJob(ctx, sensorDeviceID, "measure", fmt.Sprintf("Đo %s.", sensorLabel(action)))
		if err := s.measureOne(ctx, action); err != nil {
			measureErr = err
			break
		}
	}

	s.recordJob(ctx, waterDeviceID, "final_drain", fmt.Sprintf("Xả nước %d giây sau khi đo.", finalDrainDurationS))
	if err := s.runWater(ctx, "drain", 0, finalDrainDurationS); err != nil && measureErr == nil {
		measureErr = fmt.Errorf("xả nước: %w", err)
	}
	if measureErr != nil {
		s.recordJob(ctx, sensorDeviceID, "job_failed", measureErr.Error())
		return
	}
	s.recordJob(ctx, sensorDeviceID, "job_done", "Đo xong.")
}

func (s *Service) queryLevel(ctx context.Context) (int, error) {
	ch := s.armWater()
	if err := s.pub.PublishLevel(ctx, waterDeviceID); err != nil {
		return 0, err
	}
	sig, err := waitWater(ctx, ch, levelWaitLimit, func(sig waterSig) bool {
		return sig.Stage == "water_level"
	})
	if err != nil {
		return 0, err
	}
	return sig.Level, nil
}

// level 0: chạy đủ duration, chờ time_up. level 1/2: chờ tới phao, time_up là lỗi.
func (s *Service) runWater(ctx context.Context, target string, level, durationS int) error {
	ch := s.armWater()
	if err := s.pub.PublishPump(ctx, waterDeviceID, target, true, level, durationS); err != nil {
		return err
	}
	limit := time.Duration(durationS)*time.Second + waterWaitSlack
	sig, err := waitWater(ctx, ch, limit, func(sig waterSig) bool {
		switch sig.Stage {
		case "time_up", "already_at_level", "pump_off", "drain_off", "queue_cleared":
			return true
		case "filled":
			return target == "inlet"
		case "drained":
			return target == "drain"
		}
		return false
	})
	if err != nil {
		return err
	}
	switch sig.Stage {
	case "pump_off", "drain_off", "queue_cleared":
		if level > 0 {
			return fmt.Errorf("bơm bị tắt giữa chừng (%s)", sig.Stage)
		}
	case "time_up":
		if level > 0 {
			return fmt.Errorf("không tới phao %d trong %d giây", level, durationS)
		}
	}
	return nil
}

func (s *Service) ensureLevel(ctx context.Context, target int) error {
	if target != 1 && target != 2 {
		return fmt.Errorf("mức nước %d không hỗ trợ", target)
	}
	level, err := s.queryLevel(ctx)
	if err != nil {
		return err
	}
	if target == 1 && level == 2 {
		s.recordJob(ctx, waterDeviceID, "drain_level", "Mực ở phao 2. Xả tới khi phao 2 tắt.")
		if err := s.runWater(ctx, "drain", 2, drainToLevelDurationS); err != nil {
			return fmt.Errorf("xả nước: %w", err)
		}
		if level, err = s.queryLevel(ctx); err != nil {
			return err
		}
		if level == 2 {
			return fmt.Errorf("xả xong vẫn ở phao 2")
		}
	}
	if level >= target {
		log.Printf("[MeasureJob] nước đã ở phao %d", level)
		return nil
	}
	s.recordJob(ctx, waterDeviceID, "fill", fmt.Sprintf("Bơm nạp tới phao %d.", target))
	if err := s.runWater(ctx, "inlet", target, fillDurationS); err != nil {
		return fmt.Errorf("bơm nạp: %w", err)
	}
	return nil
}

func (s *Service) measureOne(ctx context.Context, action string) error {
	ch := s.armSensor()
	if err := s.pub.PublishMeasure(ctx, sensorDeviceID, []string{action}); err != nil {
		return err
	}
	want := action
	if action == "temp" {
		want = "temperature"
	}
	_, err := waitSensor(ctx, ch, want)
	return err
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

func waitSensor(ctx context.Context, ch <-chan sensorSig, want string) (sensorSig, error) {
	timer := time.NewTimer(sensorWaitLimit)
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
			if contains(sig.Names, want) {
				return sig, nil
			}
		case <-timer.C:
			return sensorSig{}, fmt.Errorf("hết thời gian chờ sensor %s", want)
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

func sensorFillLevel(action string) int {
	if action == "tds" {
		return 2
	}
	return 1
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

// Thứ tự cố định temp, pH, TDS: nhóm phao 1 đo trước, TDS phao 2 đo sau.
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
	for _, name := range []string{"temp", "ph", "tds"} {
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
