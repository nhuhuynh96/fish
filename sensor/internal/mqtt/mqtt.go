package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	natiu "github.com/soypat/natiu-mqtt"

	"sensor/internal/config"
	"sensor/internal/sensor"
)

const (
	keepAliveSec     = 60
	pingInterval     = 20 * time.Second
	reconnectDelay   = 5 * time.Second
	readDeadline     = 30 * time.Second
	operationTimeout = 10 * time.Second
)

// CommandMessage cấu trúc lệnh JSON nhận từ MQTT.
type CommandMessage struct {
	Action string `json:"action"`
}

// Client nói chuyện MQTT thật với broker.
type Client struct {
	cfg       config.DeviceConfig
	sensorMgr *sensor.Manager

	conn          net.Conn
	cli           *natiu.Client
	connected     bool
	lastAttempt   time.Time
	lastPing      time.Time
	lastHeartbeat time.Time
	commands      chan string
}

// NewClient khởi tạo MQTT client.
func NewClient(cfg config.DeviceConfig, sm *sensor.Manager) *Client {
	return &Client{
		cfg:       cfg,
		sensorMgr: sm,
		commands:  make(chan string, 4),
	}
}

// Topic sinh tên topic fish/<device_id>/<subtopic>.
func (c *Client) Topic(subTopic string) string {
	return fmt.Sprintf("fish/%s/%s", c.cfg.DeviceID, subTopic)
}

// IsConnected kiểm tra phiên MQTT còn sống.
func (c *Client) IsConnected() bool {
	return c.connected && c.cli != nil && c.cli.IsConnected()
}

// Connect mở TCP tới broker, gửi CONNECT và subscribe fish/<device_id>/command.
func (c *Client) Connect() error {
	c.lastAttempt = time.Now()
	c.close()

	addr := net.JoinHostPort(c.cfg.MqttHost, strconv.Itoa(c.cfg.MqttPort))
	fmt.Printf("[MQTT] Đang kết nối TCP %s...\n", addr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	c.conn = conn

	c.cli = natiu.NewClient(natiu.ClientConfig{
		Decoder: natiu.DecoderNoAlloc{UserBuffer: make([]byte, 1500)},
		OnPub: func(_ natiu.Header, _ natiu.VariablesPublish, r io.Reader) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			select {
			case c.commands <- string(b):
			default:
				fmt.Println("[MQTT] Hàng lệnh đầy, bỏ:", string(b))
			}
			return nil
		},
	})

	var vc natiu.VariablesConnect
	vc.SetDefaultMQTT([]byte(c.clientID()))
	vc.KeepAlive = keepAliveSec
	vc.CleanSession = true
	if c.cfg.MqttUser != "" {
		vc.Username = []byte(c.cfg.MqttUser)
		vc.Password = []byte(c.cfg.MqttPass)
	}
	vc.WillTopic = []byte(c.Topic("status"))
	vc.WillMessage = []byte(fmt.Sprintf(`{"device_id":"%s","status":"offline"}`, escape(c.cfg.DeviceID)))
	vc.WillRetain = true

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	err = c.cli.Connect(ctx, conn, &vc)
	cancel()
	if err != nil {
		c.close()
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), operationTimeout)
	err = c.cli.Subscribe(ctx, natiu.VariablesSubscribe{
		PacketIdentifier: 1,
		TopicFilters: []natiu.SubscribeRequest{{
			TopicFilter: []byte(c.Topic("command")),
			QoS:         natiu.QoS0,
		}},
	})
	cancel()
	if err != nil {
		c.close()
		return err
	}

	c.connected = true
	c.lastPing = time.Now()
	c.lastHeartbeat = time.Time{}
	fmt.Printf("[MQTT] Đã kết nối. Subscribe %s\n", c.Topic("command"))
	go c.readLoop(c.cli, conn)
	c.PublishStatus("online")
	return nil
}

func (c *Client) clientID() string {
	id := "sensors-" + c.cfg.DeviceID
	if len(id) > 23 {
		id = id[:23]
	}
	return id
}

func (c *Client) readLoop(cli *natiu.Client, conn net.Conn) {
	for cli.IsConnected() {
		conn.SetReadDeadline(time.Now().Add(readDeadline))
		if err := cli.HandleNext(); err != nil {
			fmt.Println("[MQTT] Mất kết nối:", err.Error())
			break
		}
	}
	if c.cli == cli {
		c.connected = false
	}
}

func (c *Client) close() {
	if c.cli != nil && c.cli.IsConnected() {
		c.cli.Disconnect(errors.New("reconnect"))
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.cli = nil
	c.conn = nil
	c.connected = false
}

// Poll nhận lệnh, giữ kết nối và gửi telemetry. Gọi từ vòng lặp chính.
func (c *Client) Poll(wifiUp bool, uptimeSec uint32, rssi int) {
	for {
		select {
		case raw := <-c.commands:
			c.HandleCommand(raw)
		default:
			goto drained
		}
	}
drained:
	if !wifiUp {
		if c.connected {
			c.close()
		}
		return
	}
	if !c.IsConnected() {
		if time.Since(c.lastAttempt) >= reconnectDelay {
			if err := c.Connect(); err != nil {
				fmt.Println("[MQTT] Kết nối lỗi:", err.Error())
			}
		}
		return
	}
	if time.Since(c.lastPing) >= pingInterval {
		c.lastPing = time.Now()
		if err := c.cli.StartPing(); err != nil {
			fmt.Println("[MQTT] Ping lỗi:", err.Error())
			c.connected = false
			return
		}
	}
	c.HandleTelemetry(uptimeSec, rssi)
}

func (c *Client) publish(subTopic, payload string, retained bool) bool {
	topic := c.Topic(subTopic)
	if !c.IsConnected() {
		fmt.Printf("[MQTT] Chưa gửi (offline) [%s]: %s\n", topic, payload)
		return false
	}
	flags, err := natiu.NewPublishFlags(natiu.QoS0, false, retained)
	if err != nil {
		fmt.Println("[MQTT] Cờ publish lỗi:", err.Error())
		return false
	}
	// natiu 0.7 từ chối PUBLISH QoS 0 nếu PacketIdentifier bằng 0.
	// Encoder không ghi field này ra dây khi QoS là 0.
	err = c.cli.PublishPayload(flags, natiu.VariablesPublish{
		TopicName:        []byte(topic),
		PacketIdentifier: 1,
	}, []byte(payload))
	if err != nil {
		fmt.Printf("[MQTT] Gửi lỗi [%s]: %v\n", topic, err)
		c.connected = false
		return false
	}
	fmt.Printf("[MQTT PUB] -> [%s]: %s\n", topic, payload)
	return true
}

// Publish gửi payload lên topic con, không retain.
func (c *Client) Publish(subTopic, payload string) {
	c.publish(subTopic, payload, false)
}

// PublishStatus gửi trạng thái hoạt động, retain để broker giữ bản mới nhất.
func (c *Client) PublishStatus(status string) {
	payload := fmt.Sprintf(`{"device_id":"%s","status":"%s","timestamp":%d}`,
		escape(c.cfg.DeviceID), status, time.Now().Unix())
	c.publish("status", payload, true)
}

// PublishEvent gửi thông báo sự kiện.
func (c *Client) PublishEvent(stage, message string) {
	payload := fmt.Sprintf(`{"device_id":"%s","stage":"%s","message":"%s","timestamp":%d}`,
		escape(c.cfg.DeviceID), stage, escape(message), time.Now().Unix())
	c.Publish("event", payload)
}

// PublishSensorPayload gửi kết quả đo.
func (c *Client) PublishSensorPayload(payload *sensor.SensorPayload) error {
	jsonStr, err := c.sensorMgr.FormatJSON(payload)
	if err != nil {
		return err
	}
	c.Publish("sensor_data", jsonStr)
	return nil
}

// HandleCommand xử lý lệnh nhận từ fish/<device_id>/command.
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

// HandleTelemetry gửi nhịp tim định kỳ.
func (c *Client) HandleTelemetry(uptimeSec uint32, rssi int) {
	if !c.IsConnected() || time.Since(c.lastHeartbeat) < 30*time.Second {
		return
	}
	c.lastHeartbeat = time.Now()
	payload := fmt.Sprintf(`{"device_id":"%s","rssi":%d,"uptime":%d,"state":"IDLE"}`,
		escape(c.cfg.DeviceID), rssi, uptimeSec)
	c.Publish("telemetry", payload)
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
