package main

import (
	"fmt"
	"machine"
	"time"

	"esp32-s3-water/internal/config"
	"esp32-s3-water/internal/mqtt"
	"esp32-s3-water/internal/portal"
	"esp32-s3-water/internal/water"
	"esp32-s3-water/internal/wifi"
)

const (
	ButtonHoldTime = 3 * time.Second
	WifiTimeout    = 15 * time.Second
)

func startPortal(wifiMgr *wifi.Manager, pumps *water.Manager, cfg config.DeviceConfig) {
	pumps.StopAll()
	apName := fmt.Sprintf("ESP32S3-Water-%s", cfg.DeviceID)
	html := portal.GetPortalHTML(cfg, apName, wifiMgr.Scan())
	wifiMgr.StartAP(apName, html)
}

func main() {
	time.Sleep(1 * time.Second)

	fmt.Println("\n==========================================")
	fmt.Println("    ESP32-S3 WATER NODE (Go + C-Bridge)   ")
	fmt.Println("==========================================")

	// 1. Phần cứng bơm / xả / phao
	bootBtn := water.BootPin()
	bootBtn.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	pumps := water.New()

	// 2. WiFi qua C-Bridge
	wifiMgr := wifi.NewManager()
	cfg, hasConfig := config.LoadConfig()

	// 3. Chưa cấu hình hoặc giữ BOOT lúc khởi động -> phát Access Point
	if !hasConfig || !bootBtn.Get() {
		if !hasConfig {
			fmt.Println("[Main] Chưa có cấu hình WiFi/MQTT. Bắt đầu phát Access Point...")
		} else {
			fmt.Println("[Main] Phát hiện giữ nút BOOT. Bắt đầu phát Access Point...")
		}
		startPortal(wifiMgr, pumps, cfg)
	} else if !wifiMgr.ConnectSTA(cfg, WifiTimeout) {
		fmt.Println("[Main] Không thể kết nối WiFi. Tự động chuyển sang AP Mode...")
		startPortal(wifiMgr, pumps, cfg)
	}

	// 4. MQTT theo topic đã cấu hình
	mqttClient := mqtt.NewClient(cfg, pumps)
	if wifiMgr.IsConnected() {
		_ = mqttClient.Connect()
	}

	// 5. Vòng lặp chính
	var buttonPressStart time.Time
	isButtonPressed := false
	startTime := time.Now()

	fmt.Println("[Main] ESP32-S3 Water đã sẵn sàng hoạt động!")

	for {
		if !bootBtn.Get() {
			if !isButtonPressed {
				isButtonPressed = true
				buttonPressStart = time.Now()
				fmt.Println("[Button] Đang giữ nút BOOT...")
			} else if time.Since(buttonPressStart) >= ButtonHoldTime && !wifiMgr.IsAP() {
				fmt.Println("\n[Button] Đã giữ nút 3 giây -> Chuyển sang chế độ Access Point!")
				isButtonPressed = false
				startPortal(wifiMgr, pumps, cfg)
			}
		} else {
			isButtonPressed = false
		}

		if wifiMgr.IsAP() {
			newCfg, submitted := wifiMgr.CheckSubmittedConfig()
			if submitted {
				fmt.Println("[Main] Nhận cấu hình mới từ Web Portal. Đang lưu và kết nối lại...")
				config.SaveConfig(newCfg)
				cfg, _ = config.LoadConfig()
				wifiMgr.StopAP()
				if wifiMgr.ConnectSTA(cfg, WifiTimeout) {
					mqttClient = mqtt.NewClient(cfg, pumps)
					_ = mqttClient.Connect()
				}
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		pumps.Handle()
		mqttClient.Flush()
		if mqttClient.IsConnected() {
			mqttClient.HandleTelemetry(uint32(time.Since(startTime).Seconds()), wifiMgr.RSSI())
		}

		time.Sleep(100 * time.Millisecond)
	}
}
