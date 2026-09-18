package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// MeasurementRepository
func (s *Store) SaveMeasurement(ctx context.Context, m *fish.Measurement) error {
	query := `
		INSERT INTO measurements (
			id, device_id, timestamp, status, duration_ms, sensors,
			temperature, ph, turbidity, tds,
			ph_adc, ph_voltage, tds_adc, tds_voltage, turbidity_adc, turbidity_voltage,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	_, err := s.db.ExecContext(ctx, query,
		m.ID, m.DeviceID, m.Timestamp, m.Status, m.DurationMs,
		pq.Array(m.Sensors), m.Temperature, m.PH, m.Turbidity, m.TDS,
		m.PHAdc, m.PHVoltage, m.TDSAdc, m.TDSVoltage, m.TurbidityAdc, m.TurbidityVoltage,
		m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert measurement: %w", err)
	}
	return nil
}

func (s *Store) GetLatestMeasurement(ctx context.Context, deviceID string) (*fish.Measurement, error) {
	// Mỗi lần đo 1 cảm biến = 1 row → gộp N bản ghi gần nhất để có đủ ph/tds/turbidity
	list, err := s.ListMeasurements(ctx, deviceID, 20)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}

	merged := list[0] // newest as base (id, timestamp, created_at)
	sensors := make([]string, 0, 4)
	seen := map[string]bool{}

	fill := func(m *fish.Measurement) {
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

	for i := range list {
		fill(&list[i])
	}
	merged.Sensors = sensors
	return &merged, nil
}

func (s *Store) ListMeasurements(ctx context.Context, deviceID string, limit int) ([]fish.Measurement, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, device_id, timestamp, status, duration_ms, sensors,
		       temperature, ph, turbidity, tds,
		       ph_adc, ph_voltage, tds_adc, tds_voltage, turbidity_adc, turbidity_voltage,
		       created_at
		FROM measurements
		WHERE device_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.db.QueryContext(ctx, query, deviceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query measurements: %w", err)
	}
	defer rows.Close()

	list := make([]fish.Measurement, 0)
	for rows.Next() {
		var m fish.Measurement
		var sensors []string
		if err := rows.Scan(
			&m.ID, &m.DeviceID, &m.Timestamp, &m.Status, &m.DurationMs,
			pq.Array(&sensors), &m.Temperature, &m.PH, &m.Turbidity, &m.TDS,
			&m.PHAdc, &m.PHVoltage, &m.TDSAdc, &m.TDSVoltage, &m.TurbidityAdc, &m.TurbidityVoltage,
			&m.CreatedAt,
		); err != nil {
			return nil, err
		}
		m.Sensors = sensors
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetCalibration(ctx context.Context, deviceID string) (*fish.DeviceCalibration, error) {
	query := `
		SELECT device_id, ph_neutral_v, ph_slope, tds_temp_c,
		       tds_ref_v, tds_ref_ppm, tds_max_ppm,
		       turb_v_clear, turb_v_dirty, turb_ntu_max, updated_at
		FROM device_calibration
		WHERE device_id = $1
	`
	row := s.db.QueryRowContext(ctx, query, deviceID)

	var cal fish.DeviceCalibration
	err := row.Scan(
		&cal.DeviceID, &cal.PHNeutralV, &cal.PHSlope, &cal.TDSTempC,
		&cal.TDSRefV, &cal.TDSRefPPM, &cal.TDSMaxPPM,
		&cal.TurbVClear, &cal.TurbVDirty, &cal.TurbNTUMax, &cal.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query calibration: %w", err)
	}
	return &cal, nil
}

func (s *Store) SaveCalibration(ctx context.Context, cal *fish.DeviceCalibration) error {
	query := `
		INSERT INTO device_calibration (
			device_id, ph_neutral_v, ph_slope, tds_temp_c,
			tds_ref_v, tds_ref_ppm, tds_max_ppm,
			turb_v_clear, turb_v_dirty, turb_ntu_max, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (device_id) DO UPDATE SET
			ph_neutral_v = EXCLUDED.ph_neutral_v,
			ph_slope = EXCLUDED.ph_slope,
			tds_temp_c = EXCLUDED.tds_temp_c,
			tds_ref_v = EXCLUDED.tds_ref_v,
			tds_ref_ppm = EXCLUDED.tds_ref_ppm,
			tds_max_ppm = EXCLUDED.tds_max_ppm,
			turb_v_clear = EXCLUDED.turb_v_clear,
			turb_v_dirty = EXCLUDED.turb_v_dirty,
			turb_ntu_max = EXCLUDED.turb_ntu_max,
			updated_at = EXCLUDED.updated_at
	`
	_, err := s.db.ExecContext(ctx, query,
		cal.DeviceID, cal.PHNeutralV, cal.PHSlope, cal.TDSTempC,
		cal.TDSRefV, cal.TDSRefPPM, cal.TDSMaxPPM,
		cal.TurbVClear, cal.TurbVDirty, cal.TurbNTUMax, cal.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert calibration: %w", err)
	}
	return nil
}

// EventRepository
func (s *Store) SaveEvent(ctx context.Context, e *fish.SamplingEvent) error {
	query := `
		INSERT INTO sampling_events (id, device_id, stage, state, message, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.ExecContext(ctx, query, e.ID, e.DeviceID, e.Stage, e.State, e.Message, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *Store) ListEvents(ctx context.Context, deviceID string, limit int) ([]fish.SamplingEvent, error) {
	if limit <= 0 {
		limit = 30
	}
	query := `
		SELECT id, device_id, stage, state, message, created_at
		FROM sampling_events
		WHERE device_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.db.QueryContext(ctx, query, deviceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	list := make([]fish.SamplingEvent, 0)
	for rows.Next() {
		var e fish.SamplingEvent
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Stage, &e.State, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, nil
}

// DeviceRepository
func (s *Store) UpsertDevice(ctx context.Context, d *fish.Device) error {
	query := `
		INSERT INTO devices (id, online, ip, state, last_seen, rssi, uptime)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE
		SET online = EXCLUDED.online,
		    ip = CASE WHEN EXCLUDED.ip <> '' THEN EXCLUDED.ip ELSE devices.ip END,
		    state = CASE WHEN EXCLUDED.state <> '' THEN EXCLUDED.state ELSE devices.state END,
		    last_seen = EXCLUDED.last_seen,
		    rssi = CASE WHEN EXCLUDED.rssi <> 0 THEN EXCLUDED.rssi ELSE devices.rssi END,
		    uptime = CASE WHEN EXCLUDED.uptime <> 0 THEN EXCLUDED.uptime ELSE devices.uptime END
	`
	_, err := s.db.ExecContext(ctx, query, d.ID, d.Online, d.IP, d.State, d.LastSeen, d.RSSI, d.Uptime)
	if err != nil {
		return fmt.Errorf("upsert device: %w", err)
	}
	return nil
}

func (s *Store) GetDevice(ctx context.Context, deviceID string) (*fish.Device, error) {
	query := `SELECT id, online, ip, state, last_seen, rssi, uptime FROM devices WHERE id = $1`
	row := s.db.QueryRowContext(ctx, query, deviceID)

	var d fish.Device
	err := row.Scan(&d.ID, &d.Online, &d.IP, &d.State, &d.LastSeen, &d.RSSI, &d.Uptime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListDevices(ctx context.Context) ([]fish.Device, error) {
	query := `SELECT id, online, ip, state, last_seen, rssi, uptime FROM devices ORDER BY last_seen DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]fish.Device, 0)
	for rows.Next() {
		var d fish.Device
		if err := rows.Scan(&d.ID, &d.Online, &d.IP, &d.State, &d.LastSeen, &d.RSSI, &d.Uptime); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}
