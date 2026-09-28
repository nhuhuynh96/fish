#include "WaterManager.h"
#include <ArduinoJson.h>
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"

WaterManager waterManager;

void WaterManager::begin() {
    // Ghi mức OFF trước khi bật OUTPUT để relay không nháy lúc khởi động
    digitalWrite(INLET_PIN, RELAY_OFF);
    digitalWrite(DRAIN_PIN, RELAY_OFF);
    pinMode(INLET_PIN, OUTPUT);
    pinMode(DRAIN_PIN, OUTPUT);
    pinMode(FLOAT1_PIN, INPUT_PULLUP);
    pinMode(FLOAT2_PIN, INPUT_PULLUP);
    inletRunning = false;
    drainRunning = false;

    LOGF("[Water] Bơm nạp=GPIO%d | Van xả=GPIO%d | Phao1=GPIO%d | Phao2=GPIO%d\n",
         INLET_PIN, DRAIN_PIN, FLOAT1_PIN, FLOAT2_PIN);
    LOGF("[Water] Phao lúc khởi động: phao1=%s phao2=%s\n",
         float1() ? "ĐỦ" : "chưa", float2() ? "ĐỦ" : "chưa");
}

const char *WaterManager::levelName(uint8_t level) {
    return level == 2 ? "phao 2" : "phao 1";
}

bool WaterManager::float1() const {
    return digitalRead(FLOAT1_PIN) == LOW;
}

bool WaterManager::float2() const {
    if (!float1()) return false;
    return digitalRead(FLOAT2_PIN) == LOW;
}

bool WaterManager::atLevel(uint8_t level) const {
    return level == 2 ? float2() : float1();
}

String WaterManager::stateName() const {
    if (inletRunning) return "FILLING_WATER";
    if (drainRunning) return "DRAINING_WATER";
    if (!queue.empty()) return "BUSY";
    return "IDLE";
}

void WaterManager::emitEvent(const String &stage, const String &message) {
    LOGF("[Water Event] [%s] %s\n", stage.c_str(), message.c_str());
    if (!mqttHandler.isConnected()) return;

    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = stage;
    doc["state"] = stateName();
    doc["message"] = message;
    doc["inlet_on"] = inletRunning;
    doc["drain_on"] = drainRunning;
    doc["float_low"] = float1();
    doc["float_high"] = float2();
    doc["queue_size"] = (unsigned)queue.size();
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

void WaterManager::setInlet(bool on) {
    if (on) {
        digitalWrite(DRAIN_PIN, RELAY_OFF);
        drainRunning = false;
    }
    digitalWrite(INLET_PIN, on ? RELAY_ON : RELAY_OFF);
    inletRunning = on;
    floatSince = 0;
    LOGF("[Pump] %s bơm nạp (GPIO%d)\n", on ? "BẬT" : "TẮT", INLET_PIN);
}

void WaterManager::setDrain(bool on) {
    if (on) {
        digitalWrite(INLET_PIN, RELAY_OFF);
        inletRunning = false;
    }
    digitalWrite(DRAIN_PIN, on ? RELAY_ON : RELAY_OFF);
    drainRunning = on;
    LOGF("[Valve] %s van xả (GPIO%d)\n", on ? "BẬT" : "TẮT", DRAIN_PIN);
}

void WaterManager::enqueueInlet(uint8_t level) {
    if (level != 2) level = 1;
    queue.push_back(level);
    drainAfterQueue = true;
    emitEvent("queued", String("Đã xếp bơm nạp tới ") + levelName(level));
}

void WaterManager::inletOff() {
    if (!inletRunning) {
        emitEvent("manual_pump", "Bơm nạp đã tắt.");
        return;
    }
    setInlet(false);
    popHead();
    if (queue.empty()) drainAfterQueue = false;
    emitEvent("manual_pump", "Đã tắt bơm nạp, bỏ lệnh bơm đang chạy.");
}

void WaterManager::drainOn() {
    if (drainRunning) {
        emitEvent("draining", "Van xả đã bật.");
        return;
    }
    setDrain(true);
    drainSince = millis();
    emitEvent("draining", "Van xả: BẬT (tự tắt sau 30 giây)");
}

void WaterManager::drainOff() {
    if (!drainRunning) {
        emitEvent("draining", "Van xả đã tắt.");
        return;
    }
    setDrain(false);
    emitEvent("draining", "Đã tắt van xả.");
}

void WaterManager::stopAll() {
    if (inletRunning) setInlet(false);
    if (drainRunning) setDrain(false);
}

void WaterManager::clearQueue() {
    size_t n = queue.size();
    queue.clear();
    drainAfterQueue = false;
    inletAttempt = 0;
    stopAll();
    emitEvent("queue_cleared", "Đã xóa hàng đợi (" + String((unsigned)n) + " lệnh).");
}

void WaterManager::handle() {
    unsigned long now = millis();
    pumpTick(now);
    drainTick(now);
    queueTick(now);
}

void WaterManager::pumpTick(unsigned long now) {
    if (!inletRunning) return;

    unsigned long onFor = now - inletSince;
    if (onFor < PUMP_GRACE_MS) {
        floatSince = 0;
        return;
    }

    if (atLevel(fillLevel)) {
        if (floatSince == 0) floatSince = now;
        if (now - floatSince >= FLOAT_DEBOUNCE_MS) {
            setInlet(false);
            inletAttempt = 0;
            emitEvent("manual_pump", String("Đủ ") + levelName(fillLevel) + ". Tắt bơm.");
        }
        return;
    }
    floatSince = 0;

    if (onFor >= INLET_TIMEOUT_MS) {
        if (inletAttempt >= INLET_ATTEMPTS) {
            setInlet(false);
            failHead("Bơm " + String((unsigned)INLET_ATTEMPTS) + " lần 30s chưa tới mực. Đã ngưng.");
            return;
        }
        inletAttempt++;
        inletSince = now;
        emitEvent("manual_pump", "Chưa đủ mực. Thử lại lần " + String((unsigned)inletAttempt) + "/" +
                                     String((unsigned)INLET_ATTEMPTS) + ".");
    }
}

void WaterManager::drainTick(unsigned long now) {
    if (drainRunning && now - drainSince >= DRAIN_TIME_MS) {
        setDrain(false);
        emitEvent("drained", "Đã xả 30s. Tắt van xả.");
    }
}

void WaterManager::queueTick(unsigned long now) {
    if (queue.empty() || inletRunning || drainRunning) return;

    uint8_t level = queue.front();
    fillLevel = level;
    if (atLevel(level)) {
        popHead();
        maybeDrain();
        return;
    }
    startInlet(now);
}

void WaterManager::startInlet(unsigned long now) {
    setInlet(true);
    inletSince = now;
    if (inletAttempt == 0) inletAttempt = 1;
    emitEvent("manual_pump", String("Bơm nạp: BẬT (") + levelName(fillLevel) + ")");
}

void WaterManager::popHead() {
    if (!queue.empty()) queue.pop_front();
    inletAttempt = 0;
}

void WaterManager::failHead(const String &message) {
    popHead();
    if (queue.empty()) drainAfterQueue = false;
    emitEvent("manual_pump_timeout", message);
}

void WaterManager::maybeDrain() {
    if (!queue.empty() || !drainAfterQueue) return;
    drainAfterQueue = false;
    setDrain(true);
    drainSince = millis();
    emitEvent("draining", "Hết hàng đợi bơm. Xả 30 giây.");
}
