package wifi

/*
#include "bridge.h"
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"

	"esp32-s3-sensors/internal/config"
)

// toCString chuyển đổi chuỗi Go sang *C.char mà không cần gọi hàm malloc của C (tránh lỗi __wrap_malloc trên bare-metal)
func toCString(s string) *C.char {
	if len(s) == 0 {
		var zero byte = 0
		return (*C.char)(unsafe.Pointer(&zero))
	}
	b := make([]byte, len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return (*C.char)(unsafe.Pointer(&b[0]))
}

// Manager quản lý trạng thái kết nối WiFi và Access Point thông qua C-Bridge
type Manager struct {
	isAPMode bool
}

// NewManager khởi tạo WiFi Manager và kích hoạt hệ thống mạng qua C
func NewManager() *Manager {
	C.wifi_bridge_init()
	return &Manager{
		isAPMode: false,
	}
}

// StartAP kích hoạt phát sóng Access Point thực tế trên ESP32-S3
func (m *Manager) StartAP(apSSID string, htmlPortal string) bool {
	m.isAPMode = true

	cSSID := toCString(apSSID)
	cPass := toCString("")
	cIP := toCString("192.168.4.1")

	// 1. Kích hoạt phát sóng Access Point qua C-Bridge
	ok := C.wifi_bridge_start_ap(cSSID, cPass, cIP)
	if !bool(ok) {
		fmt.Printf("[Go WiFi] Lỗi khi kích hoạt Access Point: %s\n", apSSID)
		return false
	}

	// 2. Khởi động Web Portal HTTP Server & DNS Captive Portal qua C-Bridge
	cHTML := toCString(htmlPortal)
	C.wifi_bridge_start_portal_server(cHTML)

	fmt.Println("\n==========================================")
	fmt.Printf("[Go WiFi] ĐÃ KÍCH HOẠT ACCESS POINT: %s\n", apSSID)
	fmt.Println("[Go WiFi] Vui lòng kết nối vào mạng WiFi này trên điện thoại/laptop")
	fmt.Println("[Go WiFi] Địa chỉ cấu hình: http://192.168.4.1")
	fmt.Println("==========================================\n")

	return true
}

// StopAP tắt chế độ Access Point
func (m *Manager) StopAP() {
	if m.isAPMode {
		C.wifi_bridge_stop_ap()
		m.isAPMode = false
	}
}

// ConnectSTA kết nối tới mạng WiFi Station đã lưu trong cấu hình
func (m *Manager) ConnectSTA(cfg config.DeviceConfig, timeout time.Duration) bool {
	m.isAPMode = false

	cSSID := toCString(cfg.WifiSSID)
	cPass := toCString(cfg.WifiPass)

	fmt.Printf("[Go WiFi] Đang kết nối tới SSID: %s (Timeout %v)...\n", cfg.WifiSSID, timeout)
	ok := C.wifi_bridge_connect_sta(cSSID, cPass, C.uint32_t(timeout.Milliseconds()))

	if bool(ok) {
		fmt.Println("[Go WiFi] Kết nối WiFi thành công!")
		fmt.Printf("[Go WiFi] IP Address: %s\n", m.IP())
		return true
	}

	fmt.Println("[Go WiFi] Kết nối WiFi thất bại!")
	return false
}

// IsConnected kiểm tra trạng thái kết nối WiFi
func (m *Manager) IsConnected() bool {
	return bool(C.wifi_bridge_is_connected())
}

// IsAP kiểm tra xem đang ở chế độ Access Point hay không
func (m *Manager) IsAP() bool {
	return m.isAPMode
}

// IP trả về địa chỉ IP hiện tại
func (m *Manager) IP() string {
	cIP := C.wifi_bridge_get_ip()
	return C.GoString(cIP)
}

// RSSI trả về cường độ sóng WiFi
func (m *Manager) RSSI() int {
	return int(C.wifi_bridge_get_rssi())
}

// CheckSubmittedConfig kiểm tra xem người dùng có vừa lưu cấu hình trên Web Portal không
func (m *Manager) CheckSubmittedConfig() (config.DeviceConfig, bool) {
	if !bool(C.wifi_bridge_has_submitted_config()) {
		return config.DefaultConfig(), false
	}

	var ssidBuf [64]C.char
	var passBuf [64]C.char
	var hostBuf [64]C.char
	var port C.int = 1883
	var userBuf [32]C.char
	var mqttPassBuf [32]C.char
	var idBuf [32]C.char

	C.wifi_bridge_get_submitted_config(
		&ssidBuf[0], 64,
		&passBuf[0], 64,
		&hostBuf[0], 64,
		&port,
		&userBuf[0], 32,
		&mqttPassBuf[0], 32,
		&idBuf[0], 32,
	)

	cfg := config.DeviceConfig{
		WifiSSID: C.GoString(&ssidBuf[0]),
		WifiPass: C.GoString(&passBuf[0]),
		MqttHost: C.GoString(&hostBuf[0]),
		MqttPort: int(port),
		MqttUser: C.GoString(&userBuf[0]),
		MqttPass: C.GoString(&mqttPassBuf[0]),
		DeviceID: C.GoString(&idBuf[0]),
	}

	return cfg, true
}
