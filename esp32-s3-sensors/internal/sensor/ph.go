package sensor

import (
	"fmt"
	"machine"
	"time"
)

const (
	PHSampleCount      = 40
	PHSampleIntervalMs = 20
	PHPowerSettleMs    = 1000
	ADCVref            = 3.3
	ADCMax12Bit        = 4095.0
)

// PHSensorDriver quản lý cảm biến pH Analog
type PHSensorDriver struct {
	adcPin   machine.Pin
	powerPin machine.Pin
	adc      machine.ADC
}

// NewPHSensorDriver khởi tạo driver pH
func NewPHSensorDriver(adcPin, powerPin machine.Pin) *PHSensorDriver {
	powerPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	powerPin.Low() // Tắt nguồn mặc định

	machine.InitADC()
	adc := machine.ADC{Pin: adcPin}
	adc.Configure(machine.ADCConfig{})

	return &PHSensorDriver{
		adcPin:   adcPin,
		powerPin: powerPin,
		adc:      adc,
	}
}

// SetPower đóng/ngắt nguồn nuôi mạch đo pH
func (p *PHSensorDriver) SetPower(enable bool) {
	if enable {
		p.powerPin.High()
		time.Sleep(time.Duration(PHPowerSettleMs) * time.Millisecond)
	} else {
		p.powerPin.Low()
	}
}

// ReadRaw đọc nhiều mẫu ADC, lọc trung vị và tính toán điện áp
func (p *PHSensorDriver) ReadRaw() (RawReading, error) {
	p.SetPower(true)
	defer p.SetPower(false)

	samples := make([]int, PHSampleCount)
	for i := 0; i < PHSampleCount; i++ {
		// TinyGo ADC Get() trả về 16-bit (0..65535) -> chuyển đổi về thang 12-bit (0..4095)
		val16 := p.adc.Get()
		val12 := int(val16 >> 4)
		samples[i] = val12
		time.Sleep(time.Duration(PHSampleIntervalMs) * time.Millisecond)
	}

	medianADC := MedianFilter(samples)
	voltage := (float32(medianADC) * ADCVref) / ADCMax12Bit

	fmt.Printf("[pH] ADC Median=%d | Điện áp=%.3fV (Lấy %d mẫu)\n", medianADC, voltage, PHSampleCount)

	return RawReading{
		ADC:         medianADC,
		Voltage:     voltage,
		SampleCount: PHSampleCount,
	}, nil
}
