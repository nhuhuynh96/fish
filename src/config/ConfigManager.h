#ifndef CONFIG_MANAGER_H
#define CONFIG_MANAGER_H

#include <Arduino.h>
#include <Preferences.h>

struct DeviceConfig {
    String wifi_ssid;
    String wifi_pass;
    String mqtt_host;
    uint16_t mqtt_port = 1883;
    String mqtt_user;
    String mqtt_pass;
    String device_id;
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
    const char *PREF_NAMESPACE = "esp32_iot";
};

extern ConfigManager configManager;
extern DeviceConfig currentConfig;

#endif // CONFIG_MANAGER_H
