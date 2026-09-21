#include "SamplingManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin() {
    currentState = STATE_IDLE;
    stateTimer = millis();
    fillFailCount = 0;
    clearCommandQueue();

    pinMode(INLET_PUMP_PIN, OUTPUT);
    digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(DRAIN_VALVE_PIN, OUTPUT);
    digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
    pinMode(FLOAT_HIGH_PIN, INPUT_PULLUP);
    pinMode(FLOAT_LOW_PIN, INPUT_PULLUP);
    LOGLN("[Sampling] Relay: bơm=GPIO18, van=GPIO19");
    LOGLN("[Sampling] Phao: mức 1 pH+turb=GPIO17, mức 2 TDS=GPIO4 | Xả cố định 30s");

    pinMode(TDS_SENSOR_PIN, INPUT);
    analogReadResolution(12);
    analogSetPinAttenuation(TDS_SENSOR_PIN, ADC_11db);
    LOGLN("[Sampling] TDS Meter V1.0 gắn GPIO35 (ADC1).");

    pinMode(TURBIDITY_SENSOR_PIN, INPUT);
    analogSetPinAttenuation(TURBIDITY_SENSOR_PIN, ADC_11db);
    LOGLN("[Sampling] Turbidity analog gắn GPIO34 (ADC1).");

    pinMode(PH_SENSOR_PIN, INPUT);
    analogSetPinAttenuation(PH_SENSOR_PIN, ADC_11db);
    LOGLN("[Sampling] PH-4502C gắn GPIO32 (Po). V+ dùng 5V, hiệu chuẩn POT gần BNC.");

    pinMode(PH_POWER_PIN, OUTPUT);
    pinMode(TURBIDITY_POWER_PIN, OUTPUT);
    pinMode(TDS_POWER_PIN, OUTPUT);
    powerOffAllSensors();
    LOGLN("[Sampling] Power: pH=GPIO14, Turbidity=GPIO33, TDS=GPIO25 (2N3904).");
    LOGLN("[Sampling] FSM: MEASURE tự xếp FILL mức 1 → pH/turb → FILL mức 2 → TDS → DRAIN.");
}

String SamplingManager::getStateName() const {
    switch (currentState) {
        case STATE_IDLE: return "IDLE";
        case STATE_FILLING: return "FILLING_WATER";
        case STATE_STABILIZING: return "STABILIZING";
        case STATE_MEASURING: return "MEASURING";
        case STATE_DRAINING: return "DRAINING_WATER";
        case STATE_PUBLISHING: return "PUBLISHING";
        default: return "UNKNOWN";
    }
}

String SamplingManager::getSensorName(SensorType type) const {
    switch (type) {
        case SENSOR_TEMP: return "temperature";
        case SENSOR_PH: return "ph";
        case SENSOR_TURBIDITY: return "turbidity";
        case SENSOR_TDS: return "tds";
        default: return "unknown";
    }
}

const char *SamplingManager::fillLevelName(FillLevel level) const {
    return level == FILL_LEVEL_HIGH ? "mức 2/TDS" : "mức 1/pH+turb";
}

bool SamplingManager::parseSensorName(const String &name, std::vector<SensorType> &outList) {
    String n = name;
    n.toLowerCase();
    n.trim();

    if (n == "all" || n == "tat_ca") {
        outList.push_back(SENSOR_PH);
        outList.push_back(SENSOR_TURBIDITY);
        outList.push_back(SENSOR_TDS);
        return true;
    } else if (n == "temp" || n == "temperature" || n == "nhiet_do") {
        outList.push_back(SENSOR_TEMP);
        return true;
    } else if (n == "ph") {
        outList.push_back(SENSOR_PH);
        return true;
    } else if (n == "turbidity" || n == "turb" || n == "do_duc" || n == "do_can") {
        outList.push_back(SENSOR_TURBIDITY);
        return true;
    } else if (n == "tds" || n == "chat_ran" || n == "chat_luong") {
        outList.push_back(SENSOR_TDS);
        return true;
    }
    return false;
}

void SamplingManager::expandSensorTokens(const std::vector<String> &sensors, std::vector<SensorType> &outList) {
    for (const String &raw : sensors) {
        String token = raw;
        token.trim();

        int start = 0;
        while (start <= (int)token.length()) {
            int comma = token.indexOf(',', start);
            String part = (comma < 0) ? token.substring(start) : token.substring(start, comma);
            part.trim();
            if (part.length() > 0) {
                parseSensorName(part, outList);
            }
            if (comma < 0) break;
            start = comma + 1;
        }
    }

    std::vector<SensorType> unique;
    for (SensorType t : outList) {
        bool exists = false;
        for (SensorType u : unique) {
            if (u == t) { exists = true; break; }
        }
        if (!exists) unique.push_back(t);
    }
    outList.swap(unique);
}

void SamplingManager::emitEvent(const String &stage, const String &message, const String &extraJson) {
    LOGF("[Sampling Event] [%s] %s\n", stage.c_str(), message.c_str());

    if (mqttHandler.isConnected()) {
        String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
        payload += "\"stage\":\"" + stage + "\",";
        payload += "\"state\":\"" + getStateName() + "\",";
        payload += "\"message\":\"" + message + "\",";
        payload += "\"inlet_on\":" + String(isInletOn() ? "true" : "false") + ",";
        payload += "\"drain_on\":" + String(isDrainOn() ? "true" : "false") + ",";
        payload += "\"float_low\":" + String(isWaterLow() ? "true" : "false") + ",";
        payload += "\"float_high\":" + String(isWaterHigh() ? "true" : "false") + ",";
        // giữ tên cũ cho FE/log cũ
        payload += "\"float_full\":" + String(isWaterHigh() ? "true" : "false") + ",";
        payload += "\"float_empty\":" + String(isWaterLow() ? "true" : "false");
        if (extraJson.length() > 0) {
            payload += "," + extraJson;
        }
        payload += "}";
        mqttHandler.publish("event", payload);
    }
}

bool SamplingManager::isWaterLow() const {
    return digitalRead(FLOAT_LOW_PIN) == FLOAT_ACTIVE_LEVEL;
}

bool SamplingManager::isWaterHigh() const {
    return digitalRead(FLOAT_HIGH_PIN) == FLOAT_ACTIVE_LEVEL;
}

bool SamplingManager::isFillTargetReached() const {
    if (currentFillLevel == FILL_LEVEL_HIGH) {
        return isWaterHigh();
    }
    // Mức 1: phao GPIO17 HOẶC đã vượt lên mức 2 (GPIO4)
    return isWaterLow() || isWaterHigh();
}

bool SamplingManager::isInletOn() const {
    return digitalRead(INLET_PUMP_PIN) == PUMP_ON_LEVEL;
}

bool SamplingManager::isDrainOn() const {
    return digitalRead(DRAIN_VALVE_PIN) == PUMP_ON_LEVEL;
}

void SamplingManager::logFloatPins(const char *why) const {
    int highPin = digitalRead(FLOAT_HIGH_PIN);
    int lowPin = digitalRead(FLOAT_LOW_PIN);
    LOGF(
        "[Phao] %s | GPIO17 mức1=%s (%s) | GPIO4 mức2=%s (%s)\n",
        why,
        lowPin == LOW ? "LOW/đóng" : "HIGH/mở",
        isWaterLow() ? "firmware=ĐỦ_MỨC_1" : "firmware=chưa mức 1",
        highPin == LOW ? "LOW/đóng" : "HIGH/mở",
        isWaterHigh() ? "firmware=ĐỦ_MỨC_2" : "firmware=chưa mức 2"
    );
}

void SamplingManager::setInletPump(bool enabled) {
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

void SamplingManager::powerOffAllSensors() {
    digitalWrite(PH_POWER_PIN, SENSOR_POWER_OFF);
    digitalWrite(TURBIDITY_POWER_PIN, SENSOR_POWER_OFF);
    digitalWrite(TDS_POWER_PIN, SENSOR_POWER_OFF);
}

void SamplingManager::setSensorPower(SensorType type, bool enabled) {
    if (enabled) {
        powerOffAllSensors();
    }

    uint8_t pin = 0;
    const char *name = "";
    switch (type) {
        case SENSOR_PH:
            pin = PH_POWER_PIN;
            name = "pH";
            break;
        case SENSOR_TURBIDITY:
            pin = TURBIDITY_POWER_PIN;
            name = "turbidity";
            break;
        case SENSOR_TDS:
            pin = TDS_POWER_PIN;
            name = "tds";
            break;
        default:
            return;
    }

    digitalWrite(pin, enabled ? SENSOR_POWER_ON : SENSOR_POWER_OFF);
    LOGF("[Power] %s %s (GPIO%u)\n", name, enabled ? "BẬT" : "TẮT", pin);

    if (enabled) {
        delay(SENSOR_POWER_SETTLE_MS);
    }
}

void SamplingManager::transitionTo(SamplingState newState) {
    if (currentState == STATE_FILLING && newState != STATE_FILLING) {
        setInletPump(false);
    }

    if (currentState == STATE_DRAINING && newState != STATE_DRAINING) {
        setDrainValve(false);
    }

    if (newState == STATE_PUBLISHING) {
        powerOffAllSensors();
        setInletPump(false);
    }

    if (newState == STATE_IDLE) {
        powerOffAllSensors();
    }

    currentState = newState;
    stateTimer = millis();

    if (newState == STATE_FILLING) {
        floatTargetSince = 0;
        manualInletActive = false;
        setInletPump(!isFillTargetReached());
        LOGF("[Sampling] FILL mục tiêu mức %s\n", fillLevelName(currentFillLevel));
    }

    if (newState == STATE_DRAINING) {
        manualDrainActive = false;
        setDrainValve(true);
    }
}

void SamplingManager::pushFill(FillLevel level) {
    PendingCommand fillCmd;
    fillCmd.type = CMD_FILL;
    fillCmd.fillLevel = level;
    commandQueue.push(fillCmd);
    LOGF("[Sampling] Queue +FILL %s (queue=%u)\n",
         fillLevelName(level), (unsigned)commandQueue.size());
}

void SamplingManager::pushMeasure(SensorType sensor) {
    PendingCommand cmd;
    cmd.type = CMD_MEASURE;
    cmd.sensor = sensor;
    commandQueue.push(cmd);
    LOGF(
        "[Sampling] Queue +MEASURE %s (queue=%u)\n",
        getSensorName(sensor).c_str(),
        (unsigned)commandQueue.size()
    );
}

void SamplingManager::pushDrain() {
    PendingCommand drainCmd;
    drainCmd.type = CMD_DRAIN;
    commandQueue.push(drainCmd);
    LOGF("[Sampling] Queue +DRAIN 30s (queue=%u)\n", (unsigned)commandQueue.size());
}

void SamplingManager::enqueueFill(FillLevel level) {
    pushFill(level);
    emitEvent("queued", String("Đã xếp FILL mức ") + fillLevelName(level) + " vào queue.");
}

void SamplingManager::enqueueDrain() {
    pushDrain();
    emitEvent("queued", "Đã xếp DRAIN 30s vào queue.");
}

void SamplingManager::enqueueMeasure(const std::vector<String> &sensors) {
    std::vector<SensorType> list;
    expandSensorTokens(sensors, list);

    if (list.empty()) {
        LOGLN("[Sampling] MEASURE không có cảm biến hợp lệ, bỏ qua.");
        emitEvent("command_error", "sensors không hợp lệ.");
        return;
    }

    fillFailCount = 0;

    bool hasLow = false;
    bool hasTds = false;
    for (SensorType t : list) {
        if (t == SENSOR_TDS) {
            hasTds = true;
        } else {
            hasLow = true;
        }
    }

    // Phao mức 1 (GPIO17): dừng khi báo đủ → đo pH / turbidity (và temp nếu có)
    if (hasLow) {
        pushFill(FILL_LEVEL_LOW);
        for (SensorType t : list) {
            if (t != SENSOR_TDS) {
                pushMeasure(t);
            }
        }
    }

    // Có TDS thì bơm tiếp tới phao mức 2 (GPIO4) rồi mới đo
    if (hasTds) {
        pushFill(FILL_LEVEL_HIGH);
        pushMeasure(SENSOR_TDS);
    }

    pushDrain();

    emitEvent(
        "queued",
        "Đã xếp chu trình đo: " +
            String(hasLow ? "FILL mức 1 → pH/turb" : "") +
            String(hasLow && hasTds ? " → " : "") +
            String(hasTds ? "FILL mức 2 → TDS" : "") +
            " → DRAIN."
    );
}

void SamplingManager::enqueuePump(const String &target, bool state) {
    String t = target;
    t.toLowerCase();
    t.trim();
    if (t != "inlet" && t != "drain") {
        LOGLN("[Sampling] PUMP target không hợp lệ (chỉ inlet|drain), bỏ qua.");
        emitEvent("command_error", "pump target phải là inlet hoặc drain.");
        return;
    }

    PendingCommand cmd;
    cmd.type = CMD_PUMP;
    cmd.pumpTarget = t;
    cmd.pumpState = state;
    commandQueue.push(cmd);
    LOGF("[Sampling] Queue +PUMP %s=%s (queue=%u)\n",
         t.c_str(), state ? "ON" : "OFF", (unsigned)commandQueue.size());
    emitEvent(
        "queued",
        String("Đã xếp PUMP ") + t + "=" + (state ? "ON" : "OFF") + " vào queue."
    );
}

void SamplingManager::enqueueStatus() {
    PendingCommand cmd;
    cmd.type = CMD_STATUS;
    commandQueue.push(cmd);
    LOGF("[Sampling] Queue +STATUS (queue=%u)\n", (unsigned)commandQueue.size());
}

void SamplingManager::clearQueue() {
    size_t dropped = commandQueue.size();
    clearCommandQueue();
    fillFailCount = 0;
    manualInletActive = false;
    manualDrainActive = false;
    powerOffAllSensors();
    setInletPump(false);
    setDrainValve(false);
    transitionTo(STATE_IDLE);
    LOGF("[Sampling] CLEAR QUEUE: đã hủy %u lệnh. IDLE.\n", (unsigned)dropped);
    emitEvent(
        "queue_cleared",
        "Đã xóa toàn bộ hàng đợi (" + String((unsigned)dropped) +
            " lệnh). Tắt bơm/van, về IDLE."
    );
}

void SamplingManager::publishStatus() {
    String statusJson = "{";
    statusJson += "\"device_id\":\"" + currentConfig.device_id + "\",";
    statusJson += "\"state\":\"" + getStateName() + "\",";
    statusJson += "\"is_busy\":" + String(isBusy() ? "true" : "false") + ",";
    statusJson += "\"queue_size\":" + String((unsigned)commandQueue.size()) + ",";
    statusJson += "\"float_low\":" + String(isWaterLow() ? "true" : "false") + ",";
    statusJson += "\"float_high\":" + String(isWaterHigh() ? "true" : "false");
    statusJson += "}";

    LOGLN("[Sampling] Status: " + statusJson);
    if (mqttHandler.isConnected()) {
        mqttHandler.publish("status", statusJson);
    }
}

void SamplingManager::processCommandQueue() {
    if (currentState != STATE_IDLE || commandQueue.empty()) {
        return;
    }

    PendingCommand cmd = commandQueue.front();
    commandQueue.pop();

    switch (cmd.type) {
        case CMD_MEASURE:
            LOGF("[Sampling] Chạy MEASURE [%s] (còn lại queue=%u)\n",
                          getSensorName(cmd.sensor).c_str(),
                          (unsigned)commandQueue.size());
            startMeasureCycle(cmd.sensor);
            break;
        case CMD_FILL:
            currentFillLevel = cmd.fillLevel;
            if (isFillTargetReached()) {
                LOGF("[Sampling] FILL %s: đã đủ mức, bỏ qua bơm.\n", fillLevelName(cmd.fillLevel));
                emitEvent(
                    "filled",
                    String("Đã đủ mức ") + fillLevelName(cmd.fillLevel) + ". Bỏ qua bơm, tiếp tục đo."
                );
                processCommandQueue();
            } else {
                LOGF("[Sampling] Chạy FILL mức %s.\n", fillLevelName(cmd.fillLevel));
                transitionTo(STATE_FILLING);
                emitEvent(
                    "filling",
                    String("Bơm nạp tới mức ") + fillLevelName(cmd.fillLevel) + "."
                );
            }
            break;
        case CMD_DRAIN:
            LOGLN("[Sampling] >>> Chạy DRAIN cố định 30s (không phao cạn) <<<");
            transitionTo(STATE_DRAINING);
            logFloatPins("bắt đầu xả 30s");
            emitEvent("draining", "Xả nước cố định 30 giây.");
            break;
        case CMD_PUMP:
            LOGLN("[Sampling] Chạy PUMP từ hàng đợi");
            setManualPump(cmd.pumpTarget, cmd.pumpState);
            break;
        case CMD_STATUS:
            LOGLN("[Sampling] Chạy STATUS từ hàng đợi");
            publishStatus();
            break;
    }
}

void SamplingManager::startMeasureCycle(SensorType sensor) {
    currentSensor = sensor;
    currentResult = SensorResult();
    currentResult.startTime = millis();

    LOGF("\n[Sampling] >>> BẮT ĐẦU ĐO [%s] <<<\n", getSensorName(sensor).c_str());
    emitEvent("stabilizing", "Bắt đầu đo cảm biến.");
    transitionTo(STATE_STABILIZING);
}

void SamplingManager::setManualPump(const String &target, bool state) {
    String t = target;
    t.toLowerCase();
    t.trim();

    if (t == "inlet") {
        if (state && isWaterHigh()) {
            setInletPump(false);
            manualInletActive = false;
            logFloatPins("từ chối bật bơm nạp");
            LOGLN("[Sampling] [Manual] Từ chối bật bơm: phao cao đã đủ.");
            emitEvent("manual_pump", "Không bật bơm vì phao mức cao đang báo đủ.");
            return;
        }

        setInletPump(state);
        manualInletActive = state;
        if (state) {
            manualInletSince = millis();
            floatHighSince = 0;
            logFloatPins("bật bơm nạp");
            LOGF("[Sampling] [Manual] Bơm nạp: BẬT (tắt khi phao cao hoặc sau %lus)\n",
                 MAX_MANUAL_PUMP_TIME / 1000UL);
        } else {
            LOGLN("[Sampling] [Manual] Bơm nạp: TẮT");
        }
        emitEvent("manual_pump", String("Bơm nạp nước: ") + (state ? "BẬT" : "TẮT"));
        return;
    }

    if (t == "drain") {
        // Không dùng phao cạn: bật/tắt thủ công, tự tắt sau timeout
        setDrainValve(state);
        manualDrainActive = state;
        if (state) {
            manualDrainSince = millis();
            logFloatPins("bật van xả thủ công");
            LOGF("[Sampling] [Manual] Van xả: BẬT (tự tắt sau %lus, không phao cạn)\n",
                 DRAIN_FIXED_TIME / 1000UL);
        } else {
            LOGLN("[Sampling] [Manual] Van xả: TẮT");
        }
        emitEvent("manual_pump", String("Van xả nước: ") + (state ? "BẬT" : "TẮT"));
        return;
    }

    LOGLN("[Sampling] [Manual] target không hợp lệ (chỉ inlet|drain).");
    emitEvent("command_error", "pump target phải là inlet hoặc drain.");
}

void SamplingManager::handleManualPumpTimeouts() {
    if (manualInletActive) {
        unsigned long onFor = millis() - manualInletSince;
        if (onFor < PUMP_FLOAT_GRACE_MS) {
            floatHighSince = 0;
        } else if (isWaterHigh()) {
            if (floatHighSince == 0) {
                floatHighSince = millis();
            }
            if (millis() - floatHighSince >= FLOAT_DEBOUNCE_TIME) {
                setInletPump(false);
                manualInletActive = false;
                floatHighSince = 0;
                logFloatPins("phao cao ổn định → tắt bơm nạp");
                LOGLN("[Sampling] [Manual] Phao mức cao → tắt bơm nạp.");
                emitEvent("manual_pump", "Phao mức cao. Đã tự tắt bơm nạp.");
            }
        } else {
            floatHighSince = 0;
        }

        if (manualInletActive && onFor >= MAX_MANUAL_PUMP_TIME) {
            setInletPump(false);
            manualInletActive = false;
            floatHighSince = 0;
            logFloatPins("hết 60s → tắt bơm nạp");
            LOGLN("[Sampling] [Manual] Hết 60s → tắt bơm nạp (an toàn).");
            emitEvent("manual_pump_timeout", "Bơm nạp thủ công quá 60s. Đã tự tắt.");
        }
    }

    if (manualDrainActive) {
        unsigned long onFor = millis() - manualDrainSince;
        if (onFor >= DRAIN_FIXED_TIME) {
            setDrainValve(false);
            manualDrainActive = false;
            LOGLN("[Sampling] [Manual] Hết 30s → tắt van xả.");
            emitEvent("manual_pump_timeout", "Van xả thủ công đủ 30s. Đã tự tắt.");
        }
    }
}

void SamplingManager::measureSensor(SensorType type) {
    switch (type) {
        case SENSOR_TEMP: {
            LOGLN("[Sampling] [Temp] Bỏ qua: chưa có cảm biến nhiệt độ.");
            emitEvent("measuring_sensor", "Bỏ qua nhiệt độ (chưa có sensor phần cứng).");
            break;
        }
        case SENSOR_PH: {
            LOGLN("[Sampling] [PH-4502C] -> Bật transistor GPIO14, đọc ADC GPIO32...");
            setSensorPower(SENSOR_PH, true);
            currentResult.phRaw = readRawADC(PH_SENSOR_PIN, PH_SAMPLE_COUNT);
            setSensorPower(SENSOR_PH, false);
            currentResult.hasPh = true;
            publishSensorReading(SENSOR_PH);
            break;
        }
        case SENSOR_TURBIDITY: {
            LOGLN("[Sampling] [Turbidity] -> Bật transistor GPIO33, đọc ADC GPIO34...");
            setSensorPower(SENSOR_TURBIDITY, true);
            currentResult.turbidityRaw = readRawADC(TURBIDITY_SENSOR_PIN, TURBIDITY_SAMPLE_COUNT);
            setSensorPower(SENSOR_TURBIDITY, false);
            currentResult.hasTurbidity = true;
            publishSensorReading(SENSOR_TURBIDITY);
            break;
        }
        case SENSOR_TDS: {
            LOGLN("[Sampling] [TDS] -> Bật transistor GPIO25, đọc ADC GPIO35...");
            setSensorPower(SENSOR_TDS, true);
            currentResult.tdsRaw = readRawADC(TDS_SENSOR_PIN, TDS_SAMPLE_COUNT);
            setSensorPower(SENSOR_TDS, false);
            currentResult.hasTds = true;
            publishSensorReading(SENSOR_TDS);
            break;
        }
    }
}

void SamplingManager::publishSensorReading(SensorType type) {
    unsigned long durationMs = millis() - currentResult.startTime;

    String payload = "{";
    payload += "\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"timestamp\":" + String(millis() / 1000) + ",";
    payload += "\"status\":\"success\",";
    payload += "\"duration_ms\":" + String(durationMs) + ",";
    payload += "\"sensors_measured\":[\"" + getSensorName(type) + "\"],";
    payload += "\"raw\":{";
    if (type == SENSOR_PH) {
        payload += "\"ph\":" + rawReadingJson(currentResult.phRaw);
    } else if (type == SENSOR_TURBIDITY) {
        payload += "\"turbidity\":" + rawReadingJson(currentResult.turbidityRaw);
    } else if (type == SENSOR_TDS) {
        payload += "\"tds\":" + rawReadingJson(currentResult.tdsRaw);
    }
    payload += "}";
    payload += "}";

    LOGLN("\n[Sampling] >>> GỬI sensor_data NGAY <<<");
    LOGLN(payload);

    if (mqttHandler.isConnected()) {
        mqttHandler.publish("sensor_data", payload);
    }

    String extra;
    if (type == SENSOR_PH) {
        extra = "\"sensor\":\"ph\",\"adc\":" + String(currentResult.phRaw.adc) +
                ",\"voltage\":" + String(currentResult.phRaw.voltage, 3);
        emitEvent("measuring_sensor", "Đã đo pH raw, đã gửi MQTT.", extra);
    } else if (type == SENSOR_TURBIDITY) {
        extra = "\"sensor\":\"turbidity\",\"adc\":" + String(currentResult.turbidityRaw.adc) +
                ",\"voltage\":" + String(currentResult.turbidityRaw.voltage, 3);
        emitEvent("measuring_sensor", "Đã đo turbidity raw, đã gửi MQTT.", extra);
    } else if (type == SENSOR_TDS) {
        extra = "\"sensor\":\"tds\",\"adc\":" + String(currentResult.tdsRaw.adc) +
                ",\"voltage\":" + String(currentResult.tdsRaw.voltage, 3);
        emitEvent("measuring_sensor", "Đã đo TDS raw, đã gửi MQTT.", extra);
    }
}

void SamplingManager::finishCycle() {
    transitionTo(STATE_IDLE);
    if (commandQueue.empty()) {
        emitEvent("idle", "Đã xử lý xong. Queue rỗng. Sẵn sàng.");
    } else {
        emitEvent("idle", "Đã xử lý xong. Còn lệnh trong queue, chạy tiếp...");
        processCommandQueue();
    }
}

void SamplingManager::clearCommandQueue() {
    while (!commandQueue.empty()) commandQueue.pop();
}

void SamplingManager::handleStateIdle() {
    processCommandQueue();
}

void SamplingManager::handleStateFilling(unsigned long elapsed) {
    if (elapsed < PUMP_FLOAT_GRACE_MS) {
        floatTargetSince = 0;
    } else if (isFillTargetReached()) {
        if (floatTargetSince == 0) {
            floatTargetSince = millis();
        }
        if (millis() - floatTargetSince >= FLOAT_DEBOUNCE_TIME) {
            fillFailCount = 0;
            logFloatPins("chu trình đo: đủ mức mục tiêu");
            LOGF("[Sampling] [Phao] Đủ mức %s. Tắt bơm.\n", fillLevelName(currentFillLevel));
            transitionTo(STATE_IDLE);
            emitEvent(
                "filled",
                String("Đã đủ mức ") + fillLevelName(currentFillLevel) + ". Tắt bơm, tiếp tục đo."
            );
            processCommandQueue();
            return;
        }
    } else {
        floatTargetSince = 0;
    }

    if (elapsed >= MAX_FILL_TIME) {
        fillFailCount++;
        LOGF("[Sampling] LỖI: Bơm quá %lus chưa tới mức %s.\n",
             MAX_FILL_TIME / 1000UL, fillLevelName(currentFillLevel));
        powerOffAllSensors();

        if (fillFailCount >= MAX_FILL_FAILS) {
            size_t dropped = commandQueue.size();
            clearCommandQueue();
            fillFailCount = 0;
            LOGF(
                "[Sampling] ABORT: timeout bơm %u lần → hủy %u lệnh còn lại.\n",
                (unsigned)MAX_FILL_FAILS,
                (unsigned)dropped
            );
            emitEvent(
                "fill_abort",
                "Bơm/phao lỗi: timeout " + String(MAX_FILL_FAILS) +
                    " lần liên tiếp. Đã hủy " + String((unsigned)dropped) +
                    " lệnh còn lại trong queue."
            );
            transitionTo(STATE_IDLE);
            return;
        }

        emitEvent(
            "fill_timeout",
            "Quá thời gian nạp nước (" + String(fillFailCount) + "/" +
                String(MAX_FILL_FAILS) + "). Hủy lệnh FILL hiện tại, chạy lệnh queue tiếp theo."
        );
        transitionTo(STATE_IDLE);
        processCommandQueue();
    }
}

void SamplingManager::handleStateStabilizing(unsigned long elapsed) {
    if (elapsed >= STABILIZE_TIME) {
        transitionTo(STATE_MEASURING);
        measureSensor(currentSensor);
        powerOffAllSensors();
        LOGF("[Sampling] Đã đo xong [%s], đã gửi MQTT.\n", getSensorName(currentSensor).c_str());
        emitEvent("measuring_complete", "Đã gửi kết quả. Tiếp tục chu trình...");
        transitionTo(STATE_PUBLISHING);
    }
}

void SamplingManager::handleStateMeasuring(unsigned long /*elapsed*/) {
}

void SamplingManager::handleStateDraining(unsigned long elapsed) {
    if (elapsed >= DRAIN_FIXED_TIME) {
        LOGLN("[Sampling] Xả đủ 30s. Tắt van.");
        transitionTo(STATE_IDLE);
        emitEvent("drained", "Đã xả đủ 30 giây. Tắt van.");
        if (commandQueue.empty()) {
            emitEvent("idle", "Đã xả xong. Queue rỗng. Sẵn sàng.");
        } else {
            emitEvent("idle", "Đã xả xong. Còn lệnh trong queue, chạy tiếp...");
            processCommandQueue();
        }
    }
}

void SamplingManager::handleStatePublishing() {
    finishCycle();
}

void SamplingManager::handle() {
    handleManualPumpTimeouts();

    unsigned long elapsed = millis() - stateTimer;

    switch (currentState) {
        case STATE_IDLE:
            handleStateIdle();
            break;
        case STATE_FILLING:
            handleStateFilling(elapsed);
            break;
        case STATE_STABILIZING:
            handleStateStabilizing(elapsed);
            break;
        case STATE_MEASURING:
            handleStateMeasuring(elapsed);
            break;
        case STATE_DRAINING:
            handleStateDraining(elapsed);
            break;
        case STATE_PUBLISHING:
            handleStatePublishing();
            break;
    }
}

int SamplingManager::medianFilter(int *buffer, int count) const {
    for (int i = 1; i < count; i++) {
        int key = buffer[i];
        int j = i - 1;
        while (j >= 0 && buffer[j] > key) {
            buffer[j + 1] = buffer[j];
            j--;
        }
        buffer[j + 1] = key;
    }
    return buffer[count / 2];
}

RawReading SamplingManager::readRawADC(uint8_t pin, int sampleCount) {
    const int maxSamples = 40;
    if (sampleCount > maxSamples) sampleCount = maxSamples;
    int samples[maxSamples];

    for (int i = 0; i < sampleCount; i++) {
        samples[i] = analogRead(pin);
        delay(20);
    }

    RawReading reading;
    reading.sampleCount = sampleCount;
    reading.adc = medianFilter(samples, sampleCount);
    reading.voltage = (reading.adc * ADC_VREF) / (float)TDS_ADC_MAX;

    LOGF("[RAW] GPIO%u | ADC=%d | V=%.3fV | samples=%d\n",
        pin, reading.adc, reading.voltage, sampleCount);

    return reading;
}

String SamplingManager::rawReadingJson(const RawReading &reading) const {
    String json = "{";
    json += "\"adc\":" + String(reading.adc) + ",";
    json += "\"voltage\":" + String(reading.voltage, 3) + ",";
    json += "\"sample_count\":" + String(reading.sampleCount);
    json += "}";
    return json;
}
