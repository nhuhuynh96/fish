#ifndef SENSOR_MANAGER_H
#define SENSOR_MANAGER_H

#include <Arduino.h>

// Đo pH, nhiệt độ DS18B20 và TDS. Nguồn từng cảm biến do lệnh MQTT "power" bật tắt;
// lệnh đọc chỉ đọc, không đụng tới nguồn.
class SensorManager {
public:
    SensorManager();
    void begin();
    void handle();
    void request(const String &action, int samples, int intervalMs);
    void setPower(const String &sensor, bool on, uint32_t maxOnS);
    void publishPowerStatus();
    String powerJson() const;
    bool isBusy() const { return busy; }

    static const uint32_t DEFAULT_MAX_ON_S = 300;
    static const uint32_t MAX_MAX_ON_S = 1800;

private:
    enum { TEMP = 0, PH = 1, TDS = 2, COUNT = 3 };

    static const uint8_t TEMP_DATA_PIN = 4;
    static const uint8_t TEMP_POWER_PIN = 5;
    static const uint8_t PH_ADC_PIN = 6;
    static const uint8_t PH_POWER_PIN = 7;
    static const uint8_t TDS_ADC_PIN = 8;
    static const uint8_t TDS_POWER_PIN = 9;

    static const int PH_SAMPLES = 40;
    static const int TDS_SAMPLES = 20;
    static const int MAX_SAMPLES = 100;
    static const int DEFAULT_INTERVAL_MS = 100;
    static const int MIN_INTERVAL_MS = 20;
    static const int MAX_INTERVAL_MS = 1000;
    static const uint32_t TEMP_CONVERSION_MS = 800;
    static constexpr float ADC_VREF = 3.3f;
    static constexpr float ADC_MAX = 4095.0f;

    bool busy = false;
    String pending;
    int pendingSamples = 0;
    int pendingIntervalMs = 0;

    bool powered[COUNT] = {false, false, false};
    uint32_t onSince[COUNT] = {0, 0, 0};
    uint32_t maxOnMs[COUNT] = {0, 0, 0};
    bool mqttWasConnected = false;

    void run(const String &action, int samples, int intervalMs);
    bool readTemperature(float &celsius);
    bool readAnalog(const char *name, uint8_t adcPin, int samples, int intervalMs, int &adc, float &voltage);
    void writePower(int idx, bool on, uint32_t maxOnS);
    void allOff(const String &reason, bool notify);
    void checkPowerTimeouts();
    void publishPowerEvent(const String &sensor, bool on, uint32_t maxOnS, const String &message);
    void publishEvent(const String &stage, const String &message);
    void publishSensorData(const String &json);
    void waitMs(uint32_t ms);

    static int sensorIndex(const String &name);
    static uint8_t powerPin(int idx);
    static const char *sensorKey(int idx);
    static const char *sensorLabel(int idx);
};

extern SensorManager sensorManager;

#endif
