#ifndef WATER_MANAGER_H
#define WATER_MANAGER_H

#include <Arduino.h>
#include <deque>

class WaterManager {
public:
    void begin();
    void handle();

    // level 1 = bơm tới phao 1, level 2 = bơm tới phao 2
    void enqueueInlet(uint8_t level);
    void inletOff();
    void drainOn();
    void drainOff();
    void clearQueue();
    void stopAll();

    String stateName() const;
    size_t queueSize() const { return queue.size(); }
    bool isInletOn() const { return inletRunning; }
    bool isDrainOn() const { return drainRunning; }
    bool float1() const;
    bool float2() const;

    void emitEvent(const String &stage, const String &message);

private:
    // GPIO19/20 là USB D-/D+ trên ESP32-S3 nên van xả dùng GPIO16
    static const uint8_t INLET_PIN = 18;
    static const uint8_t DRAIN_PIN = 16;
    static const uint8_t FLOAT1_PIN = 17;
    static const uint8_t FLOAT2_PIN = 4;
    static const uint8_t RELAY_ON = LOW;
    static const uint8_t RELAY_OFF = HIGH;

    static const unsigned long FLOAT_DEBOUNCE_MS = 800;
    static const unsigned long PUMP_GRACE_MS = 1000;
    static const unsigned long INLET_TIMEOUT_MS = 30000;
    static const unsigned long DRAIN_TIME_MS = 30000;
    static const uint8_t INLET_ATTEMPTS = 2;

    std::deque<uint8_t> queue;
    uint8_t fillLevel = 1;
    bool inletRunning = false;
    bool drainRunning = false;
    bool drainAfterQueue = false;
    uint8_t inletAttempt = 0;
    unsigned long inletSince = 0;
    unsigned long drainSince = 0;
    unsigned long floatSince = 0;

    void pumpTick(unsigned long now);
    void drainTick(unsigned long now);
    void queueTick(unsigned long now);
    void startInlet(unsigned long now);
    void popHead();
    void failHead(const String &message);
    void maybeDrain();
    bool atLevel(uint8_t level) const;
    void setInlet(bool on);
    void setDrain(bool on);
    static const char *levelName(uint8_t level);
};

extern WaterManager waterManager;

#endif // WATER_MANAGER_H
