#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>
#include <queue>

enum SensorType {
    SENSOR_TEMP = 0,
    SENSOR_PH = 1,
    SENSOR_TDS = 3
};

enum FillLevel {
    FILL_LEVEL_LOW = 1,  // phao 1 — GPIO17
    FILL_LEVEL_HIGH = 2  // phao 2 — GPIO4
};

struct PendingCommand {
    String action = "";
    FillLevel fillLevel = FILL_LEVEL_LOW;
};

struct RawReading {
    int adc = 0;
    float voltage = 0.0f;
    int sampleCount = 0;
};

class SamplingManager {
public:
    SamplingManager();
    void begin();
    void handle();

    void enqueue(const PendingCommand &cmd);
    void hanldeInletOff();
    void hanldeDrainOff();
    void clearQueue();
    size_t queueSize() const { return commandQueue.size(); }
    size_t testQueueSize() const { return testQueue.size(); }
    String getStateName() const;

private:
    std::queue<PendingCommand> commandQueue;
    std::queue<PendingCommand> testQueue;

    FillLevel fillLevel = FILL_LEVEL_LOW;
    unsigned long inletSince = 0;
    uint8_t inletAttempt = 0;

    unsigned long drainSince = 0;
    unsigned long floatLowSince = 0;
    unsigned long floatHighSince = 0;

    unsigned long measureStartedAt = 0;

    const unsigned long DRAIN_FIXED_TIME = 30000;
    const unsigned long INLET_FIXED_TIME = 30000;
    static const uint8_t INLET_ATTEMPTS = 2;

    const unsigned long FLOAT_DEBOUNCE_TIME = 800;
    const unsigned long PUMP_FLOAT_GRACE_MS = 1000;

    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_VALVE_PIN = 19;
    const uint8_t FLOAT_HIGH_PIN = 4;   // phao 2 — mực cao (TDS)
    const uint8_t FLOAT_LOW_PIN = 17;   // phao 1 — mực thấp (pH)
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    // Khô = HIGH (pull-up). Ướt đóng GND → LOW = đủ.
    const uint8_t FLOAT_LOW_ACTIVE = LOW;
    const uint8_t FLOAT_HIGH_ACTIVE = LOW;

    const uint8_t TDS_SENSOR_PIN = 35;
    const int TDS_ADC_MAX = 4095;
    static const int TDS_SAMPLE_COUNT = 30;

    const float ADC_VREF = 3.3f;

    const uint8_t PH_SENSOR_PIN = 32;
    static const int PH_SAMPLE_COUNT = 40;

    const uint8_t TEMP_DATA_PIN = 27;   // DS18B20 data, kéo 4.7k lên GPIO33
    const uint8_t TEMP_POWER_PIN = 33;  // DS18B20 VCC

    const uint8_t PH_POWER_PIN = 14;
    const uint8_t TDS_POWER_PIN = 25;
    const uint8_t SENSOR_POWER_ON = HIGH;
    const uint8_t SENSOR_POWER_OFF = LOW;
    const unsigned long SENSOR_POWER_SETTLE_MS = 1000;

    enum FillWait { FILL_BUSY = 0, FILL_DONE = 1, FILL_ABORT = 2 };

    void processHead();
    void processTestHead();
    bool isTestAction(const String &action) const;
    void finishCommand();
    void clearLocked(bool emit);
    void stopHardware();

    FillLevel requiredLevel(const String &action, FillLevel parsed) const;
    bool atLevel1() const;
    bool floatReached(FillLevel level) const;
    FillWait ensureFill(FillLevel level, bool lowerIfAboveHigh);
    void applyHeadFillLevel();
    void startInlet();
    void hanldeInletOn(FillLevel level);
    void hanldeDrainOn();
    void handlePump();
    bool measureAfterFill(FillLevel level, SensorType type);

    void measureSensor(SensorType type);
    void measureTemperature();
    float readWaterTempC();
    void publishTemperature(float celsius);
    void publishSensorReading(SensorType type, const RawReading &raw);

    bool isWaterLow() const;
    bool isWaterHigh() const;
    bool isInletOn() const;
    bool isDrainOn() const;
    void setInletPump(bool enabled);
    void setDrainValve(bool enabled);
    void setSensorPower(SensorType type, bool enabled);
    void powerOffAllSensors();
    RawReading readRawADC(uint8_t pin, int sampleCount);
    int medianFilter(int *buffer, int count) const;
    String rawReadingJson(const RawReading &reading) const;
    String getSensorName(SensorType type) const;
    const char *fillLevelName(FillLevel level) const;

    void emitEvent(const String &stage, const String &message, const String &extraJson = "");
};

extern SamplingManager samplingManager;

#endif
