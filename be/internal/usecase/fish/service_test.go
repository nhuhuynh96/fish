package fishuc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

func TestHandleSensorData_RawPH(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(store, store, store, store, nil, nil)

	payload := []byte(`{
		"device_id": "esp32_test",
		"timestamp": 100,
		"status": "success",
		"duration_ms": 5000,
		"sensors_measured": ["ph"],
		"raw": {
			"ph": {"adc": 2896, "voltage": 2.334, "sample_count": 40}
		}
	}`)

	if err := svc.HandleSensorData(context.Background(), "esp32_test", payload); err != nil {
		t.Fatalf("HandleSensorData failed: %v", err)
	}

	m, err := store.GetLatestMeasurement(context.Background(), "esp32_test")
	if err != nil {
		t.Fatalf("GetLatestMeasurement failed: %v", err)
	}
	if m == nil || m.PH == nil {
		t.Fatal("expected computed pH")
	}
	if *m.PH < 7.8 || *m.PH > 8.0 {
		t.Fatalf("expected pH ~7.92, got %.2f", *m.PH)
	}
	if m.PHAdc == nil || *m.PHAdc != 2896 {
		t.Fatalf("expected ph_adc=2896, got %v", m.PHAdc)
	}
}

func TestUpdateCalibration_AffectsNextReading(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(store, store, store, store, nil, nil)

	cal := fish.DefaultCalibration("esp32_test")
	cal.PHNeutralV = 2.334
	if err := svc.UpdateCalibration(context.Background(), cal); err != nil {
		t.Fatalf("UpdateCalibration failed: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"device_id":        "esp32_test",
		"timestamp":        101,
		"status":           "success",
		"duration_ms":      5000,
		"sensors_measured": []string{"ph"},
		"raw": map[string]any{
			"ph": map[string]any{"adc": 2896, "voltage": 2.334, "sample_count": 40},
		},
	})

	if err := svc.HandleSensorData(context.Background(), "esp32_test", payload); err != nil {
		t.Fatalf("HandleSensorData failed: %v", err)
	}

	m, _ := store.GetLatestMeasurement(context.Background(), "esp32_test")
	if m.PH == nil || *m.PH < 6.99 || *m.PH > 7.01 {
		t.Fatalf("expected pH ~7.0 after neutral_v=2.334, got %.2f", *m.PH)
	}
}
