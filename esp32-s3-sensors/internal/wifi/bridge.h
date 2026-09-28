#ifndef WIFI_BRIDGE_H
#define WIFI_BRIDGE_H

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Khởi tạo phần cứng WiFi & NVS Flash của ESP32-S3
void wifi_bridge_init(void);

// Bật chế độ Access Point phát sóng WiFi thực tế
bool wifi_bridge_start_ap(const char* ssid, const char* pass, const char* ip_str);

// Tắt chế độ Access Point
void wifi_bridge_stop_ap(void);

// Kết nối vào mạng WiFi Station đã cấu hình
bool wifi_bridge_connect_sta(const char* ssid, const char* pass, uint32_t timeout_ms);

// Kiểm tra trạng thái kết nối WiFi Station
bool wifi_bridge_is_connected(void);

// Lấy địa chỉ IP hiện tại
const char* wifi_bridge_get_ip(void);

// Lấy cường độ sóng RSSI (dBm)
int8_t wifi_bridge_get_rssi(void);

// Khởi động Web Server & DNS Captive Portal cho Access Point
void wifi_bridge_start_portal_server(const char* html_page);

// Kiểm tra xem người dùng đã bấm "Lưu cấu hình" trên Web Portal chưa
bool wifi_bridge_has_submitted_config(void);

// Đọc thông tin cấu hình mà người dùng vừa nhập từ Web Portal
void wifi_bridge_get_submitted_config(
    char* out_ssid, int ssid_len,
    char* out_pass, int pass_len,
    char* out_mqtt_host, int host_len,
    int* out_mqtt_port,
    char* out_mqtt_user, int user_len,
    char* out_mqtt_pass, int mqtt_pass_len,
    char* out_device_id, int id_len
);

#ifdef __cplusplus
}
#endif

#endif // WIFI_BRIDGE_H
