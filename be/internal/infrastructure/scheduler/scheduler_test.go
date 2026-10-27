package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

type fakeMeasurer struct {
	running bool
	calls   []string
}

func (f *fakeMeasurer) TriggerMeasurement(_ context.Context, _ string, sensors []string) error {
	f.calls = append(f.calls, strings.Join(sensors, "+"))
	return nil
}

func (f *fakeMeasurer) JobRunning() bool { return f.running }

func TestIsDue(t *testing.T) {
	last := time.Unix(1000, 0)
	cases := []struct {
		interval int
		after    time.Duration
		want     bool
	}{
		{fish.IntervalOff, time.Hour, false},
		{fish.IntervalContinuous, 0, true},
		{60, 59 * time.Second, false},
		{60, 60 * time.Second, true},
	}
	for _, c := range cases {
		if got := isDue(c.interval, last, last.Add(c.after)); got != c.want {
			t.Fatalf("isDue(%d, +%v) = %v, want %v", c.interval, c.after, got, c.want)
		}
	}
}

func TestCheckAndTrigger(t *testing.T) {
	ctx := context.Background()
	m := &fakeMeasurer{}
	s := NewScheduler(m, memory.NewStore())
	start := time.Now()
	if _, err := s.SetDeviceSchedule(ctx, fish.MeasureSchedule{
		DeviceID: "sensor", AutoEnabled: true, PhInterval: 0, TempInterval: 60, TdsInterval: -1, TurbInterval: -1,
	}); err != nil {
		t.Fatal(err)
	}

	s.checkAndTrigger(start.Add(time.Second))
	m.running = true
	s.checkAndTrigger(start.Add(61 * time.Second))
	m.running = false
	s.checkAndTrigger(start.Add(62 * time.Second))

	got := strings.Join(m.calls, ",")
	if got != "ph,temp+ph" {
		t.Fatalf("lệnh đo sai: %s", got)
	}
}

func TestScheduleSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	first := NewScheduler(&fakeMeasurer{}, store)
	if _, err := first.SetDeviceSchedule(ctx, fish.MeasureSchedule{
		DeviceID: "sensor", AutoEnabled: true, PhInterval: 900, TempInterval: -5, TdsInterval: 0,
	}); err != nil {
		t.Fatal(err)
	}

	second := NewScheduler(&fakeMeasurer{}, store)
	if err := second.Load(ctx); err != nil {
		t.Fatal(err)
	}
	got := second.GetDeviceSchedule("sensor")
	if !got.AutoEnabled || got.PhInterval != 900 || got.TempInterval != fish.IntervalOff || got.TdsInterval != 0 {
		t.Fatalf("lịch nạp lại sai: %+v", got)
	}
}
