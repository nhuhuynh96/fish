package fishuc

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

// Giả lập mạch water và bo ph: ghi lại lệnh, trả sự kiện như firmware thật.
type scriptWater struct {
	svc         *Service
	store       *memory.Store
	full        bool
	cmds        []string
	rejectPower string
	phError     bool
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

// Bơm nạp dừng khi phao đầy hoặc hết giờ; xả chạy đủ giờ.
func (w *scriptWater) PublishPump(_ context.Context, _ string, target string, state bool, duration int) error {
	if !state {
		w.cmds = append(w.cmds, target+":off")
		return nil
	}
	w.cmds = append(w.cmds, target+":"+strconv.Itoa(duration))
	switch {
	case target == "drain":
		if !w.stuckFloat {
			w.full = false
		}
		w.note("time_up")
	case w.full:
		w.note("already_full")
	case w.fillTimeout:
		w.note("time_up")
	default:
		w.full = true
		w.note("filled")
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
		w.svc.noteSensorEvent(phDeviceID, phDeviceID, "command_error", "", false, "đang bận")
		return nil
	}
	w.svc.noteSensorEvent(phDeviceID, phDeviceID, "power", sensor, on, "")
	return nil
}

func (w *scriptWater) PublishRead(_ context.Context, deviceID string, sensor string, _, _ int) error {
	w.cmds = append(w.cmds, "measure:"+sensor+"@"+deviceID)
	if deviceID == phDeviceID && w.phError {
		w.svc.noteSensorEvent(phDeviceID, phDeviceID, "sensor_error", "", false, "Modbus không trả lời")
		return nil
	}
	w.svc.noteSensor(deviceID, deviceID, sensorSig{Stage: "data", Names: []string{sensor}})
	return nil
}

func (w *scriptWater) PublishClearQueue(context.Context, string) error { return nil }
func (w *scriptWater) PublishSchedule(context.Context, string, bool, int, int, int, int) error {
	return nil
}
func (w *scriptWater) PublishAutoToggle(context.Context, string, bool) error { return nil }

// SettleS tính bằng mili giây trong test; sensor không cần làm ấm.
func newScript(t *testing.T, full bool) *scriptWater {
	t.Helper()
	prevUnit := settleUnit
	prevTimings := sensorTimings
	settleUnit = time.Millisecond
	sensorTimings = map[string]sensorTiming{}
	t.Cleanup(func() {
		settleUnit = prevUnit
		sensorTimings = prevTimings
	})
	store := memory.NewStore()
	water := &scriptWater{full: full, store: store}
	water.svc = NewService(store, store, store, store, water, nil)
	return water
}

var testTiming = fish.CycleTiming{FillMaxS: 120, DrainS: 120, SettleS: 45}

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

var refreshCmds = []string{"drain:120", "float", "inlet:120"}

var allSensorsCmds = []string{
	"measure:ph@ph",
	"power:tds:on", "measure:tds@ph", "power:tds:off",
}

func TestRunMeasure_DrainsFillsSettlesMeasuresAndKeepsWater(t *testing.T) {
	water := newScript(t, true)
	water.svc.runMeasure([]string{"tds", "ph"}, testTiming)

	want := append([]string{"power:all:off"}, refreshCmds...)
	want = append(want, allSensorsCmds...)
	checkCmds(t, water.cmds, want)
	if !water.full {
		t.Fatalf("đo xong phải giữ nước để đầu dò pH ngập")
	}
	if countStage(water.stages(t, waterDeviceID), "settle") != 1 {
		t.Fatalf("phải chờ nước ổn định sau khi bơm")
	}
	if countStage(water.stages(t, sensorDeviceID), "job_done") != 1 {
		t.Fatalf("lượt phải xong")
	}
}

func TestRunMeasure_PHOnlySkipsTDSPower(t *testing.T) {
	water := newScript(t, false)
	water.svc.runMeasure([]string{"ph", "temp"}, testTiming)
	checkCmds(t, water.cmds, append(append([]string{}, refreshCmds...), "measure:ph@ph"))
}

func TestRunMeasure_FloatStillFullAfterDrainStopsBeforeMeasuring(t *testing.T) {
	water := newScript(t, true)
	water.stuckFloat = true
	water.svc.runMeasure([]string{"ph"}, testTiming)

	checkCmds(t, water.cmds, []string{"drain:120", "float"})
	if countStage(water.stages(t, sensorDeviceID), "job_failed") != 1 {
		t.Fatalf("xả xong phao vẫn đầy phải báo job_failed, không đo nước cũ")
	}
}

func TestRunMeasure_FillTimeoutFailsWithoutMeasuring(t *testing.T) {
	water := newScript(t, false)
	water.fillTimeout = true
	water.svc.runMeasure([]string{"ph"}, testTiming)

	checkCmds(t, water.cmds, refreshCmds)
	if countStage(water.stages(t, sensorDeviceID), "job_failed") != 1 {
		t.Fatalf("bơm không đầy phải báo job_failed")
	}
}

func TestRunMeasure_UsesConfiguredTiming(t *testing.T) {
	water := newScript(t, false)
	water.svc.runMeasure([]string{"ph"}, fish.CycleTiming{FillMaxS: 60, DrainS: 90, SettleS: 0})
	checkCmds(t, water.cmds, []string{"drain:90", "float", "inlet:60", "measure:ph@ph"})
	if countStage(water.stages(t, waterDeviceID), "settle") != 0 {
		t.Fatalf("SettleS = 0 thì không chờ")
	}
}

func TestRunMeasure_PowerRejectedFailsAfterPH(t *testing.T) {
	water := newScript(t, false)
	water.rejectPower = "tds"
	water.svc.runMeasure([]string{"ph", "tds"}, testTiming)
	want := append([]string{"power:all:off"}, refreshCmds...)
	want = append(want, "measure:ph@ph", "power:tds:on", "power:tds:off")
	checkCmds(t, water.cmds, want)
}

func TestRunMeasure_PhBoardErrorFails(t *testing.T) {
	water := newScript(t, false)
	water.phError = true
	water.svc.runMeasure([]string{"ph"}, testTiming)
	checkCmds(t, water.cmds, append(append([]string{}, refreshCmds...), "measure:ph@ph"))
	if countStage(water.stages(t, sensorDeviceID), "job_failed") != 1 {
		t.Fatalf("bo pH báo lỗi thì lượt phải job_failed")
	}
}

func TestRunManualMeasure_PHOnlySkipsSensorBoard(t *testing.T) {
	water := newScript(t, false)
	water.svc.runManualMeasure([]string{"ph"})
	checkCmds(t, water.cmds, []string{"measure:ph@ph"})
}

func TestRunManualMeasure_NoWaterCommands(t *testing.T) {
	water := newScript(t, false)
	water.svc.runManualMeasure([]string{"all"})
	checkCmds(t, water.cmds, append([]string{"power:all:off"}, allSensorsCmds...))
}

func TestSetPump_RejectsStartWhileJobRunning(t *testing.T) {
	water := newScript(t, false)
	water.svc.jobRunning = true
	if err := water.svc.SetPump(context.Background(), waterDeviceID, "inlet", true, 60); !errors.Is(err, ErrJobRunning) {
		t.Fatalf("đang có lượt đo thì không cho bật bơm tay, err=%v", err)
	}
	if err := water.svc.SetPump(context.Background(), waterDeviceID, "inlet", false, 0); err != nil {
		t.Fatalf("tắt bơm luôn được phép: %v", err)
	}
	checkCmds(t, water.cmds, []string{"inlet:off"})
}

func TestNormalizeSensors_TempReadWithPHBeforeTDS(t *testing.T) {
	got := strings.Join(normalizeSensors([]string{"temp", "tds", "ph"}), ",")
	if got != "ph,tds" {
		t.Fatalf("thứ tự sai: %s", got)
	}
}
