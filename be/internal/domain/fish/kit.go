package fish

import (
	"context"
	"time"
)

// KitReading is a manual test-kit sample (DO / TAN), not an ESP sensor.
type KitReading struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"device_id"`
	DOMGL       *float64  `json:"do_mg_l,omitempty"`
	TANMGL      *float64  `json:"tan_mg_l,omitempty"`
	NH3FreeMGL  *float64  `json:"nh3_free_mg_l,omitempty"`
	PHUsed      *float64  `json:"ph_used,omitempty"`
	TempUsed    *float64  `json:"temp_used,omitempty"`
	Source      string    `json:"source"`
	MeasuredAt  time.Time `json:"measured_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type KitReadingRepository interface {
	SaveKitReading(ctx context.Context, r *KitReading) error
	ListKitReadings(ctx context.Context, deviceID string, limit int) ([]KitReading, error)
	ListKitReadingsSince(ctx context.Context, deviceID string, since time.Time, limit int) ([]KitReading, error)
}
