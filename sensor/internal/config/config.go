package config

// DeviceConfig lưu trữ cấu hình kết nối WiFi và MQTT.
type DeviceConfig struct {
	WifiSSID string `json:"wifi_ssid"`
	WifiPass string `json:"wifi_pass"`
	MqttHost string `json:"mqtt_host"`
	MqttPort int    `json:"mqtt_port"`
	MqttUser string `json:"mqtt_user"`
	MqttPass string `json:"mqtt_pass"`
	DeviceID string `json:"device_id"`
}

// DefaultConfig trả về thông số kết nối cố định.
// Topic MQTT có dạng fish/<DeviceID>/<subtopic>, ví dụ fish/hoca1/sensor_data.
func DefaultConfig() DeviceConfig {
	return DeviceConfig{
		WifiSSID: "HUYNH HOANG",
		WifiPass: "12365721",
		MqttHost: "192.168.1.5",
		MqttPort: 1883,
		MqttUser: "",
		MqttPass: "",
		DeviceID: "sensor",
	}
}
