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

type MeasureSchedule struct {
	DeviceID     string    `json:"device_id"`
	AutoEnabled  bool      `json:"auto_enabled"`
	TempInterval int       `json:"temp_interval"`
	PhInterval   int       `json:"ph_interval"`
	TurbInterval int       `json:"turb_interval"`
	TdsInterval  int       `json:"tds_interval"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func DefaultMeasureSchedule(deviceID string) *MeasureSchedule {
	return &MeasureSchedule{
		DeviceID:     deviceID,
		AutoEnabled:  false,
		TempInterval: 1800,
		PhInterval:   1800,
		TurbInterval: IntervalOff,
		TdsInterval:  3600,
		UpdatedAt:    time.Now(),
	}
}

func NormalizeInterval(v int) int {
	if v < 0 {
		return IntervalOff
	}
	return v
}

func (s *MeasureSchedule) Normalize() *MeasureSchedule {
	s.TempInterval = NormalizeInterval(s.TempInterval)
	s.PhInterval = NormalizeInterval(s.PhInterval)
	s.TurbInterval = NormalizeInterval(s.TurbInterval)
	s.TdsInterval = NormalizeInterval(s.TdsInterval)
	return s
}

type ScheduleRepository interface {
	GetSchedule(ctx context.Context, deviceID string) (*MeasureSchedule, error)
	SaveSchedule(ctx context.Context, s *MeasureSchedule) error
	ListSchedules(ctx context.Context) ([]MeasureSchedule, error)
}
