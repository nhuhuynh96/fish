package memory

import (
	"context"
	"sync"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

type Store struct {
	mu           sync.RWMutex
	measurements map[string][]fish.Measurement // deviceID -> list
	events       map[string][]fish.SamplingEvent
	devices      map[string]*fish.Device
	calibrations map[string]*fish.DeviceCalibration
}

func NewStore() *Store {
	return &Store{
		measurements: make(map[string][]fish.Measurement),
		events:       make(map[string][]fish.SamplingEvent),
		devices:      make(map[string]*fish.Device),
		calibrations: make(map[string]*fish.DeviceCalibration),
	}
}

// MeasurementRepository implementation
func (s *Store) SaveMeasurement(ctx context.Context, m *fish.Measurement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.measurements[m.DeviceID] = append([]fish.Measurement{*m}, s.measurements[m.DeviceID]...)
	if len(s.measurements[m.DeviceID]) > 100 {
		s.measurements[m.DeviceID] = s.measurements[m.DeviceID][:100]
	}
	return nil
}

func (s *Store) GetLatestMeasurement(ctx context.Context, deviceID string) (*fish.Measurement, error) {
	list, err := s.ListMeasurements(ctx, deviceID, 20)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}

	merged := list[0]
	sensors := make([]string, 0, 4)
	seen := map[string]bool{}

	for i := range list {
		m := &list[i]
		if m.Temperature != nil && merged.Temperature == nil {
			merged.Temperature = m.Temperature
		}
		if m.PH != nil && merged.PH == nil {
			merged.PH = m.PH
			merged.PHAdc = m.PHAdc
			merged.PHVoltage = m.PHVoltage
		}
		if m.TDS != nil && merged.TDS == nil {
			merged.TDS = m.TDS
			merged.TDSAdc = m.TDSAdc
			merged.TDSVoltage = m.TDSVoltage
		}
		if m.Turbidity != nil && merged.Turbidity == nil {
			merged.Turbidity = m.Turbidity
			merged.TurbidityAdc = m.TurbidityAdc
			merged.TurbidityVoltage = m.TurbidityVoltage
		}
		for _, name := range m.Sensors {
			if !seen[name] {
				seen[name] = true
				sensors = append(sensors, name)
			}
		}
	}
	merged.Sensors = sensors
	return &merged, nil
}

func (s *Store) ListMeasurements(ctx context.Context, deviceID string, limit int) ([]fish.Measurement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list, ok := s.measurements[deviceID]
	if !ok {
		return []fish.Measurement{}, nil
	}
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	res := make([]fish.Measurement, limit)
	copy(res, list[:limit])
	return res, nil
}

// EventRepository implementation
func (s *Store) SaveEvent(ctx context.Context, e *fish.SamplingEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[e.DeviceID] = append([]fish.SamplingEvent{*e}, s.events[e.DeviceID]...)
	if len(s.events[e.DeviceID]) > 100 {
		s.events[e.DeviceID] = s.events[e.DeviceID][:100]
	}
	return nil
}

func (s *Store) ListEvents(ctx context.Context, deviceID string, limit int) ([]fish.SamplingEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list, ok := s.events[deviceID]
	if !ok {
		return []fish.SamplingEvent{}, nil
	}
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	res := make([]fish.SamplingEvent, limit)
	copy(res, list[:limit])
	return res, nil
}

// DeviceRepository implementation
func (s *Store) UpsertDevice(ctx context.Context, d *fish.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	s.devices[d.ID] = &cp
	return nil
}

func (s *Store) GetDevice(ctx context.Context, deviceID string) (*fish.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[deviceID]
	if !ok {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}

func (s *Store) ListDevices(ctx context.Context) ([]fish.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]fish.Device, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, *d)
	}
	return list, nil
}

func (s *Store) GetCalibration(ctx context.Context, deviceID string) (*fish.DeviceCalibration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cal, ok := s.calibrations[deviceID]
	if !ok {
		return nil, nil
	}
	cp := *cal
	return &cp, nil
}

func (s *Store) SaveCalibration(ctx context.Context, cal *fish.DeviceCalibration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *cal
	s.calibrations[cal.DeviceID] = &cp
	return nil
}
