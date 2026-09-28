package comm

import (
	"bufio"
	"fmt"
	"machine"
	"strings"

	"sensor/internal/sensor"
)

// Protocol Frame Prefixes
const (
	PrefixCmd   = "CMD:"
	PrefixData  = "DATA:"
	PrefixEvent = "EVENT:"
	PrefixError = "ERROR:"
	PingMsg     = "PING"
	PongMsg     = "PONG"
)

// UARTBridge quản lý giao tiếp nối tiếp Serial giữa ESP32-S3 và ESP32
type UARTBridge struct {
	sensorMgr *sensor.Manager
	uart      *machine.UART
	reader    *bufio.Reader
}

// NewUARTBridge khởi tạo giao tiếp Serial với tốc độ 115200 baud
func NewUARTBridge(sm *sensor.Manager) *UARTBridge {
	// Sử dụng cổng Serial mặc định (hoặc UART0 / UART1)
	return &UARTBridge{
		sensorMgr: sm,
	}
}

// SendData gửi chuỗi JSON kết quả đo sang ESP32
func (u *UARTBridge) SendData(jsonPayload string) {
	fmt.Printf("%s%s\n", PrefixData, jsonPayload)
}

// SendEvent gửi thông báo sự kiện sang ESP32
func (u *UARTBridge) SendEvent(stage, message string) {
	fmt.Printf("%s{\"stage\":\"%s\",\"message\":\"%s\"}\n", PrefixEvent, stage, message)
}

// SendError gửi thông báo lỗi sang ESP32
func (u *UARTBridge) SendError(message string) {
	fmt.Printf("%s{\"error\":\"%s\"}\n", PrefixError, message)
}

// HandleLine xử lý một dòng lệnh nhận từ ESP32
func (u *UARTBridge) HandleLine(line string) {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return
	}

	// 1. Kiểm tra nhịp tim PING -> PONG
	if line == PingMsg {
		fmt.Println(PongMsg)
		return
	}

	// 2. Xử lý lệnh đo CMD:...
	if strings.HasPrefix(line, PrefixCmd) {
		action := strings.TrimPrefix(line, PrefixCmd)
		action = strings.ToLower(strings.TrimSpace(action))

		switch action {
		case "ph", "test_ph":
			u.SendEvent("measuring", "ESP32-S3 đang đo cảm biến pH...")
			payload, err := u.sensorMgr.MeasurePH("fish_sensors")
			if err != nil {
				u.SendError(fmt.Sprintf("Lỗi đo pH: %v", err))
				return
			}
			jsonStr, _ := u.sensorMgr.FormatJSON(payload)
			u.SendData(jsonStr)

		case "temp", "temperature", "test_temp":
			u.SendEvent("measuring", "ESP32-S3 đang đo nhiệt độ DS18B20...")
			payload, err := u.sensorMgr.MeasureTemperature("fish_sensors")
			if err != nil {
				u.SendError(fmt.Sprintf("Lỗi đo nhiệt độ: %v", err))
				return
			}
			jsonStr, _ := u.sensorMgr.FormatJSON(payload)
			u.SendData(jsonStr)

		case "tds", "test_tds":
			u.SendEvent("measuring", "ESP32-S3 đang đo cảm biến TDS...")
			payload, err := u.sensorMgr.MeasureTDS("fish_sensors")
			if err != nil {
				u.SendError(fmt.Sprintf("Lỗi đo TDS: %v", err))
				return
			}
			jsonStr, _ := u.sensorMgr.FormatJSON(payload)
			u.SendData(jsonStr)

		case "measure_all", "test", "all":
			u.SendEvent("measuring", "ESP32-S3 đang đo toàn bộ (Nhiệt độ -> pH -> TDS)...")
			payload, err := u.sensorMgr.MeasureAll("fish_sensors")
			if err != nil {
				u.SendError(fmt.Sprintf("Lỗi đo toàn bộ: %v", err))
				return
			}
			jsonStr, _ := u.sensorMgr.FormatJSON(payload)
			u.SendData(jsonStr)

		default:
			u.SendError(fmt.Sprintf("Lệnh không hỗ trợ: %s", action))
		}
	}
}
