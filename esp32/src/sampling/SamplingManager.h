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
    CMD_STATUS,
    CMD_FILL,
    CMD_DRAIN
};

// Mức nạp nước theo phao
enum FillLevel {
    FILL_LEVEL_LOW = 0,  // GPIO17 mức 1: đủ cho pH / turbidity
    FILL_LEVEL_HIGH = 1  // GPIO4 mức 2: đủ cho TDS
};

struct PendingCommand {
    CommandType type = CMD_STATUS;
    SensorType sensor = SENSOR_TEMP;
    String pumpTarget = "inlet";
    bool pumpState = false;
    FillLevel fillLevel = FILL_LEVEL_LOW;
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
    void enqueueFill(FillLevel level);
    void enqueueDrain();
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

    SensorType currentSensor = SENSOR_PH;
    FillLevel currentFillLevel = FILL_LEVEL_LOW;

    SensorResult currentResult;

    unsigned long stateTimer = 0;
    unsigned long floatTargetSince = 0;
    uint8_t fillFailCount = 0;

    bool manualInletActive = false;
    bool manualDrainActive = false;
    unsigned long manualInletSince = 0;
    unsigned long manualDrainSince = 0;
    unsigned long floatHighSince = 0;

    const unsigned long STABILIZE_TIME = 500;
    const unsigned long MAX_FILL_TIME = 60000;
    const unsigned long DRAIN_FIXED_TIME = 30000; // xả cố định 30s, không dùng phao cạn
    const unsigned long FLOAT_DEBOUNCE_TIME = 800;
    const unsigned long PUMP_FLOAT_GRACE_MS = 1000;
    const unsigned long MAX_MANUAL_PUMP_TIME = 60000;
    static const uint8_t MAX_FILL_FAILS = 3;

    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_VALVE_PIN = 19;
    const uint8_t FLOAT_HIGH_PIN = 4;   // phao mức 2 → TDS
    const uint8_t FLOAT_LOW_PIN = 17;   // phao mức 1 → pH / turbidity
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_ACTIVE_LEVEL = LOW;

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
    void pushFill(FillLevel level);
    void pushMeasure(SensorType sensor);
    void pushDrain();

    void handleStateIdle();
    void handleStateFilling(unsigned long elapsed);
    void handleStateStabilizing(unsigned long elapsed);
    void handleStateMeasuring(unsigned long elapsed);
    void handleStateDraining(unsigned long elapsed);
    void handleStatePublishing();
    void handleManualPumpTimeouts();

    bool isWaterLow() const;
    bool isWaterHigh() const;
    bool isFillTargetReached() const;
    bool isInletOn() const;
    bool isDrainOn() const;
    void logFloatPins(const char *why) const;
    void setInletPump(bool enabled);
    void setDrainValve(bool enabled);
    void setSensorPower(SensorType type, bool enabled);
    void powerOffAllSensors();
    RawReading readRawADC(uint8_t pin, int sampleCount);
    int medianFilter(int *buffer, int count) const;
    String rawReadingJson(const RawReading &reading) const;
    const char *fillLevelName(FillLevel level) const;

    void emitEvent(const String &stage, const String &message, const String &extraJson = "");

    bool parseSensorName(const String &name, std::vector<SensorType> &outList);
    void expandSensorTokens(const std::vector<String> &sensors, std::vector<SensorType> &outList);
    String getSensorName(SensorType type) const;
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
