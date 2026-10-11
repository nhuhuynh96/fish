package fishuc

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
)

type scriptWater struct {
	svc   *Service
	level int
	cmds  []string
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

func (w *scriptWater) PublishMeasure(_ context.Context, _ string, sensors []string) error {
	action := sensors[0]
	w.cmds = append(w.cmds, "measure:"+action)
	name := action
	if name == "temp" {
		name = "temperature"
	}
	w.svc.noteSensor(sensorDeviceID, sensorDeviceID, []string{name}, false, "")
	return nil
}

func (w *scriptWater) PublishClearQueue(context.Context, string) error { return nil }
func (w *scriptWater) PublishSchedule(context.Context, string, bool, int, int, int, int) error {
	return nil
}
func (w *scriptWater) PublishAutoToggle(context.Context, string, bool) error { return nil }

func runScript(t *testing.T, start int, sensors []string, want []string) *scriptWater {
	t.Helper()
	prev := waterSettleDelay
	waterSettleDelay = 0
	t.Cleanup(func() { waterSettleDelay = prev })
	store := memory.NewStore()
	water := &scriptWater{level: start}
	svc := NewService(store, store, store, store, water, nil)
	water.svc = svc

	svc.runMeasure("pond", sensors)

	if strings.Join(water.cmds, ",") != strings.Join(want, ",") {
		t.Fatalf("lệnh không đúng\n got %v\nwant %v", water.cmds, want)
	}
	return water
}

func TestRunMeasure_FullBinDrainsToLevel1ThenFillsForTDS(t *testing.T) {
	water := runScript(t, 2, []string{"tds", "ph", "temp"}, []string{
		"level", "drain:2", "level", "measure:ph",
		"level", "measure:temp",
		"level", "inlet:2", "measure:tds",
		"drain:0:90",
	})
	if water.level != 0 {
		t.Fatalf("bình phải trống sau khi xả, level=%d", water.level)
	}
}

func TestRunMeasure_EmptyBinTDS(t *testing.T) {
	runScript(t, 0, []string{"tds"}, []string{"level", "inlet:2", "measure:tds", "drain:0:90"})
}

func TestRunMeasure_EmptyBinTemp(t *testing.T) {
	runScript(t, 0, []string{"temp"}, []string{"level", "inlet:1", "measure:temp", "drain:0:90"})
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
