package fish

import (
	"context"
	"time"
)

type Measurement struct {
	ID               string    `json:"id"`
	DeviceID         string    `json:"device_id"`
	Timestamp        int64     `json:"timestamp"`
	Status           string    `json:"status"`
	DurationMs       int64     `json:"duration_ms"`
	Sensors          []string  `json:"sensors_measured"`
	Temperature      *float64  `json:"temperature,omitempty"`
	PH               *float64  `json:"ph,omitempty"`
	Turbidity        *float64  `json:"turbidity,omitempty"`
	TDS              *float64  `json:"tds,omitempty"`
	PHAdc            *int      `json:"ph_adc,omitempty"`
	PHVoltage        *float64  `json:"ph_voltage,omitempty"`
	TDSAdc           *int      `json:"tds_adc,omitempty"`
	TDSVoltage       *float64  `json:"tds_voltage,omitempty"`
	TurbidityAdc     *int      `json:"turbidity_adc,omitempty"`
	TurbidityVoltage *float64  `json:"turbidity_voltage,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type SamplingEvent struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	Stage      string    `json:"stage"`
	State      string    `json:"state"`
	Message    string    `json:"message"`
	InletOn    *bool     `json:"inlet_on,omitempty"`
	DrainOn    *bool     `json:"drain_on,omitempty"`
	FloatFull  *bool     `json:"float_full,omitempty"`
	FloatEmpty *bool     `json:"float_empty,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Device struct {
	ID       string    `json:"id"`
	Online   bool      `json:"online"`
	IP       string    `json:"ip"`
	State    string    `json:"state"`
	LastSeen time.Time `json:"last_seen"`
	RSSI     int       `json:"rssi,omitempty"`
	Uptime   int64     `json:"uptime,omitempty"`
}

type MeasurementRepository interface {
	SaveMeasurement(ctx context.Context, m *Measurement) error
	GetLatestMeasurement(ctx context.Context, deviceID string) (*Measurement, error)
	ListMeasurements(ctx context.Context, deviceID string, limit int) ([]Measurement, error)
	ListMeasurementsSince(ctx context.Context, deviceID string, since time.Time, limit int) ([]Measurement, error)
}

type EventRepository interface {
	SaveEvent(ctx context.Context, e *SamplingEvent) error
	ListEvents(ctx context.Context, deviceID string, limit int) ([]SamplingEvent, error)
}

type DeviceRepository interface {
	UpsertDevice(ctx context.Context, d *Device) error
	GetDevice(ctx context.Context, deviceID string) (*Device, error)
	ListDevices(ctx context.Context) ([]Device, error)
}

type CommandPublisher interface {
	PublishMeasure(ctx context.Context, deviceID string, sensors []string) error
	PublishPump(ctx context.Context, deviceID string, target string, state bool, level int) error
	PublishClearQueue(ctx context.Context, deviceID string) error
	PublishSchedule(ctx context.Context, deviceID string, enabled bool, temp, ph, turb, tds int) error
	PublishAutoToggle(ctx context.Context, deviceID string, enabled bool) error
}

type EventHub interface {
	Broadcast(msgType string, payload any)
}
