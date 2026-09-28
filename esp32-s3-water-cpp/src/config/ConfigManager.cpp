#include "ConfigManager.h"

ConfigManager configManager;
DeviceConfig currentConfig;

String ConfigManager::defaultDeviceId() {
    uint64_t chipid = ESP.getEfuseMac();
    char buf[16];
    snprintf(buf, sizeof(buf), "water_%04x", (uint16_t)(chipid >> 32));
    return String(buf);
}

String ConfigManager::normalizeTopic(const String &raw, const String &deviceId) {
    String t = raw;
    t.trim();
    while (t.endsWith("/")) t.remove(t.length() - 1);
    while (t.startsWith("/")) t.remove(0, 1);
    if (t.endsWith("/command")) t.remove(t.length() - 8);
    if (t.isEmpty()) t = "fish/" + deviceId;
    return t;
}

bool ConfigManager::loadConfig(DeviceConfig &config) {
    if (!preferences.begin(PREF_NAMESPACE, true)) {
        Serial.println("[Config] Chưa có NVS namespace (lần đầu chạy)");
        config.device_id = defaultDeviceId();
        config.topic = normalizeTopic("", config.device_id);
        return false;
    }

    config.wifi_ssid = preferences.getString("ssid", "");
    config.wifi_pass = preferences.getString("pass", "");
    config.mqtt_host = preferences.getString("mqtt_host", "");
    config.mqtt_port = preferences.getUShort("mqtt_port", 1883);
    config.mqtt_user = preferences.getString("mqtt_user", "");
    config.mqtt_pass = preferences.getString("mqtt_pass", "");
    config.device_id = preferences.getString("device_id", "");
    config.topic = preferences.getString("topic", "");
    preferences.end();

    if (config.device_id.isEmpty()) config.device_id = defaultDeviceId();
    config.topic = normalizeTopic(config.topic, config.device_id);

    Serial.println("[Config] Cấu hình đã lưu:");
    Serial.printf("  - WiFi SSID: %s\n", config.wifi_ssid.c_str());
    Serial.printf("  - MQTT Host: %s:%u\n", config.mqtt_host.c_str(), config.mqtt_port);
    Serial.printf("  - Device ID: %s\n", config.device_id.c_str());
    Serial.printf("  - Topic    : %s/...\n", config.topic.c_str());

    return !config.wifi_ssid.isEmpty();
}

bool ConfigManager::saveConfig(const DeviceConfig &config) {
    if (!preferences.begin(PREF_NAMESPACE, false)) {
        Serial.println("[Config] Lỗi mở NVS để ghi");
        return false;
    }
    preferences.putString("ssid", config.wifi_ssid);
    preferences.putString("pass", config.wifi_pass);
    preferences.putString("mqtt_host", config.mqtt_host);
    preferences.putUShort("mqtt_port", config.mqtt_port);
    preferences.putString("mqtt_user", config.mqtt_user);
    preferences.putString("mqtt_pass", config.mqtt_pass);
    preferences.putString("device_id", config.device_id);
    preferences.putString("topic", config.topic);
    preferences.end();
    Serial.println("[Config] Đã lưu cấu hình vào NVS");
    return true;
}

bool ConfigManager::clearConfig() {
    if (!preferences.begin(PREF_NAMESPACE, false)) return false;
    preferences.clear();
    preferences.end();
    Serial.println("[Config] Đã xóa cấu hình trong NVS");
    return true;
}
