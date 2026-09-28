package wifi

/*
#include "bridge.h"
*/
import "C"
import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"esp32-s3-water/internal/config"
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
	return &Manager{}
}

// Scan quét danh sách WiFi xung quanh để hiển thị trong ô chọn của portal
func (m *Manager) Scan() []string {
	var buf [512]C.char
	n := int(C.wifi_bridge_scan(&buf[0], C.int(len(buf))))
	if n <= 0 {
		return nil
	}
	var out []string
	for _, s := range strings.Split(C.GoString(&buf[0]), "\n") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// StartAP kích hoạt phát sóng Access Point trên ESP32-S3
func (m *Manager) StartAP(apSSID string, htmlPortal string) bool {
	m.isAPMode = true

	ok := C.wifi_bridge_start_ap(toCString(apSSID), toCString(""), toCString("192.168.4.1"))
	if !bool(ok) {
		fmt.Printf("[Go WiFi] Lỗi khi kích hoạt Access Point: %s\n", apSSID)
		return false
	}

	C.wifi_bridge_start_portal_server(toCString(htmlPortal))

	fmt.Println("\n==========================================")
	fmt.Printf("[Go WiFi] ĐÃ KÍCH HOẠT ACCESS POINT: %s\n", apSSID)
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

	fmt.Printf("[Go WiFi] Đang kết nối tới SSID: %s (Timeout %v)...\n", cfg.WifiSSID, timeout)
	ok := C.wifi_bridge_connect_sta(toCString(cfg.WifiSSID), toCString(cfg.WifiPass), C.uint32_t(timeout.Milliseconds()))
	if bool(ok) {
		fmt.Printf("[Go WiFi] Kết nối WiFi thành công! IP: %s\n", m.IP())
		return true
	}
	fmt.Println("[Go WiFi] Kết nối WiFi thất bại!")
	return false
}

func (m *Manager) IsConnected() bool { return bool(C.wifi_bridge_is_connected()) }
func (m *Manager) IsAP() bool        { return m.isAPMode }
func (m *Manager) IP() string        { return C.GoString(C.wifi_bridge_get_ip()) }
func (m *Manager) RSSI() int         { return int(C.wifi_bridge_get_rssi()) }

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
	var topicBuf [96]C.char

	C.wifi_bridge_get_submitted_config(
		&ssidBuf[0], 64,
		&passBuf[0], 64,
		&hostBuf[0], 64,
		&port,
		&userBuf[0], 32,
		&mqttPassBuf[0], 32,
		&idBuf[0], 32,
		&topicBuf[0], 96,
	)

	cfg := config.DeviceConfig{
		WifiSSID: C.GoString(&ssidBuf[0]),
		WifiPass: C.GoString(&passBuf[0]),
		MqttHost: C.GoString(&hostBuf[0]),
		MqttPort: int(port),
		MqttUser: C.GoString(&userBuf[0]),
		MqttPass: C.GoString(&mqttPassBuf[0]),
		DeviceID: C.GoString(&idBuf[0]),
		Topic:    C.GoString(&topicBuf[0]),
	}
	return cfg, true
}
