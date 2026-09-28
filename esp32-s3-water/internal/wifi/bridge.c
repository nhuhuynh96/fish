#include "bridge.h"
#include <stdio.h>
#include <string.h>

// Buffer lưu cấu hình người dùng nhập từ Web Portal
static bool g_has_submitted = false;
static char g_ssid[64] = {0};
static char g_pass[64] = {0};
static char g_mqtt_host[64] = {0};
static int  g_mqtt_port = 1883;
static char g_mqtt_user[32] = {0};
static char g_mqtt_pass[32] = {0};
static char g_device_id[32] = {0};
static char g_topic[96] = {0};

static bool g_is_connected = false;
static char g_current_ip[32] = "192.168.4.1";

void wifi_bridge_init(void) {
    printf("[C-Bridge] Khởi tạo subsystem WiFi & Network Layer ESP32-S3...\n");
}

bool wifi_bridge_start_ap(const char* ssid, const char* pass, const char* ip_str) {
    if (ssid == NULL || strlen(ssid) == 0) return false;

    if (ip_str && strlen(ip_str) > 0) {
        strncpy(g_current_ip, ip_str, sizeof(g_current_ip) - 1);
    } else {
        strcpy(g_current_ip, "192.168.4.1");
    }

    printf("\n[C-Bridge] >>> KÍCH HOẠT ACCESS POINT <<<\n");
    printf("[C-Bridge] SSID: %s\n", ssid);
    printf("[C-Bridge] IP: http://%s\n", g_current_ip);
    printf("[C-Bridge] Kênh: 1 | Max Clients: 4\n\n");
    return true;
}

void wifi_bridge_stop_ap(void) {
    printf("[C-Bridge] Đã tắt chế độ Access Point.\n");
}

int wifi_bridge_scan(char* out, int out_len) {
    static const char* found = "FishWiFi\nHome2G\nTP-Link_2.4G";
    if (out == NULL || out_len <= 0) return 0;
    strncpy(out, found, out_len - 1);
    out[out_len - 1] = '\0';
    printf("[C-Bridge] Quét WiFi: 3 mạng\n");
    return 3;
}

bool wifi_bridge_connect_sta(const char* ssid, const char* pass, uint32_t timeout_ms) {
    if (ssid == NULL || strlen(ssid) == 0) return false;

    printf("[C-Bridge] Đang kết nối WiFi Station: %s (Timeout %u ms)...\n", ssid, timeout_ms);
    g_is_connected = true;
    strcpy(g_current_ip, "192.168.1.150");
    printf("[C-Bridge] Kết nối thành công! IP: %s\n", g_current_ip);
    return true;
}

bool wifi_bridge_is_connected(void) {
    return g_is_connected;
}

const char* wifi_bridge_get_ip(void) {
    return g_current_ip;
}

int8_t wifi_bridge_get_rssi(void) {
    return -55; // dBm
}

void wifi_bridge_start_portal_server(const char* html_page) {
    printf("[C-Bridge] Khởi động HTTP Web Server (Port 80) & DNS Server Captive Portal...\n");
}

bool wifi_bridge_has_submitted_config(void) {
    return g_has_submitted;
}

void wifi_bridge_get_submitted_config(
    char* out_ssid, int ssid_len,
    char* out_pass, int pass_len,
    char* out_mqtt_host, int host_len,
    int* out_mqtt_port,
    char* out_mqtt_user, int user_len,
    char* out_mqtt_pass, int mqtt_pass_len,
    char* out_device_id, int id_len,
    char* out_topic, int topic_len
) {
    if (out_ssid) strncpy(out_ssid, g_ssid, ssid_len);
    if (out_pass) strncpy(out_pass, g_pass, pass_len);
    if (out_mqtt_host) strncpy(out_mqtt_host, g_mqtt_host, host_len);
    if (out_mqtt_port) *out_mqtt_port = g_mqtt_port;
    if (out_mqtt_user) strncpy(out_mqtt_user, g_mqtt_user, user_len);
    if (out_mqtt_pass) strncpy(out_mqtt_pass, g_mqtt_pass, mqtt_pass_len);
    if (out_device_id) strncpy(out_device_id, g_device_id, id_len);
    if (out_topic) strncpy(out_topic, g_topic, topic_len);
    g_has_submitted = false;
}
