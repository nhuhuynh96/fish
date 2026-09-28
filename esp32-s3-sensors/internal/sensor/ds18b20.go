package sensor

import (
	"fmt"
	"machine"
	"time"

	"tinygo.org/x/drivers/ds18b20"
	"tinygo.org/x/drivers/onewire"
)

// DS18B20Driver quản lý cảm biến nhiệt độ OneWire
type DS18B20Driver struct {
	dataPin  machine.Pin
	powerPin machine.Pin
	wire     onewire.Device
	sensor   ds18b20.Device
}

// NewDS18B20Driver khởi tạo driver nhiệt độ với chân data và chân cấp nguồn
func NewDS18B20Driver(dataPin, powerPin machine.Pin) *DS18B20Driver {
	powerPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	powerPin.Low() // Mặc định tắt nguồn để chống ăn mòn điện cực và tiết kiệm pin

	dataPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	w := onewire.New(dataPin)
	sensor := ds18b20.New(w)

	return &DS18B20Driver{
		dataPin:  dataPin,
		powerPin: powerPin,
		wire:     w,
		sensor:   sensor,
	}
}

// SetPower đóng/ngắt nguồn VCC cho DS18B20
func (d *DS18B20Driver) SetPower(enable bool) {
	if enable {
		d.powerPin.High()
		// Chờ nguồn và cảm biến ổn định
		time.Sleep(100 * time.Millisecond)
	} else {
		d.powerPin.Low()
	}
}

// ReadTemperatureC đọc nhiệt độ nước (°C)
func (d *DS18B20Driver) ReadTemperatureC() (float32, error) {
	d.SetPower(true)
	defer d.SetPower(false)

	// Yêu cầu cảm biến chuyển đổi nhiệt độ (truyền nil để áp dụng cho thiết bị duy nhất trên bus)
	d.sensor.RequestTemperature(nil)
	time.Sleep(800 * time.Millisecond)

	temp, err := d.sensor.ReadTemperature(nil)
	if err != nil {
		return 0, fmt.Errorf("lỗi đọc dữ liệu DS18B20: %v", err)
	}

	// Chuyển từ mili-độ C (int32) sang °C (float32)
	tempC := float32(temp) / 1000.0
	return tempC, nil
}
