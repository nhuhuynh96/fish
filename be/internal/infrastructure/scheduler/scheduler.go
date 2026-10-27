package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

type Measurer interface {
	TriggerMeasurement(ctx context.Context, deviceID string, sensors []string) error
	JobRunning() bool
}

var scheduledSensors = []string{"temp", "ph", "tds"}

type deviceState struct {
	cfg     fish.MeasureSchedule
	lastRun map[string]time.Time
}

func newState(cfg fish.MeasureSchedule, now time.Time) *deviceState {
	st := &deviceState{cfg: *cfg.Normalize(), lastRun: map[string]time.Time{}}
	for _, name := range scheduledSensors {
		st.lastRun[name] = now
	}
	return st
}

func (st *deviceState) interval(sensor string) int {
	switch sensor {
	case "temp":
		return st.cfg.TempInterval
	case "ph":
		return st.cfg.PhInterval
	case "tds":
		return st.cfg.TdsInterval
	}
	return fish.IntervalOff
}

// Lịch đo lưu trong bảng measure_schedule; bộ nhớ chỉ giữ thêm mốc lần đo trước.
type Scheduler struct {
	svc      Measurer
	repo     fish.ScheduleRepository
	devices  map[string]*deviceState
	mu       sync.Mutex
	stopChan chan struct{}
}

func NewScheduler(svc Measurer, repo fish.ScheduleRepository) *Scheduler {
	return &Scheduler{
		svc:      svc,
		repo:     repo,
		devices:  make(map[string]*deviceState),
		stopChan: make(chan struct{}),
	}
}

// Nạp lịch đã lưu. Mốc lần đo trước tính từ lúc khởi động.
func (s *Scheduler) Load(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	list, err := s.repo.ListSchedules(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cfg := range list {
		s.devices[cfg.DeviceID] = newState(cfg, now)
		log.Printf("[Backend Scheduler] Nạp lịch [%s]: Auto=%v temp=%s ph=%s tds=%s",
			cfg.DeviceID, cfg.AutoEnabled, describe(cfg.TempInterval), describe(cfg.PhInterval), describe(cfg.TdsInterval))
	}
	return nil
}

// Lưu lịch vào DB rồi áp dụng; mốc đếm bắt đầu lại từ lúc lưu.
func (s *Scheduler) SetDeviceSchedule(ctx context.Context, cfg fish.MeasureSchedule) (*fish.MeasureSchedule, error) {
	cfg.Normalize()
	cfg.UpdatedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repo != nil {
		if err := s.repo.SaveSchedule(ctx, &cfg); err != nil {
			return nil, err
		}
	}
	s.devices[cfg.DeviceID] = newState(cfg, cfg.UpdatedAt)
	log.Printf("[Backend Scheduler] Đã lưu lịch [%s]: Auto=%v temp=%s ph=%s tds=%s",
		cfg.DeviceID, cfg.AutoEnabled, describe(cfg.TempInterval), describe(cfg.PhInterval), describe(cfg.TdsInterval))
	out := cfg
	return &out, nil
}

// Bật lại auto thì đếm lại từ đầu, không đo bù những lần đã bỏ lỡ.
func (s *Scheduler) ToggleAuto(ctx context.Context, deviceID string, enabled bool) (*fish.MeasureSchedule, error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.devices[deviceID]
	if !ok {
		st = newState(*fish.DefaultMeasureSchedule(deviceID), now)
	}
	cfg := st.cfg
	cfg.AutoEnabled = enabled
	cfg.UpdatedAt = now
	if s.repo != nil {
		if err := s.repo.SaveSchedule(ctx, &cfg); err != nil {
			return nil, err
		}
	}
	st.cfg = cfg
	if enabled {
		for _, name := range scheduledSensors {
			st.lastRun[name] = now
		}
	}
	s.devices[deviceID] = st
	log.Printf("[Backend Scheduler] Auto [%s] -> %v", deviceID, enabled)
	out := cfg
	return &out, nil
}

// EnsureDevice thêm lịch mặc định (Auto tắt) vào bộ nhớ nếu device chưa có lịch; không ghi DB.
func (s *Scheduler) EnsureDevice(deviceID string) {
	if deviceID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[deviceID]; ok {
		return
	}
	s.devices[deviceID] = newState(*fish.DefaultMeasureSchedule(deviceID), time.Now())
}

func (s *Scheduler) GetDeviceSchedule(deviceID string) *fish.MeasureSchedule {
	s.EnsureDevice(deviceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.devices[deviceID].cfg
	return &cfg
}

func (s *Scheduler) Start() {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		log.Println("[Backend Scheduler] Đã khởi động bộ lập lịch đo định kỳ")
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

func isDue(interval int, last, now time.Time) bool {
	switch {
	case interval < 0:
		return false
	case interval == 0:
		return true
	default:
		return now.Sub(last) >= time.Duration(interval)*time.Second
	}
}

func describe(interval int) string {
	switch {
	case interval < 0:
		return "không đo"
	case interval == 0:
		return "liên tục"
	default:
		return (time.Duration(interval) * time.Second).String()
	}
}

func (s *Scheduler) checkAndTrigger(now time.Time) {
	// Đang có lượt đo thì chờ, chỉ số tới hạn vẫn giữ nguyên để chạy ở lượt kế.
	if s.svc.JobRunning() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	for deviceID, st := range s.devices {
		if !st.cfg.AutoEnabled {
			continue
		}
		var due []string
		for _, name := range scheduledSensors {
			if isDue(st.interval(name), st.lastRun[name], now) {
				due = append(due, name)
			}
		}
		if len(due) == 0 {
			continue
		}
		log.Printf("[Backend Scheduler] Đến hạn đo [%s]: %v", deviceID, due)
		if err := s.svc.TriggerMeasurement(ctx, deviceID, due); err != nil {
			log.Printf("[Backend Scheduler] Chưa đo [%s]: %v", deviceID, err)
			continue
		}
		for _, name := range due {
			st.lastRun[name] = now
		}
		return
	}
}
