// Package wifi điều khiển WiFi thật của ESP32-S3 qua espradio.
package wifi

import (
	"fmt"
	"time"

	"tinygo.org/x/drivers/netdev"
	nl "tinygo.org/x/drivers/netlink"
	"tinygo.org/x/espradio"
	link "tinygo.org/x/espradio/netlink"

	"sensor/internal/config"
)

var (
	started bool
	staLink link.Esplink
	ipAddr  string
)

// Manager giữ trạng thái WiFi Station.
type Manager struct{}

// NewManager tạo bộ quản lý WiFi. Radio được bật khi ConnectSTA chạy.
func NewManager() *Manager {
	return &Manager{}
}

func alreadyEnabled(err error) bool {
	if err == nil {
		return false
	}
	if err == espradio.ErrAlreadyEnabled {
		return true
	}
	msg := err.Error()
	return msg == "espradio: radio already enabled" || msg == "radio already enabled"
}

// ConnectSTA kết nối WiFi và chờ DHCP cấp IP. Chỉ trả về true khi có IP thật.
// NetConnect tự bật radio. Enable không được gọi trước đó: lần gọi thứ hai trả ErrAlreadyEnabled và dừng giữa chừng.
func (m *Manager) ConnectSTA(cfg config.DeviceConfig, timeout time.Duration) bool {
	if cfg.WifiSSID == "" {
		fmt.Println("[WiFi] SSID trống")
		return false
	}

	netdev.UseNetdev(&staLink)

	fmt.Printf("[WiFi] Đang kết nối tới %s...\n", cfg.WifiSSID)
	err := staLink.NetConnect(&nl.ConnectParams{
		Ssid:       cfg.WifiSSID,
		Passphrase: cfg.WifiPass,
		Hostname:   cfg.DeviceID,
	})
	if alreadyEnabled(err) {
		fmt.Println("[WiFi] Radio đã bật, thử vào mạng lại...")
		err = espradio.Connect(espradio.STAConfig{
			SSID:     cfg.WifiSSID,
			Password: cfg.WifiPass,
		})
	}
	if err != nil {
		fmt.Println("[WiFi] Lỗi kết nối:", err.Error())
	}

	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	fmt.Println("[WiFi] Đang chờ DHCP...")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		if ip, ok := currentIP(); ok {
			started = true
			ipAddr = ip
			fmt.Printf("\n[WiFi] Đã kết nối. IP: %s\n", ip)
			return true
		}
		fmt.Print(".")
	}
	fmt.Println("\n[WiFi] Hết thời gian chờ IP")
	return false
}

// IsConnected báo đã có địa chỉ IP từ DHCP.
func (m *Manager) IsConnected() bool {
	if !started {
		return false
	}
	ip, ok := currentIP()
	if !ok {
		return false
	}
	ipAddr = ip
	return true
}

// IP trả về địa chỉ DHCP gần nhất.
func (m *Manager) IP() string {
	return ipAddr
}

// RSSI hiện không có API trên espradio cho STA đã kết nối.
func (m *Manager) RSSI() int {
	return 0
}

func currentIP() (ip string, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	addr, err := staLink.Addr()
	if err != nil {
		return "", false
	}
	ip = addr.String()
	if ip == "" || ip == "0.0.0.0" || ip == "invalid IP" {
		return "", false
	}
	return ip, true
}
