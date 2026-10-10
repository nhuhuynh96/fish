#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>
#include <queue>

// target: 0 = chỉ chạy theo thời gian, 1 = phao 1 (GPIO17), 2 = phao 2 (GPIO4)
struct PendingCommand {
    String action = "";
    uint8_t target = 0;
    uint32_t durationMs = 60000;
};

enum ActiveJob {
    JOB_NONE = 0,
    JOB_INLET,
    JOB_DRAIN
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

    ActiveJob activeJob = JOB_NONE;
    uint8_t jobTarget = 0;
    unsigned long jobStartedMs = 0;
    unsigned long jobDurationMs = 0;
    unsigned long targetSince = 0;

    const unsigned long FLOAT_DEBOUNCE_TIME = 800;
    const unsigned long PUMP_FLOAT_GRACE_MS = 1000;

    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_PUMP_PIN = 19;

    const uint8_t FLOAT_HIGH_PIN = 4;   // phao 2
    const uint8_t FLOAT_LOW_PIN = 17;   // phao 1
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_LOW_ACTIVE = LOW;
    const uint8_t FLOAT_HIGH_ACTIVE = LOW;

    void processHead();
    void clearLocked(bool emit);
    void stopHardware();
    void stopJob(const String &stage, const String &message);

    void hanldeInletOn(uint8_t target, uint32_t durationMs);
    void hanldeDrainOn(uint8_t target, uint32_t durationMs);
    void handlePump();

    bool float1Active() const;
    bool float2Active() const;
    bool floatActive(uint8_t n) const;
    int currentLevel() const;
    bool isInletOn() const;
    bool isDrainOn() const;
    void setInletPump(bool enabled);
    void setDrainPump(bool enabled);
    String targetName(uint8_t target) const;

    void emitEvent(const String &stage, const String &message, uint8_t target = 0, uint32_t durationMs = 0);
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
