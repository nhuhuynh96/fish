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
	waterDeviceID   = "water"
	sensorDeviceID  = "sensor"
	waterWaitLimit  = 4 * time.Minute
	sensorWaitLimit = 60 * time.Second
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

func (s *Service) runMeasure(pondID string, sensors []string) {
	defer s.finishJob()
	ctx := context.Background()
	want := normalizeSensors(sensors)
	if len(want) == 0 {
		s.recordJob(ctx, pondID, "job_failed", "Không có chỉ số đo được hỗ trợ.")
		return
	}
	log.Printf("[MeasureJob] hồ %s đo %v", pondID, want)

	if contains(want, "temp") || contains(want, "ph") {
		if err := s.ensureLevel(ctx, 1); err != nil {
			s.recordJob(ctx, pondID, "job_failed", err.Error())
			return
		}
		if contains(want, "temp") {
			if err := s.measureOne(ctx, "temp"); err != nil {
				s.recordJob(ctx, pondID, "job_failed", err.Error())
				return
			}
		}
		if contains(want, "ph") {
			if err := s.measureOne(ctx, "ph"); err != nil {
				s.recordJob(ctx, pondID, "job_failed", err.Error())
				return
			}
		}
	}
	if contains(want, "tds") {
		if err := s.ensureLevel(ctx, 2); err != nil {
			s.recordJob(ctx, pondID, "job_failed", err.Error())
			return
		}
		if err := s.measureOne(ctx, "tds"); err != nil {
			s.recordJob(ctx, pondID, "job_failed", err.Error())
			return
		}
	}
	s.recordJob(ctx, pondID, "job_done", "Đo xong.")
}

func (s *Service) ensureLevel(ctx context.Context, target int) error {
	ch := s.armWater()
	if err := s.pub.PublishLevel(ctx, waterDeviceID); err != nil {
		return err
	}
	sig, err := waitWater(ctx, ch, func(sig waterSig) bool {
		return sig.Stage == "water_level"
	})
	if err != nil {
		return err
	}
	if sig.Level == target {
		log.Printf("[MeasureJob] nước đã ở level %d", target)
		return nil
	}
	if sig.Level > target {
		ch = s.armWater()
		if err := s.pub.PublishPump(ctx, waterDeviceID, "drain", true, 0); err != nil {
			return err
		}
		if _, err := waitWater(ctx, ch, func(sig waterSig) bool {
			return sig.Stage == "drained" && sig.Level == 0
		}); err != nil {
			return err
		}
	}
	ch = s.armWater()
	if err := s.pub.PublishPump(ctx, waterDeviceID, "inlet", true, target); err != nil {
		return err
	}
	_, err = waitWater(ctx, ch, func(sig waterSig) bool {
		if sig.Stage == "already_at_level" || sig.Stage == "filled" {
			return sig.Level == target
		}
		return false
	})
	return err
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

func waitWater(ctx context.Context, ch <-chan waterSig, accept func(waterSig) bool) (waterSig, error) {
	timer := time.NewTimer(waterWaitLimit)
	defer timer.Stop()
	for {
		select {
		case sig := <-ch:
			if sig.Stage == "fill_timeout" || sig.Stage == "drain_timeout" || sig.Stage == "command_error" {
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

func normalizeSensors(sensors []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
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
