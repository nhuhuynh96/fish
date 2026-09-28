package mqtt

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"esp32-s3-water/internal/config"
	"esp32-s3-water/internal/water"
)

type CommandMessage struct {
	Action string          `json:"action"`
	Level  json.RawMessage `json:"level"`
}

// Client publish/subscribe theo tiền tố topic nhập từ Access Point; mọi payload có device_id
type Client struct {
	cfg           config.DeviceConfig
	prefix        string
	water         *water.Manager
	connected     bool
	lastHeartbeat time.Time
}

func NewClient(cfg config.DeviceConfig, w *water.Manager) *Client {
	return &Client{cfg: cfg, prefix: cfg.TopicPrefix(), water: w}
}

func (c *Client) Topic(sub string) string {
	return c.prefix + "/" + sub
}

func (c *Client) Connect() error {
	fmt.Printf("[MQTT] Kết nối Broker %s:%d (device_id=%s)\n", c.cfg.MqttHost, c.cfg.MqttPort, c.cfg.DeviceID)
	fmt.Printf("[MQTT] Will %s retained offline\n", c.Topic("status"))
	c.connected = true
	c.lastHeartbeat = time.Now()
	fmt.Printf("[MQTT] Subscribe %s\n", c.Topic("command"))
	c.PublishStatus("online")
	return nil
}

func (c *Client) IsConnected() bool { return c.connected }

func (c *Client) Publish(subTopic, payload string, retained bool) {
	flag := ""
	if retained {
		flag = " retained"
	}
	fmt.Printf("[MQTT PUB%s] %s %s\n", flag, c.Topic(subTopic), payload)
}

func (c *Client) PublishStatus(status string) {
	payload := fmt.Sprintf(`{"device_id":"%s","status":"%s","role":"water","topic":"%s"}`,
		escape(c.cfg.DeviceID), status, escape(c.prefix))
	c.Publish("status", payload, true)
}

func (c *Client) PublishEvent(stage, message string) {
	w := c.water
	payload := fmt.Sprintf(
		`{"device_id":"%s","stage":"%s","state":"%s","message":"%s","inlet_on":%t,"drain_on":%t,"float_low":%t,"float_high":%t,"queue_size":%d}`,
		escape(c.cfg.DeviceID), stage, w.State(), escape(message),
		w.IsInletOn(), w.IsDrainOn(), w.Float1(), w.Float2(), w.QueueSize(),
	)
	c.Publish("event", payload, false)
}

func (c *Client) PublishLog(msg string) {
	payload := fmt.Sprintf(`{"device_id":"%s","uptime_ms":%d,"msg":"%s"}`,
		escape(c.cfg.DeviceID), time.Now().UnixMilli(), escape(msg))
	c.Publish("log", payload, false)
}

// HandleTelemetry gửi telemetry mỗi 30 giây
func (c *Client) HandleTelemetry(uptimeSec uint32, rssi int) {
	if time.Since(c.lastHeartbeat) < 30*time.Second {
		return
	}
	c.lastHeartbeat = time.Now()
	w := c.water
	payload := fmt.Sprintf(
		`{"device_id":"%s","rssi":%d,"uptime":%d,"state":"%s","queue_size":%d,"inlet_on":%t,"drain_on":%t,"float_low":%t,"float_high":%t}`,
		escape(c.cfg.DeviceID), rssi, uptimeSec, w.State(), w.QueueSize(),
		w.IsInletOn(), w.IsDrainOn(), w.Float1(), w.Float2(),
	)
	c.Publish("telemetry", payload, false)
}

// HandleCommand xử lý payload nhận ở <topic>/command
// {"action":"inlet_on","level":1|2} | inlet_off | drain_on | drain_off | clear_queue
func (c *Client) HandleCommand(raw string) {
	c.PublishLog("[Command] " + raw)
	var cmd CommandMessage
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		c.report("command_error", `JSON không hợp lệ. Ví dụ: {"action":"inlet_on","level":1}`)
		return
	}
	action := strings.ToLower(strings.TrimSpace(cmd.Action))
	switch action {
	case "inlet_on", "inlet":
		c.water.EnqueueInlet(parseLevel(cmd.Level))
	case "inlet_off":
		c.water.InletOff()
	case "drain_on":
		c.water.DrainOn()
	case "drain_off":
		c.water.DrainOff()
	case "clear_queue":
		c.water.Clear()
	default:
		c.report("command_error", "Action không hỗ trợ: "+action)
		return
	}
	c.Flush()
}

// Flush publish các event bơm/xả phát sinh từ water.Manager
func (c *Client) Flush() {
	for _, ev := range c.water.TakeEvents() {
		c.report(ev.Stage, ev.Message)
	}
}

func (c *Client) report(stage, message string) {
	fmt.Printf("[Water Event] [%s] %s\n", stage, message)
	c.PublishEvent(stage, message)
	c.PublishLog("[" + stage + "] " + message)
}

func parseLevel(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 1
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		if n >= 2 {
			return 2
		}
		return 1
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 1
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "high" || s == "cao" {
		return 2
	}
	if v, err := strconv.Atoi(s); err == nil && v >= 2 {
		return 2
	}
	return 1
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
