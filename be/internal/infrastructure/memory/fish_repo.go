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
}

func NewStore() *Store {
	return &Store{
		measurements: make(map[string][]fish.Measurement),
		events:       make(map[string][]fish.SamplingEvent),
		devices:      make(map[string]*fish.Device),
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	list, ok := s.measurements[deviceID]
	if !ok || len(list) == 0 {
		return nil, nil
	}
	res := list[0]
	return &res, nil
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
