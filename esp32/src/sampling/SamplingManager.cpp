#include "SamplingManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin() {
    clearLocked(false);
    pinMode(INLET_PUMP_PIN, OUTPUT);
    digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(DRAIN_VALVE_PIN, OUTPUT);
    digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
    pinMode(FLOAT_HIGH_PIN, INPUT_PULLUP);
    pinMode(FLOAT_LOW_PIN, INPUT_PULLUP);

    LOGLN("[Sampling] Relay Bơm=GPIO18 | Van Xả=GPIO19 | Phao1=GPIO17 | Phao2=GPIO4");
    LOGF("[Phao] lúc khởi động GPIO17=%d (%s) GPIO4=%d (%s) level=%d\n",
         digitalRead(FLOAT_LOW_PIN), float1Active() ? "phao1 ĐỦ" : "phao1 chưa",
         digitalRead(FLOAT_HIGH_PIN), float2Active() ? "phao2 ĐỦ" : "phao2 chưa",
         currentLevel());
}

String SamplingManager::getStateName() const {
    if (isInletOn()) return "FILLING_WATER";
    if (isDrainOn()) return "DRAINING_WATER";
    if (!commandQueue.empty()) return "BUSY";
    return "IDLE";
}

const char *SamplingManager::fillLevelName(FillLevel level) const {
    return level == FILL_LEVEL_HIGH ? "phao 2" : "phao 1";
}

void SamplingManager::emitEvent(const String &stage, const String &message, const String &extraJson) {
    LOGF("[Sampling Event] [%s] %s\n", stage.c_str(), message.c_str());
    if (!mqttHandler.isConnected()) return;

    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"stage\":\"" + stage + "\",";
    payload += "\"state\":\"" + getStateName() + "\",";
    payload += "\"message\":\"" + message + "\",";
    payload += "\"level\":" + String(currentLevel()) + ",";
    payload += "\"inlet_on\":" + String(isInletOn() ? "true" : "false") + ",";
    payload += "\"drain_on\":" + String(isDrainOn() ? "true" : "false") + ",";
    payload += "\"float_1\":" + String(float1Active() ? "true" : "false") + ",";
    payload += "\"float_2\":" + String(float2Active() ? "true" : "false") + ",";
    payload += "\"gpio17\":" + String((int)digitalRead(FLOAT_LOW_PIN)) + ",";
    payload += "\"gpio4\":" + String((int)digitalRead(FLOAT_HIGH_PIN));
    if (extraJson.length() > 0) {
        payload += "," + extraJson;
    }
    payload += "}";
    mqttHandler.publish("event", payload);
}

bool SamplingManager::float1Active() const {
    return digitalRead(FLOAT_LOW_PIN) == FLOAT_LOW_ACTIVE;
}

bool SamplingManager::float2Active() const {
    return digitalRead(FLOAT_HIGH_PIN) == FLOAT_HIGH_ACTIVE;
}

bool SamplingManager::isWaterLow() const {
    return float1Active();
}

int SamplingManager::currentLevel() const {
    if (float2Active()) return 2;
    if (float1Active()) return 1;
    return 0;
}

void SamplingManager::publishWaterLevel() {
    int level = currentLevel();
    String message = "Mực nước level " + String(level);
    LOGF("[Phao] %s (float_1=%d float_2=%d)\n", message.c_str(), float1Active(), float2Active());
    if (!mqttHandler.isConnected()) return;

    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"stage\":\"water_level\",";
    payload += "\"level\":" + String(level) + ",";
    payload += "\"float_1\":" + String(float1Active() ? "true" : "false") + ",";
    payload += "\"float_2\":" + String(float2Active() ? "true" : "false") + ",";
    payload += "\"inlet_on\":" + String(isInletOn() ? "true" : "false") + ",";
    payload += "\"drain_on\":" + String(isDrainOn() ? "true" : "false");
    payload += "}";
    mqttHandler.publish("event", payload);
}

bool SamplingManager::isInletOn() const {
    return digitalRead(INLET_PUMP_PIN) == PUMP_ON_LEVEL;
}

bool SamplingManager::isDrainOn() const {
    return digitalRead(DRAIN_VALVE_PIN) == PUMP_ON_LEVEL;
}

void SamplingManager::setInletPump(bool enabled) {
    floatLowSince = 0;
    floatHighSince = 0;
    if (enabled) {
        digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
    }
    digitalWrite(INLET_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    LOGLN(enabled ? "[Pump] BẬT bơm nạp (GPIO18)" : "[Pump] TẮT bơm nạp (GPIO18)");
}

void SamplingManager::setDrainValve(bool enabled) {
    if (enabled) {
        digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    }
    digitalWrite(DRAIN_VALVE_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    LOGLN(enabled ? "[Valve] BẬT van xả (GPIO19)" : "[Valve] TẮT van xả (GPIO19)");
}

void SamplingManager::stopHardware() {
    setInletPump(false);
    setDrainValve(false);
}

void SamplingManager::enqueue(const PendingCommand &cmd) {
    PendingCommand next = cmd;
    next.action.toLowerCase();
    next.action.trim();
    commandQueue.push(next);
    LOGF("[Sampling] Queue +%s level=%s (queue=%u)\n",
         next.action.c_str(), fillLevelName(next.fillLevel), (unsigned)commandQueue.size());
    emitEvent("queued", "Đã xếp " + next.action + " " + String(fillLevelName(next.fillLevel)));
}

void SamplingManager::clearQueue() {
    clearLocked(true);
}

void SamplingManager::hanldeInletOff() {
    if (!isInletOn()) {
        emitEvent("manual_pump", "Bơm nạp đã tắt. Không tắt nữa.");
        return;
    }
    setInletPump(false);
    inletAttempt = 0;
    inletSince = 0;
    emitEvent("manual_pump", "Đã tắt bơm nạp.");
}

void SamplingManager::hanldeDrainOff() {
    if (!isDrainOn()) {
        emitEvent("draining", "Van xả đã tắt. Không tắt nữa.");
        return;
    }
    setDrainValve(false);
    drainSince = 0;
    emitEvent("draining", "Đã tắt van xả.");
}

void SamplingManager::clearLocked(bool emit) {
    size_t n = commandQueue.size();
    while (!commandQueue.empty()) commandQueue.pop();
    inletAttempt = 0;
    stopHardware();
    if (emit) {
        emitEvent("queue_cleared", "Đã xóa toàn bộ hàng đợi (" + String((unsigned)n) + " lệnh).");
    }
}

void SamplingManager::applyHeadFillLevel() {
    if (commandQueue.empty()) return;
    const PendingCommand &head = commandQueue.front();
    if (head.action == "inlet_on") {
        fillLevel = head.fillLevel;
    }
}

void SamplingManager::hanldeInletOn(FillLevel level) {
    if (isInletOn()) {
        fillLevel = level;
        floatLowSince = 0;
        floatHighSince = 0;
        emitEvent("manual_pump", String("Đổi mức dừng: ") + fillLevelName(level));
        return;
    }
    if (level == FILL_LEVEL_LOW && isWaterLow()) {
        emitEvent("manual_pump", "Đủ phao 1. Không bơm nữa.");
        return;
    }
    if (level == FILL_LEVEL_HIGH && float2Active()) {
        emitEvent("manual_pump", "Đủ phao 2. Không bơm nữa.");
        return;
    }
    setInletPump(true);
    inletSince = millis();
    fillLevel = level;
    inletAttempt = 0;
    emitEvent("manual_pump", String("Bơm nạp: BẬT (") + fillLevelName(fillLevel) + ")");
}

void SamplingManager::hanldeDrainOn() {
    if (isDrainOn()) {
        emitEvent("draining", "Van xả đã bật. Không xả nữa.");
        return;
    }
    setDrainValve(true);
    drainSince = millis();
    emitEvent("draining", "Van xả: BẬT");
}

void SamplingManager::handlePump() {
    applyHeadFillLevel();
    unsigned long now = millis();
    unsigned long onFor = now - inletSince;

    if (isInletOn()) {
        if (onFor < PUMP_FLOAT_GRACE_MS) {
            floatLowSince = 0;
            floatHighSince = 0;
        } else if (fillLevel == FILL_LEVEL_LOW) {
            if (isWaterLow()) {
                if (floatLowSince == 0) floatLowSince = now;
                if (now - floatLowSince >= FLOAT_DEBOUNCE_TIME) {
                    setInletPump(false);
                    inletAttempt = 0;
                    inletSince = 0;
                    emitEvent("manual_pump", "Đủ phao 1. Tắt bơm.");
                }
            } else {
                floatLowSince = 0;
            }
        } else if (fillLevel == FILL_LEVEL_HIGH) {
            if (float2Active()) {
                if (floatHighSince == 0) floatHighSince = now;
                if (now - floatHighSince >= FLOAT_DEBOUNCE_TIME) {
                    setInletPump(false);
                    inletAttempt = 0;
                    inletSince = 0;
                    emitEvent("manual_pump", "Đủ phao 2. Tắt bơm.");
                }
            } else {
                floatHighSince = 0;
            }
        }

        if (isInletOn() && onFor >= INLET_FIXED_TIME) {
            if (inletAttempt >= INLET_ATTEMPTS) {
                setInletPump(false);
                inletAttempt = 0;
                inletSince = 0;
                emitEvent("manual_pump_timeout", "Bơm 3 lần 30s chưa tới mực. Đã ngưng.");
                return;
            }
            inletAttempt++;
            inletSince = now;
            floatLowSince = 0;
            floatHighSince = 0;
            emitEvent("manual_pump", "Chưa đủ mực. Thử lại lần " + String((unsigned)inletAttempt) + "/" + String((unsigned)INLET_ATTEMPTS) + ".");
        }
    }

    if (isDrainOn() && now - drainSince >= DRAIN_FIXED_TIME) {
        setDrainValve(false);
        drainSince = 0;
        emitEvent("drained", "Đã xả 30 giây. Tắt van.");
    }
}

void SamplingManager::finishCommand() {
    if (!commandQueue.empty()) {
        commandQueue.pop();
    }
    inletAttempt = 0;
}

void SamplingManager::processHead() {
    if (commandQueue.empty()) return;

    PendingCommand cmd = commandQueue.front();
    String action = cmd.action;

    if (action == "inlet_on" || action == "inlet") {
        hanldeInletOn(cmd.fillLevel);
        finishCommand();
        return;
    }
    if (action == "drain_on") {
        hanldeDrainOn();
        finishCommand();
        return;
    }

    emitEvent("command_error", "action không hỗ trợ: " + action);
    finishCommand();
}

void SamplingManager::handle() {
    handlePump();
    processHead();
}
