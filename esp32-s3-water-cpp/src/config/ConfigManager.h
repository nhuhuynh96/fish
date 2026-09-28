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
    // Tiền tố topic, ví dụ "fish/hoca" -> fish/hoca/command, fish/hoca/status...
    String topic;
};

class ConfigManager {
public:
    bool loadConfig(DeviceConfig &config);
    bool saveConfig(const DeviceConfig &config);
    bool clearConfig();

    static String defaultDeviceId();
    static String normalizeTopic(const String &raw, const String &deviceId);

private:
    Preferences preferences;
    const char *PREF_NAMESPACE = "fish_water";
};

extern ConfigManager configManager;
extern DeviceConfig currentConfig;

#endif // CONFIG_MANAGER_H
