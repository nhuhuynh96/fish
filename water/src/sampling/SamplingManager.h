#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>

enum ActiveJob {
    JOB_NONE = 0,
    JOB_INLET,
    JOB_DRAIN
};

// Một phao, hai bơm. Firmware chỉ làm theo lệnh; backend quyết định khi nào bơm, xả.
// Firmware vẫn tự tắt bơm khi tới phao (nếu lệnh yêu cầu), hết duration, hoặc mất MQTT.
class SamplingManager {
public:
    SamplingManager();
    void begin();
    void handle();

    void startInlet(bool untilFull, uint32_t durationMs);
    void startDrain(bool untilEmpty, uint32_t durationMs);
    void inletOff();
    void drainOff();
    void stopAll(const String &stage, const String &message);
    void publishWaterLevel();
    String getStateName() const;
    bool isFull() const { return floatFull; }

private:
    ActiveJob activeJob = JOB_NONE;
    bool jobUntilFloat = false;
    unsigned long jobStartedMs = 0;
    unsigned long jobDurationMs = 0;

    bool floatFull = false;
    bool floatRaw = false;
    unsigned long floatRawSince = 0;
    bool mqttWasConnected = false;

    const unsigned long FLOAT_DEBOUNCE_MS = 800;

    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_PUMP_PIN = 19;
    const uint8_t FLOAT_PIN = 17;
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_ACTIVE = LOW;

    bool readFloatRaw() const;
    void updateFloat();
    void handlePump();
    void stopJob(const String &stage, const String &message);
    void stopHardware();

    bool isInletOn() const;
    bool isDrainOn() const;
    void setInletPump(bool enabled);
    void setDrainPump(bool enabled);

    void emitEvent(const String &stage, const String &message);
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
