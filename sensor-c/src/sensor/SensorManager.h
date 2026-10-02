#ifndef SENSOR_MANAGER_H
#define SENSOR_MANAGER_H

#include <Arduino.h>

// Đo pH, nhiệt độ DS18B20 và TDS. Chỉ cấp nguồn đúng cảm biến đang đọc.
class SensorManager {
public:
    SensorManager();
    void begin();
    void handle();
    void request(const String &action);
    bool isBusy() const { return busy; }

private:
    static const uint8_t TEMP_DATA_PIN = 4;
    static const uint8_t TEMP_POWER_PIN = 5;
    static const uint8_t PH_ADC_PIN = 6;
    static const uint8_t PH_POWER_PIN = 7;
    static const uint8_t TDS_ADC_PIN = 8;
    static const uint8_t TDS_POWER_PIN = 9;

    static const int PH_SAMPLES = 40;
    static const int TDS_SAMPLES = 30;
    static const uint32_t SAMPLE_INTERVAL_MS = 20;
    static const uint32_t ANALOG_SETTLE_MS = 1000;
    static const uint32_t TEMP_SETTLE_MS = 1000;
    static const uint32_t TEMP_CONVERSION_MS = 800;
    static constexpr float ADC_VREF = 3.3f;
    static constexpr float ADC_MAX = 4095.0f;

    bool busy = false;
    String pending;

    void run(const String &action);
    bool readTemperature(float &celsius);
    bool readAnalog(uint8_t adcPin, uint8_t powerPin, int samples, int &adc, float &voltage);
    int median(int *samples, int count);
    void publishEvent(const String &stage, const String &message);
    void publishSensorData(const String &json);
    void waitMs(uint32_t ms);
};

extern SensorManager sensorManager;

#endif
