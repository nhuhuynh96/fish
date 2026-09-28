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

    // Khởi tạo UART2 kết nối với ESP32-S3 (TinyGo Sensor Node)
    Serial2.begin(115200, SERIAL_8N1, S3_UART_RX_PIN, S3_UART_TX_PIN);

    LOGLN("[Sampling] Relay Bơm=GPIO18 | Van Xả=GPIO19 | Phao1=GPIO17 | Phao2=GPIO4");
    LOGF("[Sampling] UART Bridge ESP32-S3: RX=GPIO%d, TX=GPIO%d (115200 Baud)\n", S3_UART_RX_PIN, S3_UART_TX_PIN);
    LOGF("[Phao] lúc khởi động GPIO17=%d (%s) GPIO4=%d (%s)\n",
         digitalRead(FLOAT_LOW_PIN), isWaterLow() ? "phao1 ĐỦ" : "phao1 chưa",
         digitalRead(FLOAT_HIGH_PIN), isWaterHigh() ? "phao2 ĐỦ" : "phao2 chưa");
}

String SamplingManager::getStateName() const {
    if (isInletOn()) return "FILLING_WATER";
    if (isDrainOn()) return "DRAINING_WATER";
    if (waitingForS3Response) return "MEASURING_SENSORS";
    if (!testQueue.empty()) return "TESTING";
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
    payload += "\"inlet_on\":" + String(isInletOn() ? "true" : "false") + ",";
    payload += "\"drain_on\":" + String(isDrainOn() ? "true" : "false") + ",";
    payload += "\"float_low\":" + String(isWaterLow() ? "true" : "false") + ",";
    payload += "\"float_high\":" + String(isWaterHigh() ? "true" : "false") + ",";
    payload += "\"float_full\":" + String(isWaterHigh() ? "true" : "false") + ",";
    payload += "\"float_empty\":" + String((!isWaterLow() && !isWaterHigh()) ? "true" : "false") + ",";
    payload += "\"gpio17\":" + String((int)digitalRead(FLOAT_LOW_PIN)) + ",";
    payload += "\"gpio4\":" + String((int)digitalRead(FLOAT_HIGH_PIN));
    if (extraJson.length() > 0) {
        payload += "," + extraJson;
    }
    payload += "}";
    mqttHandler.publish("event", payload);
}

bool SamplingManager::isWaterLow() const {
    return digitalRead(FLOAT_LOW_PIN) == FLOAT_LOW_ACTIVE;
}

bool SamplingManager::isWaterHigh() const {
    if (!isWaterLow()) return false;
    return digitalRead(FLOAT_HIGH_PIN) == FLOAT_HIGH_ACTIVE;
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
    waitingForS3Response = false;
}

FillLevel SamplingManager::requiredLevel(const String &action, FillLevel parsed) const {
    if (action == "ph") return FILL_LEVEL_LOW;
    if (action == "tds") return FILL_LEVEL_HIGH;
    return parsed;
}

bool SamplingManager::isTestAction(const String &action) const {
    return action == "test" || action == "test_ph" || action == "test_temp" ||
           action == "test_temperature" || action == "test_tds" || action == "measure_all";
}

void SamplingManager::enqueue(const PendingCommand &cmd) {
    PendingCommand next = cmd;
    next.action.toLowerCase();
    next.action.trim();
    if (isTestAction(next.action)) {
        testQueue.push(next);
        LOGF("[Sampling] Test queue +%s (test=%u)\n",
             next.action.c_str(), (unsigned)testQueue.size());
        emitEvent("queued", "Đã xếp đo thử " + next.action + " (không bơm)");
        return;
    }
    next.fillLevel = requiredLevel(next.action, next.fillLevel);
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
    size_t n = commandQueue.size() + testQueue.size();
    while (!commandQueue.empty()) commandQueue.pop();
    while (!testQueue.empty()) testQueue.pop();
    inletAttempt = 0;
    stopHardware();
    if (emit) {
        emitEvent("queue_cleared", "Đã xóa toàn bộ hàng đợi (" + String((unsigned)n) + " lệnh).");
    }
}

void SamplingManager::applyHeadFillLevel() {
    if (commandQueue.empty()) return;
    const PendingCommand &head = commandQueue.front();
    fillLevel = requiredLevel(head.action, head.fillLevel);
}

void SamplingManager::hanldeInletOn(FillLevel level) {
    if (isInletOn()) {
        emitEvent("manual_pump", "Bơm nạp đã bật. Không bơm nữa.");
        return;
    }
    if (level == FILL_LEVEL_LOW) {
        if (isWaterLow()) {
            emitEvent("manual_pump", "Đủ phao 1. Không bơm nữa.");
            return;
        }
    }
    if (level == FILL_LEVEL_HIGH) {
        if (isWaterHigh()) {
            emitEvent("manual_pump", "Đủ phao 2. Không bơm nữa.");
            return;
        }
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

void SamplingManager::sendS3Command(const String &action) {
    measureStartedAt = millis();
    waitingForS3Response = true;
    s3RequestTimeout = millis();
    LOGF("[UART -> S3] Gửi lệnh: CMD:%s\n", action.c_str());
    Serial2.printf("CMD:%s\n", action.c_str());
}

void SamplingManager::handleS3Incoming() {
    while (Serial2.available()) {
        String line = Serial2.readStringUntil('\n');
        line.trim();
        if (line.length() == 0) continue;

        LOGF("[UART <- S3] %s\n", line.c_str());

        if (line.startsWith("DATA:")) {
            String jsonPayload = line.substring(5);
            LOGLN("\n[Sampling] >>> NHẬN sensor_data TỪ ESP32-S3 <<<");
            LOGLN(jsonPayload);

            if (mqttHandler.isConnected()) {
                mqttHandler.publish("sensor_data", jsonPayload);
            }
            emitEvent("measuring_sensor", "Đã nhận kết quả đo từ ESP32-S3, đã gửi MQTT.");
            waitingForS3Response = false;
            finishCommand();
        } else if (line.startsWith("EVENT:")) {
            String eventJson = line.substring(6);
            emitEvent("s3_event", "ESP32-S3 Event", eventJson);
        } else if (line.startsWith("ERROR:")) {
            String errJson = line.substring(6);
            emitEvent("s3_error", "ESP32-S3 Báo lỗi", errJson);
            waitingForS3Response = false;
            finishCommand();
        }
    }

    // Kiểm tra timeout nếu S3 không phản hồi
    if (waitingForS3Response && (millis() - s3RequestTimeout > S3_RESPONSE_TIMEOUT_MS)) {
        LOGLN("[UART S3] Quá thời gian chờ phản hồi từ ESP32-S3!");
        emitEvent("command_error", "ESP32-S3 Timeout không phản hồi đo!");
        waitingForS3Response = false;
        finishCommand();
    }
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
            if (isWaterHigh()) {
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

        if (onFor >= INLET_FIXED_TIME) {
            if (inletAttempt >= INLET_ATTEMPTS) {
                setInletPump(false);
                inletAttempt = 0;
                inletSince = 0;
                emitEvent("manual_pump_timeout", "Bơm 3 lần 30s chưa tới mực. Đã ngưng.");
                return;
            }
            if (inletAttempt < INLET_ATTEMPTS) {
                inletAttempt++;
                inletSince = now;
                floatLowSince = 0;
                floatHighSince = 0;
                emitEvent("manual_pump", "Chưa đủ mực. Thử lại lần " + String((unsigned)inletAttempt) + "/" + String((unsigned)INLET_ATTEMPTS) + ".");
            }
        }
    }

    if (isDrainOn()) {
        if (now - drainSince >= DRAIN_FIXED_TIME) {
            setDrainValve(false);
            drainSince = 0;
            emitEvent("drained", "Đã xả 30s (hết queue). IDLE.");
        }
    }

    if (commandQueue.empty() && isWaterLow() && !isInletOn() && !isDrainOn() && !waitingForS3Response) {
        setDrainValve(true);
        drainSince = now;
        emitEvent("draining", "Hết queue. Xả 30 giây.");
    }
}

void SamplingManager::finishCommand() {
    if (!commandQueue.empty()) {
        commandQueue.pop();
    }
    inletAttempt = 0;
}

bool SamplingManager::measureAfterFill(FillLevel level, const String &action) {
    if (waitingForS3Response) return false;

    bool pumping = isInletOn();
    bool full = (level == FILL_LEVEL_LOW) ? isWaterLow() : isWaterHigh();

    if (pumping) {
        unsigned long now = millis();
        if (now - inletSince < PUMP_FLOAT_GRACE_MS) return false;
        unsigned long since = (level == FILL_LEVEL_LOW) ? floatLowSince : floatHighSince;
        full = full && since != 0 && (now - since >= FLOAT_DEBOUNCE_TIME);
    }

    if (full) {
        setInletPump(false);
        sendS3Command(action);
        return false; // Chờ S3 trả dữ liệu trong handleS3Incoming() mới finishCommand
    }

    if (!pumping) {
        hanldeInletOn(level);
    }
    return false;
}

void SamplingManager::processHead() {
    if (commandQueue.empty() || waitingForS3Response) return;

    PendingCommand cmd = commandQueue.front();
    String action = cmd.action;

    if (action == "inlet_on") {
        hanldeInletOn(cmd.fillLevel);
        finishCommand();
        return;
    }
    if (action == "drain_on") {
        hanldeDrainOn();
        finishCommand();
        return;
    }
    if (action == "ph") {
        measureAfterFill(FILL_LEVEL_LOW, "ph");
        return;
    }
    if (action == "temp" || action == "temperature") {
        sendS3Command("temp");
        return;
    }
    if (action == "tds") {
        measureAfterFill(FILL_LEVEL_HIGH, "tds");
        return;
    }

    emitEvent("command_error", "action không hỗ trợ: " + action);
    finishCommand();
}

void SamplingManager::processTestHead() {
    if (testQueue.empty() || waitingForS3Response) return;

    PendingCommand cmd = testQueue.front();
    testQueue.pop();
    String action = cmd.action;
    LOGLN("[Test] " + action + " — Gửi lệnh đo trực tiếp sang ESP32-S3");
    sendS3Command(action);
}

void SamplingManager::handle() {
    handleS3Incoming();
    while (!testQueue.empty() && !waitingForS3Response) {
        processTestHead();
    }
    handlePump();
    processHead();
}
