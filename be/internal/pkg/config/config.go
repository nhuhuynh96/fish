package config

import (
	"os"
)

type Config struct {
	AppPort    string
	DBDSN      string
	MQTTBroker string
	MQTTUser   string
	MQTTPass   string
	MQTTClient string
	CORSOrigin string
}

func Load() *Config {
	return &Config{
		AppPort:    getEnv("APP_PORT", "8080"),
		DBDSN:      getEnv("DB_DSN", ""),
		MQTTBroker: getEnv("MQTT_BROKER", "tcp://broker.emqx.io:1883"),
		MQTTUser:   getEnv("MQTT_USER", ""),
		MQTTPass:   getEnv("MQTT_PASS", ""),
		MQTTClient: getEnv("MQTT_CLIENT_ID", "fish_backend_service"),
		CORSOrigin: getEnv("CORS_ORIGIN", "*"),
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
