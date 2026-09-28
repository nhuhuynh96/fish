#include "ConfigManager.h"

ConfigManager configManager;
DeviceConfig currentConfig;

ConfigManager::ConfigManager() {}

void ConfigManager::begin() {
    preferences.begin(PREF_NAMESPACE, false);
}

bool ConfigManager::loadConfig(DeviceConfig &config) {
    preferences.begin(PREF_NAMESPACE, true);
    config.wifi_ssid = preferences.getString("wifi_ssid", "");
    config.wifi_pass = preferences.getString("wifi_pass", "");
    config.mqtt_host = preferences.getString("mqtt_host", "");
    config.mqtt_port = preferences.getUShort("mqtt_port", 1883);
    config.mqtt_user = preferences.getString("mqtt_user", "");
    config.mqtt_pass = preferences.getString("mqtt_pass", "");
    config.device_id = preferences.getString("device_id", "fish_s3_sensors");
    preferences.end();

    return config.wifi_ssid.length() > 0 && config.mqtt_host.length() > 0;
}

bool ConfigManager::saveConfig(const DeviceConfig &config) {
    preferences.begin(PREF_NAMESPACE, false);
    preferences.putString("wifi_ssid", config.wifi_ssid);
    preferences.putString("wifi_pass", config.wifi_pass);
    preferences.putString("mqtt_host", config.mqtt_host);
    preferences.putUShort("mqtt_port", config.mqtt_port);
    preferences.putString("mqtt_user", config.mqtt_user);
    preferences.putString("mqtt_pass", config.mqtt_pass);
    preferences.putString("device_id", config.device_id);
    preferences.end();
    return true;
}

bool ConfigManager::clearConfig() {
    preferences.begin(PREF_NAMESPACE, false);
    preferences.clear();
    preferences.end();
    return true;
}

bool ConfigManager::isConfigured() {
    DeviceConfig cfg;
    return loadConfig(cfg);
}
