package fishuc

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

type scriptWater struct {
	svc         *Service
	store       *memory.Store
	full        bool
	cmds        []string
	rejectPower string
	fillTimeout bool
	stuckFloat  bool
}

func (w *scriptWater) note(stage string) {
	w.svc.noteWater(waterDeviceID, waterDeviceID, stage, w.full)
}

func (w *scriptWater) PublishLevel(context.Context, string) error {
	w.cmds = append(w.cmds, "float")
	w.note("water_level")
	return nil
}

func (w *scriptWater) PublishPump(_ context.Context, _ string, target string, state bool, untilFloat bool, duration int) error {
	if !state {
		w.cmds = append(w.cmds, target+":off")
		return nil
	}
	if !untilFloat {
		w.cmds = append(w.cmds, target+":"+strconv.Itoa(duration))
		if target == "drain" && !w.stuckFloat {
			w.full = false
		}
		w.note("time_up")
		return nil
	}
	w.cmds = append(w.cmds, target+":until")
	switch {
	case target == "inlet" && w.full:
		w.note("already_full")
	case target == "inlet" && w.fillTimeout:
		w.note("time_up")
	case target == "inlet":
		w.full = true
		w.note("filled")
	case !w.full:
		w.note("already_empty")
	case w.stuckFloat:
		w.note("time_up")
	default:
		w.full = false
		w.note("drained")
	}
	return nil
}

func (w *scriptWater) PublishSensorPower(_ context.Context, _ string, sensor string, on bool, _ int) error {
	state := "off"
	if on {
		state = "on"
	}
	w.cmds = append(w.cmds, "power:"+sensor+":"+state)
	if on && sensor == w.rejectPower {
		w.svc.noteSensorEvent(sensorDeviceID, sensorDeviceID, "command_error", "", false, "TDS đang bật")
		return nil
	}
	w.svc.noteSensorEvent(sensorDeviceID, sensorDeviceID, "power", sensor, on, "")
	return nil
}

func (w *scriptWater) PublishRead(_ context.Context, _ string, sensor string, _, _ int) error {
	w.cmds = append(w.cmds, "measure:"+sensor)
	name := sensor
	if name == "temp" {
		name = "temperature"
	}
	w.svc.noteSensor(sensorDeviceID, sensorDeviceID, sensorSig{Stage: "data", Names: []string{name}})
	return nil
}

func (w *scriptWater) PublishClearQueue(context.Context, string) error { return nil }
func (w *scriptWater) PublishSchedule(context.Context, string, bool, int, int, int, int) error {
	return nil
}
func (w *scriptWater) PublishAutoToggle(context.Context, string, bool) error { return nil }

// Chờ nước ổn định 1 ms để có sự kiện warmup mà test vẫn nhanh; sensor không cần làm ấm.
func newScript(t *testing.T, full bool) *scriptWater {
	t.Helper()
	prevSettle := waterSettleDelay
	prevTimings := sensorTimings
	waterSettleDelay = time.Millisecond
	sensorTimings = map[string]sensorTiming{}
	t.Cleanup(func() {
		waterSettleDelay = prevSettle
		sensorTimings = prevTimings
	})
	store := memory.NewStore()
	water := &scriptWater{full: full, store: store}
	water.svc = NewService(store, store, store, store, water, nil)
	return water
}

func checkCmds(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("lệnh không đúng\n got %v\nwant %v", got, want)
	}
}

func (w *scriptWater) stages(t *testing.T, deviceID string) []string {
	t.Helper()
	events, err := w.store.ListEvents(context.Background(), deviceID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range events {
		out = append(out, e.Stage)
	}
	return out
}

func countStage(stages []string, stage string) int {
	n := 0
	for _, s := range stages {
		if s == stage {
			n++
		}
	}
	return n
}

var allSensorsCmds = []string{
	"power:ph:on", "measure:ph", "power:ph:off",
	"power:temp:on", "measure:temp", "power:temp:off",
	"power:tds:on", "measure:tds", "power:tds:off",
}

func TestRunMeasure_EmptyBinFillsOnceMeasuresAllThenDrains(t *testing.T) {
	water := newScript(t, false)
	water.svc.runMeasure("pond", []string{"tds", "ph", "temp"})

	want := append([]string{"power:all:off", "float", "inlet:until"}, allSensorsCmds...)
	want = append(want, "drain:until", "drain:20")
	checkCmds(t, water.cmds, want)
	if water.full {
		t.Fatalf("bình phải trống sau khi xả")
	}
	sensor := water.stages(t, sensorDeviceID)
	if countStage(sensor, "warmup") != 1 || countStage(sensor, "job_done") != 1 {
		t.Fatalf("chỉ sensor đầu chờ nước ổn định và lượt phải xong: %v", sensor)
	}
}

func TestRunMeasure_FullBinSkipsFillAndSettle(t *testing.T) {
	water := newScript(t, true)
	water.svc.runMeasure("pond", []string{"all"})

	want := append([]string{"power:all:off", "float"}, allSensorsCmds...)
	want = append(want, "drain:until", "drain:20")
	checkCmds(t, water.cmds, want)
	if n := countStage(water.stages(t, sensorDeviceID), "warmup"); n != 0 {
		t.Fatalf("bình đầy sẵn thì không chờ nước ổn định, warmup=%d", n)
	}
}

func TestRunMeasure_FillTimeoutStillDrainsAndFails(t *testing.T) {
	water := newScript(t, false)
	water.fillTimeout = true
	water.svc.runMeasure("pond", []string{"ph"})

	checkCmds(t, water.cmds, []string{"power:all:off", "float", "inlet:until", "drain:until", "drain:20"})
	if countStage(water.stages(t, sensorDeviceID), "job_failed") != 1 {
		t.Fatalf("bơm không đầy phải báo job_failed")
	}
}

func TestRunMeasure_StuckFloatWarnsButFinishes(t *testing.T) {
	water := newScript(t, true)
	water.stuckFloat = true
	water.svc.runMeasure("pond", []string{"temp"})

	checkCmds(t, water.cmds, []string{
		"power:all:off", "float", "power:temp:on", "measure:temp", "power:temp:off", "drain:until", "drain:20",
	})
	if countStage(water.stages(t, waterDeviceID), "drain_warning") != 1 {
		t.Fatalf("phao không tắt phải có drain_warning")
	}
	if countStage(water.stages(t, sensorDeviceID), "job_done") != 1 {
		t.Fatalf("số đo đã có thì lượt vẫn xong")
	}
}

func TestRunMeasure_PowerRejectedSkipsReadButStillDrains(t *testing.T) {
	water := newScript(t, true)
	water.rejectPower = "ph"
	water.svc.runMeasure("pond", []string{"ph", "tds"})
	checkCmds(t, water.cmds, []string{
		"power:all:off", "float", "power:ph:on", "power:ph:off", "drain:until", "drain:20",
	})
}

func TestRunManualMeasure_NoWaterCommands(t *testing.T) {
	water := newScript(t, false)
	water.svc.runManualMeasure([]string{"all"})
	checkCmds(t, water.cmds, append([]string{"power:all:off"}, allSensorsCmds...))
}

func TestNormalizeSensors_PHBeforeTemp(t *testing.T) {
	got := strings.Join(normalizeSensors([]string{"temp", "tds", "ph"}), ",")
	if got != "ph,temp,tds" {
		t.Fatalf("thứ tự sai: %s", got)
	}
}
