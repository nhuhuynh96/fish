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
    bool waitingForS3Response = false;
    unsigned long s3RequestTimeout = 0;

    const unsigned long DRAIN_FIXED_TIME = 30000;
    const unsigned long INLET_FIXED_TIME = 30000;
    static const uint8_t INLET_ATTEMPTS = 2;

    const unsigned long FLOAT_DEBOUNCE_TIME = 800;
    const unsigned long PUMP_FLOAT_GRACE_MS = 1000;
    const unsigned long S3_RESPONSE_TIMEOUT_MS = 15000;

    // Chân điều khiển Relay Bơm & Van xả
    const uint8_t INLET_PUMP_PIN = 18;
    const uint8_t DRAIN_VALVE_PIN = 19;

    // Chân đọc cảm biến phao nước
    const uint8_t FLOAT_HIGH_PIN = 4;   // phao 2 — mực cao (TDS)
    const uint8_t FLOAT_LOW_PIN = 17;   // phao 1 — mực thấp (pH)
    const uint8_t PUMP_ON_LEVEL = LOW;
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_LOW_ACTIVE = LOW;
    const uint8_t FLOAT_HIGH_ACTIVE = LOW;

    // Chân giao tiếp UART với ESP32-S3 (TinyGo Sensor Node)
    const uint8_t S3_UART_RX_PIN = 16;  // Nối vào TX (GPIO 43) của ESP32-S3
    const uint8_t S3_UART_TX_PIN = 23;  // Nối vào RX (GPIO 44) của ESP32-S3

    void processHead();
    void processTestHead();
    bool isTestAction(const String &action) const;
    void finishCommand();
    void clearLocked(bool emit);
    void stopHardware();

    FillLevel requiredLevel(const String &action, FillLevel parsed) const;
    void applyHeadFillLevel();
    void hanldeInletOn(FillLevel level);
    void hanldeDrainOn();
    void handlePump();
    bool measureAfterFill(FillLevel level, const String &action);

    void sendS3Command(const String &action);
    void handleS3Incoming();

    bool isWaterLow() const;
    bool isWaterHigh() const;
    bool isInletOn() const;
    bool isDrainOn() const;
    void setInletPump(bool enabled);
    void setDrainValve(bool enabled);
    const char *fillLevelName(FillLevel level) const;

    void emitEvent(const String &stage, const String &message, const String &extraJson = "");
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
