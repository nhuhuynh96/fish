package config

import "os"

type Config struct {
	DeviceID     string
	MQTTBroker   string
	MQTTUser     string
	MQTTPass     string
	MQTTClientID string
}

func Load() *Config {
	deviceID := getEnv("DEVICE_ID", "esp32_go_sim")
	return &Config{
		DeviceID:     deviceID,
		MQTTBroker:   getEnv("MQTT_BROKER", "tcp://127.0.0.1:1883"),
		MQTTUser:     getEnv("MQTT_USER", ""),
		MQTTPass:     getEnv("MQTT_PASS", ""),
		MQTTClientID: getEnv("MQTT_CLIENT_ID", deviceID+"_go"),
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
