package config

import (
	"fmt"
)

// DeviceConfig lưu trữ cấu hình kết nối WiFi và MQTT
type DeviceConfig struct {
	WifiSSID string `json:"wifi_ssid"`
	WifiPass string `json:"wifi_pass"`
	MqttHost string `json:"mqtt_host"`
	MqttPort int    `json:"mqtt_port"`
	MqttUser string `json:"mqtt_user"`
	MqttPass string `json:"mqtt_pass"`
	DeviceID string `json:"device_id"`
}

// DefaultConfig trả về cấu hình mặc định ban đầu
func DefaultConfig() DeviceConfig {
	return DeviceConfig{
		WifiSSID: "",
		WifiPass: "",
		MqttHost: "192.168.1.100",
		MqttPort: 1883,
		MqttUser: "",
		MqttPass: "",
		DeviceID: "fish_s3_sensors",
	}
}

// IsConfigured kiểm tra thiết bị đã có cấu hình WiFi hợp lệ chưa
func (c *DeviceConfig) IsConfigured() bool {
	return len(c.WifiSSID) > 0 && len(c.MqttHost) > 0
}

// Global active configuration
var CurrentConfig = DefaultConfig()

// In-memory config storage for TinyGo runtime
func LoadConfig() (DeviceConfig, bool) {
	if CurrentConfig.IsConfigured() {
		return CurrentConfig, true
	}
	return CurrentConfig, false
}

func SaveConfig(cfg DeviceConfig) error {
	CurrentConfig = cfg
	fmt.Printf("[Config] Đã cập nhật cấu hình: SSID=%s, MQTT=%s:%d, DeviceID=%s\n",
		cfg.WifiSSID, cfg.MqttHost, cfg.MqttPort, cfg.DeviceID)
	return nil
}
