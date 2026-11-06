package fish

import (
	"context"
	"time"
)

// Khoảng đo tính bằng giây: -1 không đo, 0 đo liên tục (xong lượt này chạy lượt kế), > 0 chờ đủ số giây.
const (
	IntervalOff        = -1
	IntervalContinuous = 0
)

// Thời gian bơm, xả, chờ ổn định của một lượt đo tự động (giây).
const (
	DefaultFillMaxS = 120
	DefaultDrainS   = 120
	DefaultSettleS  = 45
	MaxPumpS        = 300
	MaxSettleS      = 600
)

type MeasureSchedule struct {
	DeviceID     string `json:"device_id"`
	AutoEnabled  bool   `json:"auto_enabled"`
	TempInterval int    `json:"temp_interval"`
	PhInterval   int    `json:"ph_interval"`
	TurbInterval int    `json:"turb_interval"`
	TdsInterval  int    `json:"tds_interval"`
	// Bơm nạp dừng khi phao đầy hoặc hết FillMaxS; xả chạy đủ DrainS; chờ SettleS rồi đo.
	FillMaxS  int       `json:"fill_max_s"`
	DrainS    int       `json:"drain_s"`
	SettleS   int       `json:"settle_s"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CycleTiming là phần thời gian lượt đo cần, tách khỏi lịch để truyền cho usecase.
type CycleTiming struct {
	FillMaxS int
	DrainS   int
	SettleS  int
}

func DefaultCycleTiming() CycleTiming {
	return CycleTiming{FillMaxS: DefaultFillMaxS, DrainS: DefaultDrainS, SettleS: DefaultSettleS}
}

func DefaultMeasureSchedule(deviceID string) *MeasureSchedule {
	return &MeasureSchedule{
		DeviceID:     deviceID,
		AutoEnabled:  false,
		TempInterval: 1800,
		PhInterval:   1800,
		TurbInterval: IntervalOff,
		TdsInterval:  3600,
		FillMaxS:     DefaultFillMaxS,
		DrainS:       DefaultDrainS,
		SettleS:      DefaultSettleS,
		UpdatedAt:    time.Now(),
	}
}

func NormalizeInterval(v int) int {
	if v < 0 {
		return IntervalOff
	}
	return v
}

func clampSeconds(v, fallback, max int) int {
	if v <= 0 {
		return fallback
	}
	if v > max {
		return max
	}
	return v
}

func (s *MeasureSchedule) Normalize() *MeasureSchedule {
	s.TempInterval = NormalizeInterval(s.TempInterval)
	s.PhInterval = NormalizeInterval(s.PhInterval)
	s.TurbInterval = NormalizeInterval(s.TurbInterval)
	s.TdsInterval = NormalizeInterval(s.TdsInterval)
	s.FillMaxS = clampSeconds(s.FillMaxS, DefaultFillMaxS, MaxPumpS)
	s.DrainS = clampSeconds(s.DrainS, DefaultDrainS, MaxPumpS)
	if s.SettleS < 0 {
		s.SettleS = DefaultSettleS
	}
	if s.SettleS > MaxSettleS {
		s.SettleS = MaxSettleS
	}
	return s
}

func (s *MeasureSchedule) Timing() CycleTiming {
	n := *s
	n.Normalize()
	return CycleTiming{FillMaxS: n.FillMaxS, DrainS: n.DrainS, SettleS: n.SettleS}
}

type ScheduleRepository interface {
	GetSchedule(ctx context.Context, deviceID string) (*MeasureSchedule, error)
	SaveSchedule(ctx context.Context, s *MeasureSchedule) error
	ListSchedules(ctx context.Context) ([]MeasureSchedule, error)
}
