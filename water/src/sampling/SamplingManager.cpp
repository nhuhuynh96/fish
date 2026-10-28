#include "SamplingManager.h"

#include <ArduinoJson.h>

#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"
#include "../mqtt/MQTTHandler.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin() {
    pinMode(INLET_PUMP_PIN, OUTPUT);
    digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(DRAIN_PUMP_PIN, OUTPUT);
    digitalWrite(DRAIN_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(FLOAT_PIN, INPUT_PULLUP);

    floatRaw = readFloatRaw();
    floatFull = floatRaw;
    floatRawSince = millis();

    LOGLN("[Sampling] Relay Bơm nạp=GPIO18 | Bơm xả=GPIO19 | Phao=GPIO17");
    LOGF("[Phao] lúc khởi động GPIO17=%d (%s)\n", digitalRead(FLOAT_PIN), floatFull ? "đầy" : "chưa đầy");
}

String SamplingManager::getStateName() const {
    if (activeJob == JOB_INLET) return "FILLING_WATER";
    if (activeJob == JOB_DRAIN) return "DRAINING_WATER";
    return "IDLE";
}

void SamplingManager::emitEvent(const String &stage, const String &message) {
    LOGF("[Sampling Event] [%s] %s\n", stage.c_str(), message.c_str());
    if (!mqttHandler.isConnected()) return;

    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = stage;
    doc["state"] = getStateName();
    doc["message"] = message;
    doc["float"] = floatFull;
    doc["level"] = floatFull ? 1 : 0;
    doc["inlet_on"] = isInletOn();
    doc["drain_on"] = isDrainOn();
    if (activeJob != JOB_NONE) {
        doc["until_float"] = jobUntilFloat;
        doc["duration_s"] = (unsigned long)(jobDurationMs / 1000);
    }
    doc["gpio17"] = (int)digitalRead(FLOAT_PIN);
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

bool SamplingManager::readFloatRaw() const {
    return digitalRead(FLOAT_PIN) == FLOAT_ACTIVE;
}

// Phao chỉ đổi trạng thái khi chân giữ nguyên đủ FLOAT_DEBOUNCE_MS; bơm chạy làm dây phao nhiễu.
void SamplingManager::updateFloat() {
    bool raw = readFloatRaw();
    unsigned long now = millis();
    if (raw != floatRaw) {
        floatRaw = raw;
        floatRawSince = now;
        return;
    }
    if (raw == floatFull || now - floatRawSince < FLOAT_DEBOUNCE_MS) return;
    floatFull = raw;
    emitEvent("float_changed", floatFull ? "Phao đầy." : "Phao tắt.");
}

void SamplingManager::publishWaterLevel() {
    emitEvent("water_level", floatFull ? "Phao đầy." : "Phao chưa đầy.");
}

bool SamplingManager::isInletOn() const {
    return digitalRead(INLET_PUMP_PIN) == PUMP_ON_LEVEL;
}

bool SamplingManager::isDrainOn() const {
    return digitalRead(DRAIN_PUMP_PIN) == PUMP_ON_LEVEL;
}

void SamplingManager::setInletPump(bool enabled) {
    if (enabled) digitalWrite(DRAIN_PUMP_PIN, PUMP_OFF_LEVEL);
    digitalWrite(INLET_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    LOGLN(enabled ? "[Pump] BẬT bơm nạp (GPIO18)" : "[Pump] TẮT bơm nạp (GPIO18)");
}

void SamplingManager::setDrainPump(bool enabled) {
    if (enabled) digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    digitalWrite(DRAIN_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    LOGLN(enabled ? "[Pump] BẬT bơm xả (GPIO19)" : "[Pump] TẮT bơm xả (GPIO19)");
}

void SamplingManager::stopHardware() {
    setInletPump(false);
    setDrainPump(false);
}

// Tắt bơm trước khi gửi để inlet_on/drain_on trong sự kiện là false.
void SamplingManager::stopJob(const String &stage, const String &message) {
    stopHardware();
    activeJob = JOB_NONE;
    jobUntilFloat = false;
    jobStartedMs = 0;
    jobDurationMs = 0;
    emitEvent(stage, message);
}

void SamplingManager::stopAll(const String &stage, const String &message) {
    stopJob(stage, message);
}

static String secondsText(uint32_t durationMs) {
    return String((unsigned long)(durationMs / 1000)) + "s";
}

void SamplingManager::startInlet(bool untilFull, uint32_t durationMs) {
    if (activeJob != JOB_NONE) {
        emitEvent("command_error", "Đang chạy bơm, gửi stop trước khi bơm nạp.");
        return;
    }
    if (untilFull && floatFull) {
        emitEvent("already_full", "Phao đã đầy. Không bơm.");
        return;
    }
    setInletPump(true);
    activeJob = JOB_INLET;
    jobUntilFloat = untilFull;
    jobStartedMs = millis();
    jobDurationMs = durationMs;
    emitEvent("filling", untilFull ? "Bơm nạp: BẬT tới khi phao đầy, tối đa " + secondsText(durationMs) + "."
                                   : "Bơm nạp: BẬT trong " + secondsText(durationMs) + ".");
}

void SamplingManager::startDrain(bool untilEmpty, uint32_t durationMs) {
    if (activeJob != JOB_NONE) {
        emitEvent("command_error", "Đang chạy bơm, gửi stop trước khi xả.");
        return;
    }
    if (untilEmpty && !floatFull) {
        emitEvent("already_empty", "Phao đã tắt. Không xả.");
        return;
    }
    setDrainPump(true);
    activeJob = JOB_DRAIN;
    jobUntilFloat = untilEmpty;
    jobStartedMs = millis();
    jobDurationMs = durationMs;
    emitEvent("draining", untilEmpty ? "Bơm xả: BẬT tới khi phao tắt, tối đa " + secondsText(durationMs) + "."
                                     : "Bơm xả: BẬT trong " + secondsText(durationMs) + ".");
}

void SamplingManager::inletOff() {
    if (activeJob == JOB_INLET) {
        stopJob("pump_off", "Đã tắt bơm nạp.");
        return;
    }
    if (isInletOn()) setInletPump(false);
    emitEvent("pump_off", "Bơm nạp đã tắt.");
}

void SamplingManager::drainOff() {
    if (activeJob == JOB_DRAIN) {
        stopJob("drain_off", "Đã tắt bơm xả.");
        return;
    }
    if (isDrainOn()) setDrainPump(false);
    emitEvent("drain_off", "Bơm xả đã tắt.");
}

void SamplingManager::handlePump() {
    if (activeJob == JOB_NONE) return;
    bool inlet = activeJob == JOB_INLET;

    if (jobUntilFloat) {
        if (inlet && floatFull) {
            stopJob("filled", "Phao đầy. Tắt bơm nạp.");
            return;
        }
        if (!inlet && !floatFull) {
            stopJob("drained", "Phao tắt. Tắt bơm xả.");
            return;
        }
    }

    if (millis() - jobStartedMs >= jobDurationMs) {
        String seconds = secondsText(jobDurationMs);
        stopJob("time_up", inlet ? "Hết " + seconds + ". Tắt bơm nạp." : "Hết " + seconds + ". Tắt bơm xả.");
    }
}

void SamplingManager::handle() {
    bool connected = mqttHandler.isConnected();
    if (mqttWasConnected && !connected && (activeJob != JOB_NONE || isInletOn() || isDrainOn())) {
        LOGLN("[Sampling] Mất MQTT, tắt cả hai bơm.");
        stopHardware();
        activeJob = JOB_NONE;
        jobUntilFloat = false;
    }
    mqttWasConnected = connected;

    updateFloat();
    handlePump();
}
