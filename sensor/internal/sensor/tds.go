package sensor

import (
	"fmt"
	"machine"
	"time"
)

const (
	TDSSampleCount      = 30
	TDSSampleIntervalMs = 20
	TDSPowerSettleMs    = 1000
)

// TDSSensorDriver quản lý cảm biến độ dẫn TDS Analog
type TDSSensorDriver struct {
	adcPin   machine.Pin
	powerPin machine.Pin
	adc      machine.ADC
}

// NewTDSSensorDriver khởi tạo driver TDS
func NewTDSSensorDriver(adcPin, powerPin machine.Pin) *TDSSensorDriver {
	powerPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	powerPin.Low() // Tắt nguồn mặc định

	machine.InitADC()
	adc := machine.ADC{Pin: adcPin}
	adc.Configure(machine.ADCConfig{})

	return &TDSSensorDriver{
		adcPin:   adcPin,
		powerPin: powerPin,
		adc:      adc,
	}
}

// SetPower đóng/ngắt nguồn nuôi mạch đo TDS
func (t *TDSSensorDriver) SetPower(enable bool) {
	if enable {
		t.powerPin.High()
		time.Sleep(time.Duration(TDSPowerSettleMs) * time.Millisecond)
	} else {
		t.powerPin.Low()
	}
}

// ReadRaw đọc nhiều mẫu ADC, lọc trung vị và tính toán điện áp
func (t *TDSSensorDriver) ReadRaw() (RawReading, error) {
	t.SetPower(true)
	defer t.SetPower(false)

	samples := make([]int, TDSSampleCount)
	for i := 0; i < TDSSampleCount; i++ {
		val16 := t.adc.Get()
		val12 := int(val16 >> 4)
		samples[i] = val12
		time.Sleep(time.Duration(TDSSampleIntervalMs) * time.Millisecond)
	}

	medianADC := MedianFilter(samples)
	voltage := (float32(medianADC) * ADCVref) / ADCMax12Bit

	fmt.Printf("[TDS] ADC Median=%d | Điện áp=%.3fV (Lấy %d mẫu)\n", medianADC, voltage, TDSSampleCount)

	return RawReading{
		ADC:         medianADC,
		Voltage:     voltage,
		SampleCount: TDSSampleCount,
	}, nil
}
