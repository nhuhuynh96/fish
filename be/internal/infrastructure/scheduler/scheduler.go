package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	fishuc "github.com/nhuhuynh/iot-fish/internal/usecase/fish"
)

type DeviceSchedule struct {
	AutoEnabled  bool
	TempInterval time.Duration
	PhInterval   time.Duration
	TurbInterval time.Duration
	TdsInterval  time.Duration

	lastTempRun time.Time
	lastPhRun   time.Time
	lastTurbRun time.Time
	lastTdsRun  time.Time
}

type Scheduler struct {
	svc       *fishuc.Service
	schedules map[string]*DeviceSchedule
	mu        sync.RWMutex
	stopChan  chan struct{}
}

func NewScheduler(svc *fishuc.Service) *Scheduler {
	return &Scheduler{
		svc:       svc,
		schedules: make(map[string]*DeviceSchedule),
		stopChan:  make(chan struct{}),
	}
}

// Cấu hình lịch cho 1 thiết bị
func (s *Scheduler) SetDeviceSchedule(deviceID string, auto bool, tempSec, phSec, turbSec, tdsSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tempSec <= 0 {
		tempSec = 60
	}
	if phSec <= 0 {
		phSec = 120
	}
	if turbSec <= 0 {
		turbSec = 180
	}
	if tdsSec <= 0 {
		tdsSec = 300
	}

	now := time.Now()
	s.schedules[deviceID] = &DeviceSchedule{
		AutoEnabled:  auto,
		TempInterval: time.Duration(tempSec) * time.Second,
		PhInterval:   time.Duration(phSec) * time.Second,
		TurbInterval: time.Duration(turbSec) * time.Second,
		TdsInterval:  time.Duration(tdsSec) * time.Second,
		lastTempRun:  now,
		lastPhRun:    now,
		lastTurbRun:  now,
		lastTdsRun:   now,
	}

	log.Printf("[Backend Scheduler] Đã thiết lập lịch đo cho [%s]: Auto=%v (Temp: %vs, pH: %vs, Turb: %vs, TDS: %vs)",
		deviceID, auto, tempSec, phSec, turbSec, tdsSec)
}

// Bật / tắt tự động cho thiết bị
func (s *Scheduler) ToggleAuto(deviceID string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sched, ok := s.schedules[deviceID]
	if !ok {
		// Tạo mặc định
		now := time.Now()
		sched = &DeviceSchedule{
			AutoEnabled:  enabled,
			TempInterval: 60 * time.Second,
			PhInterval:   120 * time.Second,
			TurbInterval: 180 * time.Second,
			TdsInterval:  300 * time.Second,
			lastTempRun:  now,
			lastPhRun:    now,
			lastTurbRun:  now,
			lastTdsRun:   now,
		}
		s.schedules[deviceID] = sched
	} else {
		sched.AutoEnabled = enabled
	}

	log.Printf("[Backend Scheduler] Đã đổi trạng thái Auto cho [%s] -> %v", deviceID, enabled)
}

// Khởi động vòng lặp kiểm tra lịch đo định kỳ
func (s *Scheduler) Start() {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		log.Println("[Backend Scheduler] Đã khởi động bộ lập lịch đo định kỳ (Cron/Ticker Worker)")
		for {
			select {
			case <-s.stopChan:
				ticker.Stop()
				log.Println("[Backend Scheduler] Đã dừng bộ lập lịch.")
				return
			case now := <-ticker.C:
				s.checkAndTrigger(now)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
}

func (s *Scheduler) checkAndTrigger(now time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()

	for deviceID, sched := range s.schedules {
		if !sched.AutoEnabled {
			continue
		}

		var dueSensors []string

		if sched.TempInterval > 0 && now.Sub(sched.lastTempRun) >= sched.TempInterval {
			dueSensors = append(dueSensors, "temp")
			sched.lastTempRun = now
		}
		if sched.PhInterval > 0 && now.Sub(sched.lastPhRun) >= sched.PhInterval {
			dueSensors = append(dueSensors, "ph")
			sched.lastPhRun = now
		}
		if sched.TurbInterval > 0 && now.Sub(sched.lastTurbRun) >= sched.TurbInterval {
			dueSensors = append(dueSensors, "turbidity")
			sched.lastTurbRun = now
		}
		if sched.TdsInterval > 0 && now.Sub(sched.lastTdsRun) >= sched.TdsInterval {
			dueSensors = append(dueSensors, "tds")
			sched.lastTdsRun = now
		}

		if len(dueSensors) > 0 {
			log.Printf("[Backend Scheduler] >>> Đến hạn đo tự động cho [%s]: %v -> Bắn MQTT Command xuống ESP32 <<<", deviceID, dueSensors)
			if err := s.svc.TriggerMeasurement(ctx, deviceID, dueSensors); err != nil {
				log.Printf("[Backend Scheduler] Lỗi khi bắn lệnh đo xuống [%s]: %v", deviceID, err)
			}
		}
	}
}
