package hwsim

import (
	"log"
	"sync"
	"time"

	"github.com/nhuhuynh/iot-fish/esp32_go/internal/domain/sampling"
)

// Chamber simulates pumps, floats, and ADC (no real GPIO).
type Chamber struct {
	mu     sync.Mutex
	inlet  bool
	drain  bool
	level1 bool
	level2 bool
	since  time.Time
}

func NewChamber() *Chamber {
	return &Chamber{since: time.Now()}
}

func (c *Chamber) SetInlet(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if on {
		c.drain = false
	}
	if on != c.inlet {
		c.since = time.Now()
	}
	c.inlet = on
	if on {
		log.Println("[Pump] BẬT bơm nạp (sim)")
	} else {
		log.Println("[Pump] TẮT bơm nạp (sim)")
	}
}

func (c *Chamber) SetDrain(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if on {
		c.inlet = false
	}
	if on != c.drain {
		c.since = time.Now()
	}
	c.drain = on
	if on {
		log.Println("[Valve] BẬT van xả (sim)")
	} else {
		log.Println("[Valve] TẮT van xả (sim)")
	}
}

func (c *Chamber) InletOn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inlet
}

func (c *Chamber) DrainOn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.drain
}

func (c *Chamber) FloatLow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceLocked()
	return c.level1
}

func (c *Chamber) FloatHigh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceLocked()
	return c.level2
}

func (c *Chamber) SetSensorPower(sensor sampling.SensorType, on bool) {
	if on {
		log.Printf("[Power] %s BẬT (sim)", sensor.Name())
		// powerOffAllSensors()
	}

}

func (c *Chamber) ReadADC(sensor sampling.SensorType) sampling.RawReading {
	switch sensor {
	case sampling.SensorPH:
		return sampling.RawReading{ADC: 1800, Voltage: 1.45, SampleCount: 40}
	case sampling.SensorTurbidity:
		return sampling.RawReading{ADC: 2200, Voltage: 1.78, SampleCount: 30}
	case sampling.SensorTDS:
		return sampling.RawReading{ADC: 900, Voltage: 0.73, SampleCount: 30}
	default:
		return sampling.RawReading{}
	}
}

func (c *Chamber) advanceLocked() {
	elapsed := time.Since(c.since)
	if c.inlet {
		if elapsed >= 2*time.Second {
			c.level1 = true
		}
		if elapsed >= 4*time.Second {
			c.level2 = true
		}
	}
	if c.drain {
		if elapsed >= 1*time.Second {
			c.level2 = false
		}
		if elapsed >= 3*time.Second {
			c.level1 = false
		}
	}
}
