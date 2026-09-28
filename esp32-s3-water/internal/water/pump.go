package water

import (
	"machine"
	"time"
)

const (
	pinInlet = machine.Pin(18)
	// GPIO19/20 là USB D-/D+ trên ESP32-S3 nên van xả dùng GPIO16
	pinDrain  = machine.Pin(16)
	pinFloat1 = machine.Pin(17)
	pinFloat2 = machine.Pin(4)
	bootPin   = machine.Pin(0)

	floatDebounce = 800 * time.Millisecond
	pumpGrace     = 1 * time.Second
	inletTimeout  = 30 * time.Second
	drainTime     = 30 * time.Second
	inletAttempts = 2
)

type Event struct {
	Stage   string
	Message string
}

// Manager điều khiển bơm nạp, van xả theo phao. Relay kích mức LOW, phao đóng (LOW) là đủ nước
type Manager struct {
	queue           []int
	fillLevel       int
	inletSince      time.Time
	inletAttempt    int
	drainSince      time.Time
	floatSince      time.Time
	drainAfterQueue bool
	inletRunning    bool
	drainRunning    bool
	events          []Event
}

func New() *Manager {
	pinInlet.High()
	pinDrain.High()
	pinInlet.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pinDrain.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pinFloat1.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	pinFloat2.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	m := &Manager{fillLevel: 1}
	m.setInlet(false)
	m.setDrain(false)
	return m
}

func BootPin() machine.Pin { return bootPin }

func levelName(level int) string {
	if level == 2 {
		return "phao 2"
	}
	return "phao 1"
}

// EnqueueInlet xếp lệnh bơm tới phao 1 (level 1) hoặc phao 2 (level 2)
func (m *Manager) EnqueueInlet(level int) {
	if level != 2 {
		level = 1
	}
	m.queue = append(m.queue, level)
	m.drainAfterQueue = true
	m.emit("queued", "Đã xếp bơm nạp tới "+levelName(level))
}

func (m *Manager) InletOff() {
	if !m.inletRunning {
		m.emit("manual_pump", "Bơm nạp đã tắt.")
		return
	}
	m.setInlet(false)
	m.pop()
	if len(m.queue) == 0 {
		m.drainAfterQueue = false
	}
	m.emit("manual_pump", "Đã tắt bơm nạp, bỏ lệnh bơm đang chạy.")
}

func (m *Manager) DrainOn() {
	if m.drainRunning {
		m.emit("draining", "Van xả đã bật.")
		return
	}
	m.setDrain(true)
	m.drainSince = time.Now()
	m.emit("draining", "Van xả: BẬT (tự tắt sau 30 giây)")
}

func (m *Manager) DrainOff() {
	if !m.drainRunning {
		m.emit("draining", "Van xả đã tắt.")
		return
	}
	m.setDrain(false)
	m.emit("draining", "Đã tắt van xả.")
}

func (m *Manager) Clear() {
	n := len(m.queue)
	m.queue = nil
	m.drainAfterQueue = false
	m.inletAttempt = 0
	m.StopAll()
	m.emit("queue_cleared", "Đã xóa hàng đợi ("+itoa(n)+" lệnh).")
}

func (m *Manager) StopAll() {
	if m.inletRunning {
		m.setInlet(false)
	}
	if m.drainRunning {
		m.setDrain(false)
	}
}

func (m *Manager) State() string {
	switch {
	case m.inletRunning:
		return "FILLING_WATER"
	case m.drainRunning:
		return "DRAINING_WATER"
	case len(m.queue) > 0:
		return "BUSY"
	}
	return "IDLE"
}

func (m *Manager) QueueSize() int  { return len(m.queue) }
func (m *Manager) IsInletOn() bool { return m.inletRunning }
func (m *Manager) IsDrainOn() bool { return m.drainRunning }
func (m *Manager) Float1() bool    { return !pinFloat1.Get() }
func (m *Manager) Float2() bool    { return m.Float1() && !pinFloat2.Get() }

func (m *Manager) TakeEvents() []Event {
	out := m.events
	m.events = nil
	return out
}

func (m *Manager) Handle() {
	now := time.Now()
	m.pumpTick(now)
	m.drainTick(now)
	m.queueTick(now)
}

func (m *Manager) atLevel(level int) bool {
	if level == 2 {
		return m.Float2()
	}
	return m.Float1()
}

func (m *Manager) pumpTick(now time.Time) {
	if !m.inletRunning {
		return
	}
	onFor := now.Sub(m.inletSince)
	if onFor < pumpGrace {
		m.floatSince = time.Time{}
		return
	}
	if m.atLevel(m.fillLevel) {
		if m.floatSince.IsZero() {
			m.floatSince = now
		}
		if now.Sub(m.floatSince) >= floatDebounce {
			m.setInlet(false)
			m.inletAttempt = 0
			m.emit("manual_pump", "Đủ "+levelName(m.fillLevel)+". Tắt bơm.")
		}
		return
	}
	m.floatSince = time.Time{}

	if onFor >= inletTimeout {
		if m.inletAttempt >= inletAttempts {
			m.setInlet(false)
			m.failHead("Bơm " + itoa(inletAttempts) + " lần 30s chưa tới mực. Đã ngưng.")
			return
		}
		m.inletAttempt++
		m.inletSince = now
		m.emit("manual_pump", "Chưa đủ mực. Thử lại lần "+itoa(m.inletAttempt)+"/"+itoa(inletAttempts)+".")
	}
}

func (m *Manager) drainTick(now time.Time) {
	if m.drainRunning && now.Sub(m.drainSince) >= drainTime {
		m.setDrain(false)
		m.emit("drained", "Đã xả 30s. Tắt van xả.")
	}
}

func (m *Manager) queueTick(now time.Time) {
	if len(m.queue) == 0 || m.inletRunning || m.drainRunning {
		return
	}
	m.fillLevel = m.queue[0]
	if m.atLevel(m.fillLevel) {
		m.pop()
		m.maybeDrain()
		return
	}
	m.setInlet(true)
	m.inletSince = now
	if m.inletAttempt == 0 {
		m.inletAttempt = 1
	}
	m.emit("manual_pump", "Bơm nạp: BẬT ("+levelName(m.fillLevel)+")")
}

func (m *Manager) pop() {
	if len(m.queue) > 0 {
		m.queue = m.queue[1:]
	}
	m.inletAttempt = 0
}

func (m *Manager) failHead(message string) {
	m.pop()
	if len(m.queue) == 0 {
		m.drainAfterQueue = false
	}
	m.emit("manual_pump_timeout", message)
}

// maybeDrain chỉ xả khi vừa chạy hết hàng đợi bơm; nâng phao tay lúc rảnh thì không xả
func (m *Manager) maybeDrain() {
	if len(m.queue) > 0 || !m.drainAfterQueue {
		return
	}
	m.drainAfterQueue = false
	m.setDrain(true)
	m.drainSince = time.Now()
	m.emit("draining", "Hết hàng đợi bơm. Xả 30 giây.")
}

func (m *Manager) setInlet(on bool) {
	if on {
		pinDrain.High()
		m.drainRunning = false
		pinInlet.Low()
	} else {
		pinInlet.High()
	}
	m.inletRunning = on
	m.floatSince = time.Time{}
}

func (m *Manager) setDrain(on bool) {
	if on {
		pinInlet.High()
		m.inletRunning = false
		pinDrain.Low()
	} else {
		pinDrain.High()
	}
	m.drainRunning = on
}

func (m *Manager) emit(stage, message string) {
	m.events = append(m.events, Event{Stage: stage, Message: message})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
