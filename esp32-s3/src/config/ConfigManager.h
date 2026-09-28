#ifndef S3_CONFIG_MANAGER_H
#define S3_CONFIG_MANAGER_H

#include <Arduino.h>
#include <Preferences.h>

struct DeviceConfig {
    String wifi_ssid;
    String wifi_pass;
    String mqtt_host;
    uint16_t mqtt_port = 1883;
    String mqtt_user;
    String mqtt_pass;
    String device_id = "fish_s3_sensors";
};

class ConfigManager {
public:
    ConfigManager();
    void begin();
    bool loadConfig(DeviceConfig &config);
    bool saveConfig(const DeviceConfig &config);
    bool clearConfig();
    bool isConfigured();

private:
    Preferences preferences;
    const char *PREF_NAMESPACE = "s3_sensors";
};

extern ConfigManager configManager;
extern DeviceConfig currentConfig;

#endif // S3_CONFIG_MANAGER_H
