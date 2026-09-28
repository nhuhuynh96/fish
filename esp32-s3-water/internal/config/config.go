package config

import (
	"fmt"
	"strings"
)

// DeviceConfig lưu trữ cấu hình kết nối WiFi, MQTT, device_id và topic
type DeviceConfig struct {
	WifiSSID string `json:"wifi_ssid"`
	WifiPass string `json:"wifi_pass"`
	MqttHost string `json:"mqtt_host"`
	MqttPort int    `json:"mqtt_port"`
	MqttUser string `json:"mqtt_user"`
	MqttPass string `json:"mqtt_pass"`
	DeviceID string `json:"device_id"`
	// Tiền tố topic nhập từ Access Point, ví dụ "fish/be_ca_1"
	Topic string `json:"topic"`
}

// DefaultConfig trả về cấu hình mặc định ban đầu
func DefaultConfig() DeviceConfig {
	return DeviceConfig{
		MqttHost: "192.168.1.10",
		MqttPort: 1883,
		DeviceID: "fish_water",
	}
}

// IsConfigured kiểm tra thiết bị đã có cấu hình WiFi hợp lệ chưa
func (c *DeviceConfig) IsConfigured() bool {
	return len(c.WifiSSID) > 0 && len(c.MqttHost) > 0
}

// TopicPrefix trả về tiền tố topic đã chuẩn hóa. Bỏ trống thì dùng fish/<device_id>
func (c *DeviceConfig) TopicPrefix() string {
	return NormalizeTopic(c.Topic, c.DeviceID)
}

func NormalizeTopic(raw, deviceID string) string {
	t := strings.Trim(strings.TrimSpace(raw), "/")
	t = strings.TrimSuffix(t, "/command")
	if t == "" {
		t = "fish/" + deviceID
	}
	return t
}

// Global active configuration
var CurrentConfig = DefaultConfig()

// In-memory config storage for TinyGo runtime
func LoadConfig() (DeviceConfig, bool) {
	return CurrentConfig, CurrentConfig.IsConfigured()
}

func SaveConfig(cfg DeviceConfig) error {
	cfg.Topic = cfg.TopicPrefix()
	CurrentConfig = cfg
	fmt.Printf("[Config] Đã cập nhật cấu hình: SSID=%s, MQTT=%s:%d, DeviceID=%s, Topic=%s\n",
		cfg.WifiSSID, cfg.MqttHost, cfg.MqttPort, cfg.DeviceID, cfg.Topic)
	return nil
}
