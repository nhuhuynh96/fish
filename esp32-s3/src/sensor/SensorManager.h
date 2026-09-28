#ifndef S3_SENSOR_MANAGER_H
#define S3_SENSOR_MANAGER_H

#include <Arduino.h>

struct RawReading {
    int adc = 0;
    float voltage = 0.0f;
    int sampleCount = 0;
};

class SensorManager {
public:
    SensorManager();
    void begin();
    
    void measureAll();
    void measureTemperature();
    void measurePH();
    void measureTDS();

    void powerOffAll();

private:
    // Chân kết nối cảm biến trên ESP32-S3-N16R8
    const uint8_t TEMP_DATA_PIN = 4;   // DS18B20 OneWire Data (Kéo trở 4.7k lên 3.3V)
    const uint8_t TEMP_POWER_PIN = 5;  // Cấp nguồn DS18B20

    const uint8_t PH_ADC_PIN = 6;      // pH Analog Out (ADC1_CH5)
    const uint8_t PH_POWER_PIN = 7;    // Cấp nguồn pH
    static const int PH_SAMPLE_COUNT = 40;

    const uint8_t TDS_ADC_PIN = 8;     // TDS Analog Out (ADC1_CH7)
    const uint8_t TDS_POWER_PIN = 9;   // Cấp nguồn TDS
    static const int TDS_SAMPLE_COUNT = 30;

    const float ADC_VREF = 3.3f;
    const int ADC_MAX = 4095;
    const unsigned long POWER_SETTLE_MS = 1000;

    float readDS18B20();
    RawReading readRawADC(uint8_t pin, int sampleCount);
    int medianFilter(int *buffer, int count) const;
    void setPower(uint8_t powerPin, bool enable);

    void publishSensorData(const String &jsonPayload);
    void emitEvent(const String &stage, const String &message);
};

extern SensorManager sensorManager;

#endif // S3_SENSOR_MANAGER_H
