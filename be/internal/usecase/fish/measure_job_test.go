package fishuc

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

type scriptWater struct {
	svc         *Service
	level       int
	cmds        []string
	rejectPower string
}

func (w *scriptWater) note(stage string, level int) {
	w.svc.noteWater(waterDeviceID, waterDeviceID, stage, level, "")
}

func (w *scriptWater) PublishLevel(context.Context, string) error {
	w.cmds = append(w.cmds, "level")
	w.note("water_level", w.level)
	return nil
}

func (w *scriptWater) PublishPump(_ context.Context, _ string, target string, state bool, level, duration int) error {
	if target == "drain" && state {
		if level == 0 {
			w.cmds = append(w.cmds, "drain:0:"+strconv.Itoa(duration))
			w.level = 0
			w.note("time_up", 0)
			return nil
		}
		w.cmds = append(w.cmds, "drain:"+strconv.Itoa(level))
		w.level = level - 1
		w.note("drained", w.level)
		return nil
	}
	if target == "inlet" && state {
		w.cmds = append(w.cmds, "inlet:"+strconv.Itoa(level))
		w.level = level
		w.note("filled", level)
		return nil
	}
	w.cmds = append(w.cmds, target+":off")
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

func newScript(t *testing.T, start int) *scriptWater {
	t.Helper()
	prevSettle := waterSettleDelay
	prevTimings := sensorTimings
	waterSettleDelay = 0
	sensorTimings = map[string]sensorTiming{}
	t.Cleanup(func() {
		waterSettleDelay = prevSettle
		sensorTimings = prevTimings
	})
	store := memory.NewStore()
	water := &scriptWater{level: start}
	water.svc = NewService(store, store, store, store, water, nil)
	return water
}

func checkCmds(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("lệnh không đúng\n got %v\nwant %v", got, want)
	}
}

func runScript(t *testing.T, start int, sensors []string, want []string) *scriptWater {
	t.Helper()
	water := newScript(t, start)
	water.svc.runMeasure("pond", sensors)
	checkCmds(t, water.cmds, want)
	return water
}

func TestRunMeasure_FullBinDrainsToLevel1ThenFillsForTDS(t *testing.T) {
	water := runScript(t, 2, []string{"tds", "ph", "temp"}, []string{
		"power:all:off",
		"level", "drain:2", "level", "power:ph:on", "measure:ph", "power:ph:off",
		"level", "power:temp:on", "measure:temp", "power:temp:off",
		"level", "inlet:2", "power:tds:on", "measure:tds", "power:tds:off",
		"drain:0:90",
	})
	if water.level != 0 {
		t.Fatalf("bình phải trống sau khi xả, level=%d", water.level)
	}
}

func TestRunMeasure_EmptyBinTDS(t *testing.T) {
	runScript(t, 0, []string{"tds"}, []string{
		"power:all:off", "level", "inlet:2", "power:tds:on", "measure:tds", "power:tds:off", "drain:0:90",
	})
}

func TestRunMeasure_EmptyBinTemp(t *testing.T) {
	runScript(t, 0, []string{"temp"}, []string{
		"power:all:off", "level", "inlet:1", "power:temp:on", "measure:temp", "power:temp:off", "drain:0:90",
	})
}

func TestRunMeasure_PowerRejectedSkipsReadButStillDrains(t *testing.T) {
	water := newScript(t, 1)
	water.rejectPower = "ph"
	water.svc.runMeasure("pond", []string{"ph", "tds"})
	checkCmds(t, water.cmds, []string{
		"power:all:off", "level", "power:ph:on", "power:ph:off", "drain:0:90",
	})
}

func TestRunManualMeasure_NoWaterCommands(t *testing.T) {
	water := newScript(t, 0)
	water.svc.runManualMeasure([]string{"all"})
	checkCmds(t, water.cmds, []string{
		"power:all:off",
		"power:ph:on", "measure:ph", "power:ph:off",
		"power:temp:on", "measure:temp", "power:temp:off",
		"power:tds:on", "measure:tds", "power:tds:off",
	})
}

func TestNormalizeSensors_PHBeforeTemp(t *testing.T) {
	got := strings.Join(normalizeSensors([]string{"temp", "tds", "ph"}), ",")
	if got != "ph,temp,tds" {
		t.Fatalf("thứ tự sai: %s", got)
	}
}

func TestSensorFillLevel(t *testing.T) {
	if sensorFillLevel("temp") != 1 || sensorFillLevel("ph") != 1 || sensorFillLevel("tds") != 2 {
		t.Fatalf("phao sai: temp=%d tds=%d ph=%d", sensorFillLevel("temp"), sensorFillLevel("tds"), sensorFillLevel("ph"))
	}
}
