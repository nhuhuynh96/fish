#ifndef TDS_MANAGER_H
#define TDS_MANAGER_H

#include <Arduino.h>

// TDS Meter V1.0: nguồn do lệnh MQTT "power" bật tắt, lệnh "tds" chỉ đọc.
// Que TDS bơm dòng xoay chiều vào nước nên chỉ có điện lúc đọc TDS, không lúc đọc pH.
class TdsManager {
public:
    void begin();
    void handle();

    void setPower(const String &sensor, bool on, uint32_t maxOnS);
    void publishPowerStatus();
    void requestRead(int samples, int intervalMs);
    bool isPowered() const { return powered; }
    bool isBusy() const { return busy; }

    static const uint32_t DEFAULT_MAX_ON_S = 300;
    static const uint32_t MAX_MAX_ON_S = 1800;

private:
    // GPIO 3 thuộc ADC1, đọc được khi WiFi bật; GPIO 5 (ADC2) đang làm TX RS485.
    static const uint8_t TDS_ADC_PIN = 3;
    static const uint8_t TDS_POWER_PIN = 10;
    static const int DEFAULT_SAMPLES = 20;
    static const int MAX_SAMPLES = 100;
    static const int DEFAULT_INTERVAL_MS = 100;

    bool powered = false;
    uint32_t onSince = 0;
    uint32_t maxOnMs = 0;
    bool mqttWasConnected = false;
    bool busy = false;
    bool pending = false;
    int pendingSamples = 0;
    int pendingIntervalMs = 0;

    void writePower(bool on, uint32_t maxOnS);
    void run(int samples, int intervalMs);
    void publishPowerEvent(const char *sensor, bool on, uint32_t maxOnS, const String &message, const char *stage = "power");
    void publishEvent(const char *stage, const String &message);
    void waitMs(uint32_t ms);
};

extern TdsManager tdsManager;

#endif // TDS_MANAGER_H
