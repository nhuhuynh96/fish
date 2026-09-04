#include "ConfigManager.h"

ConfigManager configManager;
DeviceConfig currentConfig;

ConfigManager::ConfigManager() {}

void ConfigManager::begin() {
    // Preferences initialization is handled per operation
}

bool ConfigManager::loadConfig(DeviceConfig &config) {
    if (!preferences.begin(PREF_NAMESPACE, true)) { // Read-only mode
        Serial.println("[Config] Không thể mở NVS namespace");
        return false;
    }

    config.wifi_ssid = preferences.getString("ssid", "");
    config.wifi_pass = preferences.getString("pass", "");
    config.mqtt_host = preferences.getString("mqtt_host", "");
    config.mqtt_port = preferences.getUShort("mqtt_port", 1883);
    config.mqtt_user = preferences.getString("mqtt_user", "");
    config.mqtt_pass = preferences.getString("mqtt_pass", "");
    config.device_id = preferences.getString("device_id", "");

    preferences.end();

    // Tự sinh device_id mặc định nếu chưa có
    if (config.device_id.isEmpty()) {
        uint64_t chipid = ESP.getEfuseMac();
        config.device_id = "esp32_" + String((uint32_t)(chipid >> 32), HEX) + String((uint32_t)chipid, HEX);
    }

    Serial.println("[Config] Đọc cấu hình thành công:");
    Serial.printf("  - WiFi SSID: %s\n", config.wifi_ssid.c_str());
    Serial.printf("  - MQTT Host: %s:%u\n", config.mqtt_host.c_str(), config.mqtt_port);
    Serial.printf("  - MQTT User: %s\n", config.mqtt_user.c_str());
    Serial.printf("  - Device ID: %s\n", config.device_id.c_str());

    return !config.wifi_ssid.isEmpty();
}

bool ConfigManager::saveConfig(const DeviceConfig &config) {
    if (!preferences.begin(PREF_NAMESPACE, false)) { // Read-write mode
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

    preferences.end();
    Serial.println("[Config] Đã lưu cấu hình vào NVS!");
    return true;
}

bool ConfigManager::clearConfig() {
    if (!preferences.begin(PREF_NAMESPACE, false)) {
        return false;
    }
    preferences.clear();
    preferences.end();
    Serial.println("[Config] Đã xóa toàn bộ cấu hình trong NVS!");
    return true;
}

bool ConfigManager::isConfigured() {
    if (!preferences.begin(PREF_NAMESPACE, true)) {
        return false;
    }
    String ssid = preferences.getString("ssid", "");
    preferences.end();
    return !ssid.isEmpty();
}
