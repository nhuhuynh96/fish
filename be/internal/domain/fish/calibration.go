package fish

import (
	"context"
	"time"
)

type DeviceCalibration struct {
	DeviceID   string    `json:"device_id"`
	PHNeutralV float64   `json:"ph_neutral_v"`
	PHSlope    float64   `json:"ph_slope"`
	TDSTempC   float64   `json:"tds_temp_c"`
	TurbVClear float64   `json:"turb_v_clear"`
	TurbVDirty float64   `json:"turb_v_dirty"`
	TurbNTUMax float64   `json:"turb_ntu_max"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func DefaultCalibration(deviceID string) *DeviceCalibration {
	return &DeviceCalibration{
		DeviceID:   deviceID,
		PHNeutralV: 2.50,
		PHSlope:    0.18,
		TDSTempC:   25.0,
		TurbVClear: 2.15,
		TurbVDirty: 1.00,
		TurbNTUMax: 1000.0,
		UpdatedAt:  time.Now(),
	}
}

type CalibrationRepository interface {
	GetCalibration(ctx context.Context, deviceID string) (*DeviceCalibration, error)
	SaveCalibration(ctx context.Context, cal *DeviceCalibration) error
}
