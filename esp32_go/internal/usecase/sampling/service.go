package samplinguc

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nhuhuynh/iot-fish/esp32_go/internal/domain/sampling"
)

type Service struct {
	mu       sync.Mutex
	deviceID string
	hw       sampling.Hardware
	bus      sampling.Bus
	queue    []sampling.Command

	fillLevel    sampling.FillLevel
	inletSince   time.Time
	inletAttempt int
	inletFailed  bool

	drainSince        time.Time
	drainFixedActive  bool
	drainToLevel1     bool
	skipDrainToLevel1 bool

	floatSince time.Time
}

func New(deviceID string, hw sampling.Hardware, bus sampling.Bus) *Service {
	return &Service{deviceID: deviceID, hw: hw, bus: bus}
}

func (s *Service) SetBus(bus sampling.Bus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bus = bus
}

func (s *Service) Begin() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = nil
	s.stopHardware()
	log.Println("[Sampling] Queue: action + fillLevel. Phao 0=mức1, 1=mức2.")
}

func (s *Service) StateName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateNameLocked()
}

func (s *Service) stateNameLocked() string {
	if s.hw.InletOn() {
		return "FILLING_WATER"
	}
	if s.hw.DrainOn() {
		return "DRAINING_WATER"
	}
	if len(s.queue) > 0 {
		return "BUSY"
	}
	return "IDLE"
}

func (s *Service) QueueLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

func (s *Service) Tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlePump(now)
	s.processHead(now)
}

func (s *Service) Enqueue(cmd sampling.Command) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd.Action = sampling.ActionType(strings.ToLower(strings.TrimSpace(string(cmd.Action))))
	cmd.FillLevel = sampling.NormalizeFillLevel(cmd.FillLevel)
	if cmd.Action == sampling.ActionClearQueue || cmd.Action == "clear" || cmd.Action == "abort" {
		s.clearLocked()
		return
	}
	s.queue = append(s.queue, cmd)
	s.emit("queued", fmt.Sprintf("Đã xếp %s fillLevel=%d", cmd.Action, cmd.FillLevel), nil)
}

func (s *Service) ClearQueue() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
}

func (s *Service) QueueSnapshot() []sampling.Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sampling.Command, len(s.queue))
	copy(out, s.queue)
	return out
}

func (s *Service) clearLocked() {
	n := len(s.queue)
	s.queue = nil
	s.inletAttempt = 0
	s.inletFailed = false
	s.drainFixedActive = false
	s.drainToLevel1 = false
	s.skipDrainToLevel1 = false
	s.hasFloatTarget = false
	s.stopHardware()
	s.emit("queue_cleared", fmt.Sprintf("Đã xóa toàn bộ hàng đợi (%d lệnh).", n), nil)
}

func (s *Service) stopHardware() {
	s.hw.SetInlet(false)
	s.hw.SetDrain(false)
	s.hw.SetSensorPower(sampling.SensorPH, false)
	s.hw.SetSensorPower(sampling.SensorTurbidity, false)
	s.hw.SetSensorPower(sampling.SensorTDS, false)
}

func (s *Service) pop() {
	if len(s.queue) == 0 {
		return
	}
	s.queue = s.queue[1:]
	s.inletAttempt = 0
	s.inletFailed = false
	s.hasFloatTarget = false
	s.skipDrainToLevel1 = false
}

type fillResult int

const (
	fillBusy fillResult = iota
	fillDone
	fillAbort
)

func (s *Service) fillStopPump(level sampling.FillLevel) bool {
	if level == sampling.FillLevelHigh {
		return s.hw.FloatHigh()
	}
	return s.hw.FloatLow()
}

func (s *Service) level1Ready() bool {
	return s.hw.FloatLow() && !s.hw.FloatHigh()
}

func (s *Service) atLevel2() bool {
	return s.hw.FloatHigh() && s.hw.FloatLow()
}

func (s *Service) processHead(now time.Time) {
	if len(s.queue) == 0 {
		return
	}
	cmd := s.queue[0]
	switch cmd.Action {
	case sampling.ActionInlet:
		if s.ensureFill(cmd.FillLevel, now) != fillBusy {
			s.pop()
		}
	case sampling.ActionDrain:
		if s.ensureDrainFixed(now) {
			s.pop()
		}
	case sampling.ActionPH:
		switch s.ensureLevel1(now) {
		case fillDone:
			s.measureSensor(sampling.SensorPH)
			s.pop()
		case fillAbort:
			s.pop()
		}
	case sampling.ActionTurbidity:
		switch s.ensureLevel1(now) {
		case fillDone:
			s.measureSensor(sampling.SensorTurbidity)
			s.pop()
		case fillAbort:
			s.pop()
		}
	case sampling.ActionTDS:
		switch s.ensureFill(sampling.FillLevelHigh, now) {
		case fillDone:
			s.measureSensor(sampling.SensorTDS)
			s.pop()
		case fillAbort:
			s.pop()
		}
	case sampling.ActionStatus:
		s.publishStatus()
		s.pop()
	case sampling.ActionStop:
		s.stopHardware()
		s.drainFixedActive = false
		s.drainToLevel1 = false
		s.skipDrainToLevel1 = false
		s.emit("manual_pump", "Đã dừng bơm/van.", nil)
		s.pop()
	default:
		s.emit("command_error", "action không hỗ trợ: "+string(cmd.Action), nil)
		s.pop()
	}
}

func (s *Service) ensureFill(level sampling.FillLevel, now time.Time) fillResult {
	s.fillLevel = sampling.NormalizeFillLevel(level)
	if s.inletFailed {
		s.emit("manual_pump_timeout", "Bơm 3 lần 30s chưa tới "+s.fillLevel.Name()+". Bỏ lệnh.", nil)
		s.inletFailed = false
		s.inletAttempt = 0
		return fillAbort
	}
	ready := s.level1Ready()
	if s.fillLevel == sampling.FillLevelHigh {
		ready = s.hw.FloatHigh()
	}
	if ready && !s.hw.InletOn() {
		return fillDone
	}
	if s.fillStopPump(s.fillLevel) {
		return fillBusy
	}
	if !s.hw.InletOn() {
		s.startInlet(now)
	}
	return fillBusy
}

func (s *Service) ensureLevel1(now time.Time) fillResult {
	if s.skipDrainToLevel1 && s.hw.FloatLow() && !s.hw.InletOn() {
		return fillDone
	}
	if !s.skipDrainToLevel1 && s.atLevel2() {
		s.drainToLevel1 = true
		s.drainFixedActive = false
		if !s.hw.DrainOn() {
			s.hw.SetDrain(true)
			s.drainSince = now
			s.emit("draining", "Đang đầy phao 2. Xả xuống mức 1.", nil)
		}
		return fillBusy
	}
	if s.drainToLevel1 {
		if s.hw.DrainOn() {
			s.hw.SetDrain(false)
		}
		s.drainToLevel1 = false
		s.emit("drained", "Phao 2 hết đầy. Đã xuống mức 1.", nil)
	}
	if s.level1Ready() && !s.hw.InletOn() {
		return fillDone
	}
	return s.ensureFill(sampling.FillLevelLow, now)
}

func (s *Service) ensureDrainFixed(now time.Time) bool {
	s.drainToLevel1 = false
	if !s.drainFixedActive {
		if s.hw.DrainOn() {
			return false
		}
		s.hw.SetDrain(true)
		s.drainSince = now
		s.drainFixedActive = true
		s.emit("manual_pump", "Van xả nước: BẬT", nil)
		return false
	}
	if !s.hw.DrainOn() {
		s.drainFixedActive = false
		return true
	}
	return false
}

func (s *Service) startInlet(now time.Time) {
	s.hw.SetInlet(true)
	s.inletSince = now
	s.hasFloatTarget = false
	if s.inletAttempt == 0 {
		s.inletAttempt = 1
	}
	s.emit("manual_pump", "Bơm nạp nước: BẬT ("+s.fillLevel.Name()+")", nil)
}

func (s *Service) handlePump(now time.Time) {
	if s.hw.InletOn() {
		onFor := now.Sub(s.inletSince)
		if onFor < sampling.PumpFloatGrace {
			s.hasFloatTarget = false
		} else if s.fillStopPump(s.fillLevel) {
			if !s.hasFloatTarget {
				s.floatSince = now
				s.hasFloatTarget = true
			}
			if now.Sub(s.floatSince) >= sampling.FloatDebounce {
				s.hw.SetInlet(false)
				s.inletAttempt = 0
				s.hasFloatTarget = false
				s.emit("manual_pump", "Đủ "+s.fillLevel.Name()+". Đã tự tắt bơm nạp.", nil)
			}
		} else {
			s.hasFloatTarget = false
		}
		if s.hw.InletOn() && onFor >= sampling.ManualPumpTime {
			if s.inletAttempt < sampling.MaxManualAttempts {
				s.inletAttempt++
				s.hw.SetInlet(false)
				s.hw.SetInlet(true)
				s.inletSince = now
				s.hasFloatTarget = false
				s.emit("manual_pump", fmt.Sprintf("Chưa đủ mực. Thử bơm lại lần %d/%d.", s.inletAttempt, sampling.MaxManualAttempts), nil)
			} else {
				s.hw.SetInlet(false)
				s.inletAttempt = 0
				s.inletFailed = true
				s.emit("manual_pump_timeout", "Bơm 3 lần 30s chưa tới mực. Đã ngưng.", nil)
			}
		}
	}

	if !s.hw.DrainOn() {
		return
	}
	if s.drainToLevel1 {
		if !s.hw.FloatHigh() {
			s.hw.SetDrain(false)
			s.drainToLevel1 = false
			s.emit("drained", "Phao 2 hết đầy. Tắt van xả.", nil)
		} else if now.Sub(s.drainSince) >= sampling.DrainFixedTime {
			s.hw.SetDrain(false)
			s.drainToLevel1 = false
			s.skipDrainToLevel1 = true
			s.emit("drain_timeout", "Xả xuống mức 1 quá 30s. Tiếp tục bơm/đo.", nil)
		}
		return
	}
	if s.drainFixedActive && now.Sub(s.drainSince) >= sampling.DrainFixedTime {
		s.hw.SetDrain(false)
		s.emit("manual_pump_timeout", "Van xả đủ 30s. Đã tự tắt.", nil)
	}
}

func (s *Service) measureSensor(t sampling.SensorType) {
	if t == sampling.SensorTemp {
		s.emit("measuring_sensor", "Bỏ qua nhiệt độ (chưa có sensor phần cứng).", nil)
		return
	}
	s.hw.SetSensorPower(t, true)
	raw := s.hw.ReadADC(t)
	s.hw.SetSensorPower(t, false)
	s.publish(t, raw)
}

func (s *Service) publish(t sampling.SensorType, raw sampling.RawReading) {
	body := map[string]any{
		"device_id":        s.deviceID,
		"timestamp":        time.Now().Unix(),
		"status":           "success",
		"sensors_measured": []string{t.Name()},
		"raw":              map[string]any{t.Name(): raw},
	}
	payload, _ := json.Marshal(body)
	log.Printf("[Sampling] GỬI sensor_data %s", payload)
	if s.bus != nil && s.bus.Connected() {
		_ = s.bus.Publish("sensor_data", payload, false)
	}
	s.emit("measuring_sensor", "Đã đo "+t.Name()+" raw, đã gửi MQTT.", map[string]any{
		"sensor":  t.Name(),
		"adc":     raw.ADC,
		"voltage": raw.Voltage,
	})
}

func (s *Service) publishStatus() {
	body := map[string]any{
		"device_id":  s.deviceID,
		"state":      s.stateNameLocked(),
		"is_busy":    len(s.queue) > 0 || s.hw.InletOn() || s.hw.DrainOn(),
		"queue_size": len(s.queue),
		"float_low":  s.hw.FloatLow(),
		"float_high": s.hw.FloatHigh(),
	}
	payload, _ := json.Marshal(body)
	if s.bus != nil && s.bus.Connected() {
		_ = s.bus.Publish("status", payload, false)
	}
}

func (s *Service) emit(stage, message string, extra map[string]any) {
	log.Printf("[Sampling Event] [%s] %s", stage, message)
	body := map[string]any{
		"device_id":   s.deviceID,
		"stage":       stage,
		"state":       s.stateNameLocked(),
		"message":     message,
		"inlet_on":    s.hw.InletOn(),
		"drain_on":    s.hw.DrainOn(),
		"float_low":   s.hw.FloatLow(),
		"float_high":  s.hw.FloatHigh(),
		"float_full":  s.hw.FloatHigh(),
		"float_empty": s.hw.FloatLow(),
	}
	for k, v := range extra {
		body[k] = v
	}
	payload, _ := json.Marshal(body)
	if s.bus != nil && s.bus.Connected() {
		_ = s.bus.Publish("event", payload, false)
	}
}
