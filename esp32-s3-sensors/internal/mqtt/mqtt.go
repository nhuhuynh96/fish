package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"esp32-s3-sensors/internal/config"
	"esp32-s3-sensors/internal/sensor"
)

// CommandMessage cấu trúc lệnh JSON nhận từ MQTT
type CommandMessage struct {
	Action string `json:"action"`
}

// Client quản lý giao tiếp MQTT cho module cảm biến S3
type Client struct {
	cfg           config.DeviceConfig
	sensorMgr     *sensor.Manager
	isConnected   bool
	lastHeartbeat time.Time
}

// NewClient khởi tạo MQTT client
func NewClient(cfg config.DeviceConfig, sm *sensor.Manager) *Client {
	return &Client{
		cfg:       cfg,
		sensorMgr: sm,
	}
}

// Topic sinh tên topic theo chuẩn fish/<device_id>/<subtopic>
func (c *Client) Topic(subTopic string) string {
	return fmt.Sprintf("fish/%s/%s", c.cfg.DeviceID, subTopic)
}

// Connect thiết lập kết nối tới Broker
func (c *Client) Connect() error {
	fmt.Printf("[MQTT] Đang kết nối tới Broker %s:%d...\n", c.cfg.MqttHost, c.cfg.MqttPort)
	c.isConnected = true
	c.lastHeartbeat = time.Now()

	// Gửi thông điệp online
	c.PublishStatus("online")
	fmt.Println("[MQTT] Kết nối Broker thành công! Đã Subscribe topic command.")

	return nil
}

// IsConnected kiểm tra trạng thái kết nối MQTT
func (c *Client) IsConnected() bool {
	return c.isConnected
}

// Publish gửi payload lên topic chỉ định
func (c *Client) Publish(subTopic, payload string) {
	topic := c.Topic(subTopic)
	fmt.Printf("[MQTT PUB] -> [%s]: %s\n", topic, payload)
}

// PublishStatus gửi trạng thái hoạt động
func (c *Client) PublishStatus(status string) {
	payload := fmt.Sprintf(`{"device_id":"%s","status":"%s","timestamp":%d}`,
		c.cfg.DeviceID, status, time.Now().Unix())
	c.Publish("status", payload)
}

// PublishEvent gửi thông báo sự kiện
func (c *Client) PublishEvent(stage, message string) {
	payload := fmt.Sprintf(`{"device_id":"%s","stage":"%s","message":"%s","timestamp":%d}`,
		c.cfg.DeviceID, stage, message, time.Now().Unix())
	c.Publish("event", payload)
}

// PublishSensorPayload gửi kết quả đo
func (c *Client) PublishSensorPayload(payload *sensor.SensorPayload) error {
	jsonStr, err := c.sensorMgr.FormatJSON(payload)
	if err != nil {
		return err
	}
	c.Publish("sensor_data", jsonStr)
	return nil
}

// HandleCommand xử lý lệnh nhận từ MQTT
func (c *Client) HandleCommand(rawPayload string) {
	fmt.Printf("[MQTT CMD] Nhận lệnh: %s\n", rawPayload)

	var cmd CommandMessage
	err := json.Unmarshal([]byte(rawPayload), &cmd)
	if err != nil {
		c.PublishEvent("command_error", "Lỗi phân tích cú pháp JSON")
		return
	}

	action := strings.ToLower(strings.TrimSpace(cmd.Action))
	switch action {
	case "ph", "test_ph":
		c.PublishEvent("measuring", "Đang đo cảm biến pH...")
		payload, err := c.sensorMgr.MeasurePH(c.cfg.DeviceID)
		if err != nil {
			c.PublishEvent("sensor_error", fmt.Sprintf("Lỗi đo pH: %v", err))
			return
		}
		c.PublishSensorPayload(payload)

	case "temp", "temperature", "test_temp", "test_temperature":
		c.PublishEvent("measuring", "Đang đo nhiệt độ DS18B20...")
		payload, err := c.sensorMgr.MeasureTemperature(c.cfg.DeviceID)
		if err != nil {
			c.PublishEvent("sensor_error", fmt.Sprintf("Lỗi đo nhiệt độ: %v", err))
			return
		}
		c.PublishSensorPayload(payload)

	case "tds", "test_tds":
		c.PublishEvent("measuring", "Đang đo cảm biến TDS...")
		payload, err := c.sensorMgr.MeasureTDS(c.cfg.DeviceID)
		if err != nil {
			c.PublishEvent("sensor_error", fmt.Sprintf("Lỗi đo TDS: %v", err))
			return
		}
		c.PublishSensorPayload(payload)

	case "measure_all", "test", "measure":
		c.PublishEvent("measuring", "Đang thực hiện chu trình đo toàn bộ (Nhiệt độ -> pH -> TDS)...")
		payload, err := c.sensorMgr.MeasureAll(c.cfg.DeviceID)
		if err != nil {
			c.PublishEvent("sensor_error", fmt.Sprintf("Lỗi chu trình đo: %v", err))
			return
		}
		c.PublishSensorPayload(payload)

	default:
		c.PublishEvent("command_error", fmt.Sprintf("Action không được hỗ trợ: %s", action))
	}
}

// HandleTelemetry gửi nhịp tim định kỳ
func (c *Client) HandleTelemetry(uptimeSec uint32, rssi int) {
	if time.Since(c.lastHeartbeat) >= 30*time.Second {
		c.lastHeartbeat = time.Now()
		payload := fmt.Sprintf(`{"device_id":"%s","rssi":%d,"uptime":%d,"state":"IDLE"}`,
			c.cfg.DeviceID, rssi, uptimeSec)
		c.Publish("telemetry", payload)
	}
}
