package samplinguc

import (
	"testing"
	"time"

	"github.com/nhuhuynh/iot-fish/esp32_go/internal/domain/sampling"
)

type fakeHW struct {
	inlet, drain bool
	low, high    bool
	adc          map[sampling.SensorType]sampling.RawReading
}

func (f *fakeHW) SetInlet(on bool) {
	f.inlet = on
	if on {
		f.drain = false
	}
}
func (f *fakeHW) SetDrain(on bool) {
	f.drain = on
	if on {
		f.inlet = false
	}
}
func (f *fakeHW) InletOn() bool                  { return f.inlet }
func (f *fakeHW) DrainOn() bool                  { return f.drain }
func (f *fakeHW) FloatLow() bool                 { return f.low }
func (f *fakeHW) FloatHigh() bool                { return f.high }
func (f *fakeHW) SetSensorPower(sampling.SensorType, bool) {}
func (f *fakeHW) ReadADC(t sampling.SensorType) sampling.RawReading {
	if f.adc != nil {
		if r, ok := f.adc[t]; ok {
			return r
		}
	}
	return sampling.RawReading{ADC: 1000, Voltage: 1.2, SampleCount: 30}
}

type nopBus struct{}

func (nopBus) Publish(string, []byte, bool) error { return nil }
func (nopBus) Connected() bool                    { return false }

func TestEnqueue_Order(t *testing.T) {
	hw := &fakeHW{}
	svc := New("dev1", hw, nopBus{})
	svc.Enqueue(sampling.Command{Action: sampling.ActionPH})
	svc.Enqueue(sampling.Command{Action: sampling.ActionTurbidity})
	svc.Enqueue(sampling.Command{Action: sampling.ActionTDS})
	q := svc.QueueSnapshot()
	if len(q) != 3 {
		t.Fatalf("len=%d", len(q))
	}
	if q[0].Action != sampling.ActionPH || q[1].Action != sampling.ActionTurbidity || q[2].Action != sampling.ActionTDS {
		t.Fatalf("order=%v", q)
	}
}

func TestPH_WaitsUntilLevel1(t *testing.T) {
	hw := &fakeHW{}
	svc := New("dev1", hw, nopBus{})
	svc.Enqueue(sampling.Command{Action: sampling.ActionPH})
	now := time.Now()
	svc.Tick(now)
	if svc.QueueLen() != 1 {
		t.Fatal("ph must stay on queue until float ready")
	}
	if !hw.InletOn() {
		t.Fatal("should start inlet to mức 1")
	}
	hw.low = true
	t2 := now.Add(sampling.PumpFloatGrace + time.Millisecond)
	svc.Tick(t2)
	svc.Tick(t2.Add(sampling.FloatDebounce + time.Millisecond))
	if hw.InletOn() {
		t.Fatal("pump should stop at phao 1")
	}
	if svc.QueueLen() != 0 {
		t.Fatalf("ph should pop after measure, queue=%d", svc.QueueLen())
	}
}

func TestInletHigh_StopsOnFloatHigh(t *testing.T) {
	hw := &fakeHW{}
	svc := New("dev1", hw, nopBus{})
	svc.Enqueue(sampling.Command{Action: sampling.ActionInlet, FillLevel: sampling.FillLevelHigh})
	now := time.Now()
	svc.Tick(now)
	if !hw.InletOn() {
		t.Fatal("inlet should start")
	}
	hw.high = true
	hw.low = true
	t2 := now.Add(sampling.PumpFloatGrace + time.Millisecond)
	svc.Tick(t2)
	svc.Tick(t2.Add(sampling.FloatDebounce + time.Millisecond))
	if hw.InletOn() {
		t.Fatal("inlet should stop at phao 2")
	}
	if svc.QueueLen() != 0 {
		t.Fatalf("inlet command should pop, queue=%d", svc.QueueLen())
	}
}

func TestDrain_StopsAfter30sWithoutInlet(t *testing.T) {
	hw := &fakeHW{}
	svc := New("dev1", hw, nopBus{})
	svc.Enqueue(sampling.Command{Action: sampling.ActionDrain})
	now := time.Now()
	svc.Tick(now)
	if !hw.DrainOn() {
		t.Fatal("drain should start")
	}
	if svc.QueueLen() != 1 {
		t.Fatal("drain stays on queue until 30s")
	}
	svc.Tick(now.Add(sampling.DrainFixedTime + time.Millisecond))
	if hw.DrainOn() {
		t.Fatal("drain should auto-off")
	}
	svc.Tick(now.Add(sampling.DrainFixedTime + 2*time.Millisecond))
	if svc.QueueLen() != 0 {
		t.Fatalf("drain should pop, queue=%d", svc.QueueLen())
	}
}

func TestPH_StuckHighWithoutLow_StartsInletNotDrain(t *testing.T) {
	hw := &fakeHW{high: true, low: false}
	svc := New("dev1", hw, nopBus{})
	svc.Enqueue(sampling.Command{Action: sampling.ActionPH})
	svc.Tick(time.Now())
	if hw.DrainOn() {
		t.Fatal("phao 2 kẹt đầy nhưng phao 1 trống thì không được xả")
	}
	if !hw.InletOn() {
		t.Fatal("phải bơm nạp tới mức 1")
	}
}
