package main

import (
	"fmt"
	"time"

	"sensor/internal/config"
	"sensor/internal/mqtt"
	"sensor/internal/sensor"
	"sensor/internal/wifi"
)

func main() {
	time.Sleep(1 * time.Second)

	fmt.Println("\n==========================================")
	fmt.Println("   ESP32-S3 SENSORS NODE                  ")
	fmt.Println("==========================================")

	pins := sensor.DefaultPinConfig()
	sensorMgr := sensor.NewManager(pins)
	sensorMgr.PowerOffAll()

	wifiMgr := wifi.NewManager()
	cfg := config.DefaultConfig()
	fmt.Printf("[Main] WiFi: %s | MQTT: %s:%d | Topic: fish/%s\n",
		cfg.WifiSSID, cfg.MqttHost, cfg.MqttPort, cfg.DeviceID)

	if !wifiMgr.ConnectSTA(cfg, 20*time.Second) {
		fmt.Println("[Main] WiFi chưa kết nối, sẽ thử lại.")
	}

	mqttClient := mqtt.NewClient(cfg, sensorMgr)
	startTime := time.Now()
	lastWifiRetry := time.Now()
	fmt.Println("[Main] ESP32-S3 đã sẵn sàng hoạt động!")

	for {
		if !wifiMgr.IsConnected() && time.Since(lastWifiRetry) >= 10*time.Second {
			lastWifiRetry = time.Now()
			fmt.Println("[Main] WiFi chưa kết nối, thử lại...")
			wifiMgr.ConnectSTA(cfg, 20*time.Second)
		}

		uptime := uint32(time.Since(startTime).Seconds())
		mqttClient.Poll(wifiMgr.IsConnected(), uptime, wifiMgr.RSSI())
		time.Sleep(100 * time.Millisecond)
	}
}
