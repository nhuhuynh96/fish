#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>
#include <queue>

enum FillLevel {
    FILL_LEVEL_LOW = 1,  // phao 1 — GPIO17
    FILL_LEVEL_HIGH = 2  // phao 2 — GPIO4
};

struct PendingCommand {
    String action = "";
    FillLevel fillLevel = FILL_LEVEL_LOW;
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
    void publishWaterLevel();
    size_t queueSize() const { return commandQueue.size(); }
    String getStateName() const;

private:
    std::queue<PendingCommand> commandQueue;

    FillLevel fillLevel = FILL_LEVEL_LOW;
    unsigned long inletSince = 0;
    uint8_t inletAttempt = 0;

    unsigned long drainSince = 0;
    unsigned long drainEmptySince = 0;
    unsigned long floatLowSince = 0;
    unsigned long floatHighSince = 0;

    const unsigned long DRAIN_SAFETY_TIME = 180000;
    const unsigned long INLET_FIXED_TIME = 30000;
    static const uint8_t INLET_ATTEMPTS = 2;

    const unsigned long FLOAT_DEBOUNCE_TIME = 800;
    const unsigned long PUMP_FLOAT_GRACE_MS = 1000;

    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_VALVE_PIN = 19;

    const uint8_t FLOAT_HIGH_PIN = 4;   // phao 2
    const uint8_t FLOAT_LOW_PIN = 17;   // phao 1
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_LOW_ACTIVE = LOW;
    const uint8_t FLOAT_HIGH_ACTIVE = LOW;

    void processHead();
    void finishCommand();
    void clearLocked(bool emit);
    void stopHardware();

    void applyHeadFillLevel();
    void hanldeInletOn(FillLevel level);
    void hanldeDrainOn();
    void handlePump();

    bool float1Active() const;
    bool float2Active() const;
    bool isWaterLow() const;
    int currentLevel() const;
    bool isInletOn() const;
    bool isDrainOn() const;
    void setInletPump(bool enabled);
    void setDrainValve(bool enabled);
    const char *fillLevelName(FillLevel level) const;

    void emitEvent(const String &stage, const String &message, const String &extraJson = "");
    void emitStage(const String &stage, const String &message, int target);
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
