#include "SamplingManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin() {
    pinMode(INLET_PUMP_PIN, OUTPUT);
    digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(DRAIN_PUMP_PIN, OUTPUT);
    digitalWrite(DRAIN_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(FLOAT_HIGH_PIN, INPUT_PULLUP);
    pinMode(FLOAT_LOW_PIN, INPUT_PULLUP);
    clearLocked(false);

    LOGLN("[Sampling] Relay Bơm nạp=GPIO18 | Bơm xả=GPIO19 | Phao1=GPIO17 | Phao2=GPIO4");
    LOGF("[Phao] lúc khởi động GPIO17=%d (%s) GPIO4=%d (%s) level=%d\n",
         digitalRead(FLOAT_LOW_PIN), float1Active() ? "phao1 ĐỦ" : "phao1 chưa",
         digitalRead(FLOAT_HIGH_PIN), float2Active() ? "phao2 ĐỦ" : "phao2 chưa",
         currentLevel());
}

String SamplingManager::getStateName() const {
    if (activeJob == JOB_INLET) return "FILLING_WATER";
    if (activeJob == JOB_DRAIN) return "DRAINING_WATER";
    if (!commandQueue.empty()) return "BUSY";
    return "IDLE";
}

String SamplingManager::targetName(uint8_t target) const {
    if (target == 0) return "theo thời gian";
    return "phao " + String((unsigned)target);
}

void SamplingManager::emitEvent(const String &stage, const String &message, uint8_t target, uint32_t durationMs) {
    LOGF("[Sampling Event] [%s] %s\n", stage.c_str(), message.c_str());
    if (!mqttHandler.isConnected()) return;

    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"stage\":\"" + stage + "\",";
    payload += "\"state\":\"" + getStateName() + "\",";
    payload += "\"message\":\"" + message + "\",";
    payload += "\"level\":" + String(currentLevel()) + ",";
    payload += "\"target\":" + String((unsigned)target) + ",";
    payload += "\"duration_s\":" + String((unsigned long)(durationMs / 1000)) + ",";
    payload += "\"inlet_on\":" + String(isInletOn() ? "true" : "false") + ",";
    payload += "\"drain_on\":" + String(isDrainOn() ? "true" : "false") + ",";
    payload += "\"float_1\":" + String(float1Active() ? "true" : "false") + ",";
    payload += "\"float_2\":" + String(float2Active() ? "true" : "false") + ",";
    payload += "\"gpio17\":" + String((int)digitalRead(FLOAT_LOW_PIN)) + ",";
    payload += "\"gpio4\":" + String((int)digitalRead(FLOAT_HIGH_PIN));
    payload += "}";
    mqttHandler.publish("event", payload);
}

bool SamplingManager::float1Active() const {
    return digitalRead(FLOAT_LOW_PIN) == FLOAT_LOW_ACTIVE;
}

bool SamplingManager::float2Active() const {
    return digitalRead(FLOAT_HIGH_PIN) == FLOAT_HIGH_ACTIVE;
}

bool SamplingManager::floatActive(uint8_t n) const {
    if (n == 1) return float1Active();
    if (n == 2) return float2Active();
    return false;
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
    return digitalRead(DRAIN_PUMP_PIN) == PUMP_ON_LEVEL;
}

void SamplingManager::setInletPump(bool enabled) {
    if (enabled) {
        digitalWrite(DRAIN_PUMP_PIN, PUMP_OFF_LEVEL);
    }
    digitalWrite(INLET_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    LOGLN(enabled ? "[Pump] BẬT bơm nạp (GPIO18)" : "[Pump] TẮT bơm nạp (GPIO18)");
}

void SamplingManager::setDrainPump(bool enabled) {
    if (enabled) {
        digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    }
    digitalWrite(DRAIN_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    LOGLN(enabled ? "[Pump] BẬT bơm xả (GPIO19)" : "[Pump] TẮT bơm xả (GPIO19)");
}

void SamplingManager::stopHardware() {
    setInletPump(false);
    setDrainPump(false);
}

// Tắt bơm trước khi gửi để inlet_on/drain_on trong sự kiện là false.
void SamplingManager::stopJob(const String &stage, const String &message) {
    uint8_t target = jobTarget;
    uint32_t durationMs = jobDurationMs;
    stopHardware();
    activeJob = JOB_NONE;
    jobTarget = 0;
    jobStartedMs = 0;
    jobDurationMs = 0;
    targetSince = 0;
    emitEvent(stage, message, target, durationMs);
}

void SamplingManager::enqueue(const PendingCommand &cmd) {
    PendingCommand next = cmd;
    next.action.toLowerCase();
    next.action.trim();
    commandQueue.push(next);
    LOGF("[Sampling] Queue +%s %s %lus (queue=%u)\n",
         next.action.c_str(), targetName(next.target).c_str(),
         (unsigned long)(next.durationMs / 1000), (unsigned)commandQueue.size());
    emitEvent("queued", "Đã xếp " + next.action + " " + targetName(next.target), next.target, next.durationMs);
}

void SamplingManager::clearQueue() {
    clearLocked(true);
}

void SamplingManager::clearLocked(bool emit) {
    size_t n = commandQueue.size();
    while (!commandQueue.empty()) commandQueue.pop();
    stopHardware();
    activeJob = JOB_NONE;
    jobTarget = 0;
    jobStartedMs = 0;
    jobDurationMs = 0;
    targetSince = 0;
    if (emit) {
        emitEvent("queue_cleared", "Đã xóa toàn bộ hàng đợi (" + String((unsigned)n) + " lệnh).");
    }
}

void SamplingManager::hanldeInletOff() {
    if (activeJob == JOB_INLET) {
        stopJob("pump_off", "Đã tắt bơm nạp.");
        return;
    }
    if (isInletOn()) setInletPump(false);
    emitEvent("pump_off", "Bơm nạp đã tắt.");
}

void SamplingManager::hanldeDrainOff() {
    if (activeJob == JOB_DRAIN) {
        stopJob("drain_off", "Đã tắt bơm xả.");
        return;
    }
    if (isDrainOn()) setDrainPump(false);
    emitEvent("drain_off", "Bơm xả đã tắt.");
}

void SamplingManager::hanldeInletOn(uint8_t target, uint32_t durationMs) {
    String seconds = String((unsigned long)(durationMs / 1000)) + "s";
    if (target != 0 && floatActive(target)) {
        emitEvent("already_at_level", "Đủ " + targetName(target) + ". Không bơm.", target, durationMs);
        return;
    }
    setInletPump(true);
    activeJob = JOB_INLET;
    jobTarget = target;
    jobStartedMs = millis();
    jobDurationMs = durationMs;
    targetSince = 0;
    if (target == 0) {
        emitEvent("filling", "Bơm nạp: BẬT trong " + seconds + ".", target, durationMs);
    } else {
        emitEvent("filling", "Bơm nạp: BẬT tới " + targetName(target) + ", tối đa " + seconds + ".", target, durationMs);
    }
}

void SamplingManager::hanldeDrainOn(uint8_t target, uint32_t durationMs) {
    String seconds = String((unsigned long)(durationMs / 1000)) + "s";
    if (target != 0 && !floatActive(target)) {
        emitEvent("already_at_level", targetName(target) + " đã tắt. Không xả.", target, durationMs);
        return;
    }
    setDrainPump(true);
    activeJob = JOB_DRAIN;
    jobTarget = target;
    jobStartedMs = millis();
    jobDurationMs = durationMs;
    targetSince = 0;
    if (target == 0) {
        emitEvent("draining", "Bơm xả: BẬT trong " + seconds + ".", target, durationMs);
    } else {
        emitEvent("draining", "Bơm xả: BẬT tới khi " + targetName(target) + " tắt, tối đa " + seconds + ".", target, durationMs);
    }
}

void SamplingManager::handlePump() {
    if (activeJob == JOB_NONE) return;

    unsigned long now = millis();
    unsigned long elapsed = now - jobStartedMs;
    bool inlet = activeJob == JOB_INLET;

    if (jobTarget != 0 && elapsed >= PUMP_FLOAT_GRACE_MS) {
        bool reached = inlet ? floatActive(jobTarget) : !floatActive(jobTarget);
        if (reached) {
            if (targetSince == 0) targetSince = now;
            if (now - targetSince >= FLOAT_DEBOUNCE_TIME) {
                if (inlet) {
                    stopJob("filled", "Đủ " + targetName(jobTarget) + ". Tắt bơm nạp.");
                } else {
                    stopJob("drained", targetName(jobTarget) + " đã tắt. Tắt bơm xả.");
                }
                return;
            }
        } else {
            targetSince = 0;
        }
    }

    if (elapsed >= jobDurationMs) {
        String seconds = String((unsigned long)(jobDurationMs / 1000)) + "s";
        stopJob("time_up", inlet ? "Hết " + seconds + ". Tắt bơm nạp." : "Hết " + seconds + ". Tắt bơm xả.");
    }
}

void SamplingManager::processHead() {
    if (activeJob != JOB_NONE || commandQueue.empty()) return;

    PendingCommand cmd = commandQueue.front();
    commandQueue.pop();

    if (cmd.action == "inlet_on" || cmd.action == "inlet") {
        hanldeInletOn(cmd.target, cmd.durationMs);
        return;
    }
    if (cmd.action == "drain_on") {
        hanldeDrainOn(cmd.target, cmd.durationMs);
        return;
    }

    emitEvent("command_error", "action không hỗ trợ: " + cmd.action);
}

void SamplingManager::handle() {
    handlePump();
    processHead();
}
