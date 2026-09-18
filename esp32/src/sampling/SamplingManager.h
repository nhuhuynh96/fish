#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>
#include <vector>
#include <queue>

enum SensorType {
    SENSOR_TEMP = 0,
    SENSOR_PH = 1,
    SENSOR_TURBIDITY = 2,
    SENSOR_TDS = 3
};

enum SamplingState {
    STATE_IDLE,
    STATE_FILLING,
    STATE_STABILIZING,
    STATE_MEASURING,
    STATE_DRAINING,
    STATE_PUBLISHING
};

enum CommandType {
    CMD_MEASURE,
    CMD_PUMP,
    CMD_STATUS
};

struct PendingCommand {
    CommandType type = CMD_STATUS;
    SensorType sensor = SENSOR_TEMP;
    String pumpTarget = "inlet";
    bool pumpState = false;
};

struct RawReading {
    int adc = 0;
    float voltage = 0.0f;
    int sampleCount = 0;
};

struct SensorResult {
    RawReading phRaw;
    bool hasPh = false;

    RawReading turbidityRaw;
    bool hasTurbidity = false;

    RawReading tdsRaw;
    bool hasTds = false;

    unsigned long startTime = 0;
    unsigned long durationMs = 0;
};

class SamplingManager {
public:
    SamplingManager();
    void begin();
    void handle();

    void enqueueMeasure(const std::vector<String> &sensors);
    void enqueuePump(const String &target, bool state);
    void enqueueStatus();
    void clearQueue();

    void setManualPump(const String &target, bool state);
    void publishStatus();

    bool isBusy() const { return currentState != STATE_IDLE; }
    size_t queueSize() const { return commandQueue.size(); }
    SamplingState getState() const { return currentState; }
    String getStateName() const;

private:
    SamplingState currentState = STATE_IDLE;

    std::queue<PendingCommand> commandQueue;

    // Cảm biến của lệnh MEASURE đang chạy
    SensorType currentSensor = SENSOR_PH;

    SensorResult currentResult;

    unsigned long stateTimer = 0;
    unsigned long floatFullSince = 0;
    unsigned long floatEmptySince = 0;
    uint8_t fillFailCount = 0;

    bool manualInletActive = false;
    bool manualDrainActive = false;
    unsigned long manualInletSince = 0;
    unsigned long manualDrainSince = 0;

    const unsigned long STABILIZE_TIME = 500;
    const unsigned long MAX_FILL_TIME = 60000;   // giữ cho tương thích (measure không còn dùng)
    const unsigned long MAX_DRAIN_TIME = 60000;
    const unsigned long FLOAT_DEBOUNCE_TIME = 300;
    const unsigned long MAX_MANUAL_PUMP_TIME = 60000; // bơm/van thủ công: phao hoặc 60s
    static const uint8_t MAX_FILL_FAILS = 3;

    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_VALVE_PIN = 19;
    const uint8_t FLOAT_FULL_PIN = 4;
    const uint8_t FLOAT_EMPTY_PIN = 17;
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_FULL_LEVEL = LOW;
    const uint8_t FLOAT_EMPTY_LEVEL = LOW;

    const uint8_t TDS_SENSOR_PIN = 35;
    const int TDS_ADC_MAX = 4095;
    static const int TDS_SAMPLE_COUNT = 30;

    const uint8_t TURBIDITY_SENSOR_PIN = 34;
    const float ADC_VREF = 3.3f;
    static const int TURBIDITY_SAMPLE_COUNT = 30;

    const uint8_t PH_SENSOR_PIN = 32;
    static const int PH_SAMPLE_COUNT = 40;

    const uint8_t PH_POWER_PIN = 14;
    const uint8_t TURBIDITY_POWER_PIN = 33;
    const uint8_t TDS_POWER_PIN = 25;
    const uint8_t SENSOR_POWER_ON = HIGH;
    const uint8_t SENSOR_POWER_OFF = LOW;
    const unsigned long SENSOR_POWER_SETTLE_MS = 1200;

    void transitionTo(SamplingState newState);
    void measureSensor(SensorType type);
    void publishSensorReading(SensorType type);
    void finishCycle();
    void processCommandQueue();
    void startMeasureCycle(SensorType sensor);
    void clearCommandQueue();

    void handleStateIdle();
    void handleStateFilling(unsigned long elapsed);
    void handleStateStabilizing(unsigned long elapsed);
    void handleStateMeasuring(unsigned long elapsed);
    void handleStateDraining(unsigned long elapsed);
    void handleStatePublishing();
    void handleManualPumpTimeouts();

    bool isWaterFull() const;
    bool isWaterEmpty() const;
    void setInletPump(bool enabled);
    void setDrainValve(bool enabled);
    void setSensorPower(SensorType type, bool enabled);
    void powerOffAllSensors();
    RawReading readRawADC(uint8_t pin, int sampleCount);
    int medianFilter(int *buffer, int count) const;
    String rawReadingJson(const RawReading &reading) const;

    void emitEvent(const String &stage, const String &message, const String &extraJson = "");

    bool parseSensorName(const String &name, std::vector<SensorType> &outList);
    void expandSensorTokens(const std::vector<String> &sensors, std::vector<SensorType> &outList);
    String getSensorName(SensorType type) const;
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
