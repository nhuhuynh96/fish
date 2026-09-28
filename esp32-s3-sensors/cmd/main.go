package main

import (
	"fmt"
	"machine"
	"time"

	"esp32-s3-sensors/internal/config"
	"esp32-s3-sensors/internal/mqtt"
	"esp32-s3-sensors/internal/portal"
	"esp32-s3-sensors/internal/sensor"
	"esp32-s3-sensors/internal/wifi"
)

const (
	ButtonHoldTime = 3 * time.Second
)

func main() {
	time.Sleep(1 * time.Second)

	fmt.Println("\n==========================================")
	fmt.Println("   ESP32-S3 SENSORS NODE (Go + C-Bridge)  ")
	fmt.Println("==========================================")

	// 1. Khởi tạo chân phần cứng cảm biến
	pins := sensor.DefaultPinConfig()
	bootBtn := machine.Pin(pins.BootButtonPin)
	bootBtn.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	sensorMgr := sensor.NewManager(pins)
	sensorMgr.PowerOffAll()

	// 2. Khởi tạo WiFi Manager với Cgo Bridge
	wifiMgr := wifi.NewManager()
	cfg, hasConfig := config.LoadConfig()

	// 3. Kiểm tra nếu chưa cấu hình hoặc giữ nút BOOT lúc khởi động
	bootPressedAtStartup := !bootBtn.Get() // LOW khi nhấn

	if !hasConfig || bootPressedAtStartup {
		if !hasConfig {
			fmt.Println("[Main] Chưa có cấu hình WiFi/MQTT. Bắt đầu phát Access Point...")
		} else {
			fmt.Println("[Main] Phát hiện giữ nút BOOT. Bắt đầu phát Access Point...")
		}

		apName := fmt.Sprintf("ESP32S3-Sensors-%s", cfg.DeviceID)
		html := portal.GetPortalHTML(cfg, apName)
		wifiMgr.StartAP(apName, html)
	} else {
		// Kết nối WiFi STA
		connected := wifiMgr.ConnectSTA(cfg, 15*time.Second)
		if !connected {
			fmt.Println("[Main] Không thể kết nối WiFi. Tự động chuyển sang AP Mode...")
			apName := fmt.Sprintf("ESP32S3-Sensors-%s", cfg.DeviceID)
			html := portal.GetPortalHTML(cfg, apName)
			wifiMgr.StartAP(apName, html)
		}
	}

	// 4. Khởi tạo MQTT client
	mqttClient := mqtt.NewClient(cfg, sensorMgr)
	if wifiMgr.IsConnected() {
		_ = mqttClient.Connect()
	}

	// 5. Vòng lặp chính
	var buttonPressStart time.Time
	isButtonPressed := false
	startTime := time.Now()

	fmt.Println("[Main] ESP32-S3 đã sẵn sàng hoạt động!")

	for {
		// Kiểm tra nút nhấn BOOT trong lúc chạy để mở lại Access Point
		btnState := !bootBtn.Get()
		if btnState {
			if !isButtonPressed {
				isButtonPressed = true
				buttonPressStart = time.Now()
				fmt.Println("[Button] Đang giữ nút BOOT...")
			} else {
				if time.Since(buttonPressStart) >= ButtonHoldTime && !wifiMgr.IsAP() {
					fmt.Println("\n[Button] Đã giữ nút 3 giây -> Chuyển sang chế độ Access Point!")
					isButtonPressed = false
					apName := fmt.Sprintf("ESP32S3-Sensors-%s", cfg.DeviceID)
					html := portal.GetPortalHTML(cfg, apName)
					wifiMgr.StartAP(apName, html)
				}
			}
		} else {
			if isButtonPressed {
				isButtonPressed = false
			}
		}

		// Nếu đang ở chế độ AP, kiểm tra xem người dùng có bấm "Lưu cấu hình" không
		if wifiMgr.IsAP() {
			newCfg, submitted := wifiMgr.CheckSubmittedConfig()
			if submitted {
				fmt.Println("[Main] Nhận cấu hình mới từ Web Portal. Đang lưu và kết nối lại...")
				config.SaveConfig(newCfg)
				wifiMgr.StopAP()
				wifiMgr.ConnectSTA(newCfg, 15*time.Second)
				if wifiMgr.IsConnected() {
					mqttClient = mqtt.NewClient(newCfg, sensorMgr)
					_ = mqttClient.Connect()
				}
			}
		}

		// Gửi Telemetry định kỳ khi đã kết nối MQTT
		if mqttClient.IsConnected() {
			uptime := uint32(time.Since(startTime).Seconds())
			rssi := wifiMgr.RSSI()
			mqttClient.HandleTelemetry(uptime, rssi)
		}

		time.Sleep(100 * time.Millisecond)
	}
}
