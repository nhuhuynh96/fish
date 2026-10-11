package fish

import (
	"context"
	"time"
)

// KitReading is a manual test-kit sample (DO / TAN / NO2 / NO3 / KH), not an ESP sensor.
type KitReading struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	DOMGL      *float64  `json:"do_mg_l,omitempty"`
	TANMGL     *float64  `json:"tan_mg_l,omitempty"`
	NO2MGL     *float64  `json:"no2_mg_l,omitempty"`
	NO3MGL     *float64  `json:"no3_mg_l,omitempty"`
	KHDKH      *float64  `json:"kh_dkh,omitempty"`
	NH3FreeMGL *float64  `json:"nh3_free_mg_l,omitempty"`
	PHUsed     *float64  `json:"ph_used,omitempty"`
	TempUsed   *float64  `json:"temp_used,omitempty"`
	Source     string    `json:"source"`
	MeasuredAt time.Time `json:"measured_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (k *KitReading) HasAny() bool {
	return k.DOMGL != nil || k.TANMGL != nil || k.NO2MGL != nil || k.NO3MGL != nil || k.KHDKH != nil
}

type KitReadingRepository interface {
	SaveKitReading(ctx context.Context, r *KitReading) error
	ListKitReadings(ctx context.Context, deviceID string, limit int) ([]KitReading, error)
	ListKitReadingsSince(ctx context.Context, deviceID string, since time.Time, limit int) ([]KitReading, error)
}
