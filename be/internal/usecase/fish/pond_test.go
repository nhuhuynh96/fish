package fishuc

import (
	"context"
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

func f64(v float64) *float64 { return &v }

func TestHandleSensorData_PHBoardAppliesGainOffset(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := NewService(store, store, store, store, nil, nil)

	cal := fish.DefaultCalibration(phDeviceID)
	cal.PHGain = 1.02
	cal.PHOffset = -0.2
	if err := svc.UpdateCalibration(ctx, cal); err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"device_id":"ph","status":"success","sensors_measured":["ph"],"data":{"ph":7.00},"raw":{"ph_reg":700,"ok":10,"fail":0}}`)
	if err := svc.HandleSensorData(ctx, phDeviceID, payload); err != nil {
		t.Fatal(err)
	}
	m, _ := store.GetLatestMeasurement(ctx, phDeviceID)
	if m == nil || m.PH == nil || m.PHRaw == nil {
		t.Fatalf("thiếu pH: %+v", m)
	}
	if *m.PHRaw != 7.00 || *m.PH < 6.93 || *m.PH > 6.95 {
		t.Fatalf("pH sau hiệu chỉnh phải 1.02*7-0.2=6.94, got raw=%.2f ph=%.3f", *m.PHRaw, *m.PH)
	}
}

func TestPondLatestAndHistoryMergeBoards(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := NewService(store, store, store, store, nil, nil)
	now := time.Now()

	_ = store.SaveMeasurement(ctx, &fish.Measurement{ID: "t", DeviceID: sensorDeviceID, Sensors: []string{"temperature"}, Temperature: f64(27), CreatedAt: now.Add(-3 * time.Minute)})
	_ = store.SaveMeasurement(ctx, &fish.Measurement{ID: "p", DeviceID: phDeviceID, Sensors: []string{"ph"}, PH: f64(8.1), PHRaw: f64(8.2), CreatedAt: now.Add(-2 * time.Minute)})
	_ = store.SaveMeasurement(ctx, &fish.Measurement{ID: "d", DeviceID: sensorDeviceID, Sensors: []string{"tds"}, TDS: f64(400), CreatedAt: now.Add(-1 * time.Minute)})

	latest, err := svc.GetPondLatest(ctx)
	if err != nil || latest == nil {
		t.Fatalf("latest: %v %v", latest, err)
	}
	if latest.PH == nil || *latest.PH != 8.1 || latest.Temperature == nil || latest.TDS == nil {
		t.Fatalf("latest phải có đủ pH, nhiệt độ, TDS: %+v", latest)
	}

	hist, err := svc.ListPondHistory(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 || hist[0].ID != "d" || hist[1].ID != "p" || hist[2].ID != "t" {
		t.Fatalf("lịch sử phải gộp hai bo, mới nhất trước: %+v", hist)
	}
	if latest.TempSource != tempSourceDS18B20 || latest.WaterTemp == nil || *latest.WaterTemp != 27 {
		t.Fatalf("không có nhiệt độ pH thì dùng DS18B20: %+v", latest)
	}
}

func TestWaterTempPrefersPHProbe(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := NewService(store, store, store, store, nil, nil)
	now := time.Now()

	_ = store.SaveMeasurement(ctx, &fish.Measurement{ID: "t", DeviceID: sensorDeviceID, Temperature: f64(26.7), CreatedAt: now.Add(-1 * time.Minute)})
	_ = store.SaveMeasurement(ctx, &fish.Measurement{ID: "p", DeviceID: phDeviceID, PH: f64(7.9), PHTemp: f64(29.6), CreatedAt: now.Add(-2 * time.Minute)})

	latest, _ := svc.GetPondLatest(ctx)
	if latest.TempSource != tempSourcePH || *latest.WaterTemp != 29.6 || *latest.Temperature != 26.7 {
		t.Fatalf("nhiệt độ nước lấy cảm biến pH, DS18B20 vẫn giữ riêng: %+v", latest)
	}

	hist, _ := svc.ListPondHistory(ctx, 10)
	if hist[0].WaterTemp != nil || hist[1].WaterTemp == nil || *hist[1].WaterTemp != 29.6 {
		t.Fatalf("có nhiệt độ pH thì lịch sử không trộn DS18B20: %+v", hist)
	}

	stale := now.Add(-3 * time.Hour)
	store2 := memory.NewStore()
	svc2 := NewService(store2, store2, store2, store2, nil, nil)
	_ = store2.SaveMeasurement(ctx, &fish.Measurement{ID: "p", DeviceID: phDeviceID, PH: f64(7.9), PHTemp: f64(29.6), CreatedAt: stale})
	_ = store2.SaveMeasurement(ctx, &fish.Measurement{ID: "t", DeviceID: sensorDeviceID, Temperature: f64(27.1), CreatedAt: now})
	latest2, _ := svc2.GetPondLatest(ctx)
	if latest2.TempSource != tempSourceDS18B20 || *latest2.WaterTemp != 27.1 {
		t.Fatalf("nhiệt độ pH quá cũ thì dùng DS18B20 dự phòng: %+v", latest2)
	}
}

func TestHandleSensorData_PHBoardStoresProbeTemp(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := NewService(store, store, store, store, nil, nil)
	payload := []byte(`{"device_id":"ph","status":"success","sensors_measured":["ph"],"data":{"ph":7.5},"raw":{"ph_reg":750,"temp_c":29.9,"ok":10,"fail":0}}`)
	if err := svc.HandleSensorData(ctx, phDeviceID, payload); err != nil {
		t.Fatal(err)
	}
	m, _ := store.GetLatestMeasurement(ctx, phDeviceID)
	if m.PHTemp == nil || *m.PHTemp != 29.9 || m.Temperature != nil {
		t.Fatalf("ph_temp phải lưu riêng, không ghi vào temperature: %+v", m)
	}
}
