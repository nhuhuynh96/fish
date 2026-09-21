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
    pinMode(FLOAT_FULL_PIN, INPUT_PULLUP);
    pinMode(FLOAT_EMPTY_PIN, INPUT_PULLUP);
    LOGLN("[Sampling] Relay: bơm=GPIO18, van=GPIO19 | Phao: đầy=GPIO4, cạn=GPIO17");

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
    LOGLN("[Sampling] FSM: mỗi lệnh đo = bơm nạp (đầu queue) → đo → xả (cuối queue).");
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

bool SamplingManager::parseSensorName(const String &name, std::vector<SensorType> &outList) {
    String n = name;
    n.toLowerCase();
    n.trim();

    // "all" = cảm biến có phần cứng (chưa có temp)
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
        payload += "\"float_full\":" + String(isWaterFull() ? "true" : "false") + ",";
        payload += "\"float_empty\":" + String(isWaterEmpty() ? "true" : "false");
        if (extraJson.length() > 0) {
            payload += "," + extraJson;
        }
        payload += "}";
        mqttHandler.publish("event", payload);
    }
}

bool SamplingManager::isWaterFull() const {
    return digitalRead(FLOAT_FULL_PIN) == FLOAT_FULL_LEVEL;
}

bool SamplingManager::isWaterEmpty() const {
    return digitalRead(FLOAT_EMPTY_PIN) == FLOAT_EMPTY_LEVEL;
}

bool SamplingManager::isInletOn() const {
    return digitalRead(INLET_PUMP_PIN) == PUMP_ON_LEVEL;
}

bool SamplingManager::isDrainOn() const {
    return digitalRead(DRAIN_VALVE_PIN) == PUMP_ON_LEVEL;
}

void SamplingManager::logFloatPins(const char *why) const {
    int fullPin = digitalRead(FLOAT_FULL_PIN);
    int emptyPin = digitalRead(FLOAT_EMPTY_PIN);
    LOGF(
        "[Phao] %s | GPIO4 đầy=%s (%s) | GPIO17 cạn=%s (%s)\n",
        why,
        fullPin == LOW ? "LOW/đóng" : "HIGH/mở",
        isWaterFull() ? "firmware=ĐẦY" : "firmware=chưa đầy",
        emptyPin == LOW ? "LOW/đóng" : "HIGH/mở",
        isWaterEmpty() ? "firmware=CẠN" : "firmware=chưa cạn"
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
        floatFullSince = 0;
        manualInletActive = false; // FSM FILLING quản lý bơm
        setInletPump(!isWaterFull());
    }

    if (newState == STATE_DRAINING) {
        floatEmptySince = 0;
        manualDrainActive = false;
        setDrainValve(!isWaterEmpty());
    }
}

void SamplingManager::enqueueMeasure(const std::vector<String> &sensors) {
    std::vector<SensorType> list;
    expandSensorTokens(sensors, list);

    if (list.empty()) {
        LOGLN("[Sampling] MEASURE không có cảm biến hợp lệ, bỏ qua.");
        emitEvent("command_error", "sensors không hợp lệ.");
        return;
    }

    // Lệnh đo mới từ người dùng → cho phép thử bơm lại sau khi đã abort
    fillFailCount = 0;

    PendingCommand fillCmd;
    fillCmd.type = CMD_FILL;
    commandQueue.push(fillCmd);
    LOGF("[Sampling] Queue +FILL (queue=%u)\n", (unsigned)commandQueue.size());

    for (SensorType t : list) {
        PendingCommand cmd;
        cmd.type = CMD_MEASURE;
        cmd.sensor = t;
        commandQueue.push(cmd);
        LOGF(
            "[Sampling] Queue +MEASURE %s (queue=%u)\n",
            getSensorName(t).c_str(),
            (unsigned)commandQueue.size()
        );
    }

    PendingCommand drainCmd;
    drainCmd.type = CMD_DRAIN;
    commandQueue.push(drainCmd);
    LOGF("[Sampling] Queue +DRAIN (queue=%u)\n", (unsigned)commandQueue.size());

    emitEvent(
        "queued",
        "Đã xếp bơm nạp → " + String((unsigned)list.size()) + " lệnh đo → xả vào commandQueue."
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
    emitEvent("queued", "Đã xếp lệnh bơm vào commandQueue.");
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
    statusJson += "\"queue_size\":" + String((unsigned)commandQueue.size());
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
            if (isWaterFull()) {
                LOGLN("[Sampling] FILL: phao đã đầy, bỏ qua bơm nạp.");
                emitEvent("filled", "Phao đã đầy. Bỏ qua bơm nạp, tiếp tục đo.");
                processCommandQueue();
            } else {
                LOGLN("[Sampling] Chạy FILL từ hàng đợi (đầu chu trình đo).");
                transitionTo(STATE_FILLING);
                emitEvent("filling", "Bắt đầu bơm nạp nước trước khi đo.");
            }
            break;
        case CMD_DRAIN:
            if (isWaterEmpty()) {
                LOGLN("[Sampling] DRAIN: phao đã cạn, bỏ qua xả.");
                emitEvent("drained", "Phao đã cạn. Bỏ qua xả nước.");
                processCommandQueue();
            } else {
                LOGLN("[Sampling] Chạy DRAIN từ hàng đợi (cuối chu trình đo).");
                transitionTo(STATE_DRAINING);
                emitEvent("draining", "Bắt đầu xả nước sau khi đo xong.");
            }
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

    LOGF("\n[Sampling] >>> BẮT ĐẦU ĐO [%s] (không kiểm tra mực nước) <<<\n",
         getSensorName(sensor).c_str());
    emitEvent("stabilizing", "Bắt đầu đo cảm biến (đã nạp nước ở đầu queue).");
    transitionTo(STATE_STABILIZING);
}

void SamplingManager::setManualPump(const String &target, bool state) {
    String t = target;
    t.toLowerCase();
    t.trim();

    if (t == "inlet") {
        if (state && isWaterFull()) {
            setInletPump(false);
            manualInletActive = false;
            logFloatPins("từ chối bật bơm nạp");
            LOGLN("[Sampling] [Manual] Từ chối bật bơm: phao đang báo đầy.");
            emitEvent("manual_pump", "Không bật bơm vì phao đang báo nước đầy.");
            return;
        }

        setInletPump(state);
        manualInletActive = state;
        if (state) {
            manualInletSince = millis();
            floatFullSince = 0;
            logFloatPins("bật bơm nạp");
            LOGF("[Sampling] [Manual] Bơm nạp: BẬT (tắt khi phao đầy ổn định hoặc sau %lus)\n",
                 MAX_MANUAL_PUMP_TIME / 1000UL);
        } else {
            LOGLN("[Sampling] [Manual] Bơm nạp: TẮT");
        }
        emitEvent("manual_pump", String("Bơm nạp nước: ") + (state ? "BẬT" : "TẮT"));
        return;
    }

    if (t == "drain") {
        if (state && isWaterEmpty()) {
            setDrainValve(false);
            manualDrainActive = false;
            logFloatPins("từ chối mở van xả");
            LOGLN("[Sampling] [Manual] Từ chối mở van: phao đang báo cạn.");
            emitEvent("manual_pump", "Không mở van xả vì phao đang báo nước cạn.");
            return;
        }

        setDrainValve(state);
        manualDrainActive = state;
        if (state) {
            manualDrainSince = millis();
            floatEmptySince = 0;
            logFloatPins("bật van xả");
            LOGF("[Sampling] [Manual] Van xả: BẬT (tắt khi phao cạn ổn định hoặc sau %lus)\n",
                 MAX_MANUAL_PUMP_TIME / 1000UL);
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
    // Bơm/xả thủ công độc lập với chu trình đo
    if (manualInletActive) {
        unsigned long onFor = millis() - manualInletSince;
        if (onFor < PUMP_FLOAT_GRACE_MS) {
            floatFullSince = 0;
        } else if (isWaterFull()) {
            if (floatFullSince == 0) {
                floatFullSince = millis();
            }
            if (millis() - floatFullSince >= FLOAT_DEBOUNCE_TIME) {
                setInletPump(false);
                manualInletActive = false;
                floatFullSince = 0;
                logFloatPins("phao đầy ổn định → tắt bơm nạp");
                LOGLN("[Sampling] [Manual] Phao đầy → tắt bơm nạp.");
                emitEvent("manual_pump", "Phao đầy. Đã tự tắt bơm nạp.");
            }
        } else {
            floatFullSince = 0;
        }

        if (manualInletActive && onFor >= MAX_MANUAL_PUMP_TIME) {
            setInletPump(false);
            manualInletActive = false;
            floatFullSince = 0;
            logFloatPins("hết 60s → tắt bơm nạp");
            LOGLN("[Sampling] [Manual] Hết 60s → tắt bơm nạp (an toàn).");
            emitEvent("manual_pump_timeout", "Bơm nạp thủ công quá 60s. Đã tự tắt.");
        }
    }

    if (manualDrainActive) {
        unsigned long onFor = millis() - manualDrainSince;
        if (onFor < PUMP_FLOAT_GRACE_MS) {
            floatEmptySince = 0;
        } else if (isWaterEmpty()) {
            if (floatEmptySince == 0) {
                floatEmptySince = millis();
            }
            if (millis() - floatEmptySince >= FLOAT_DEBOUNCE_TIME) {
                setDrainValve(false);
                manualDrainActive = false;
                floatEmptySince = 0;
                logFloatPins("phao cạn ổn định → tắt van xả");
                LOGLN("[Sampling] [Manual] Phao cạn → tắt van xả.");
                emitEvent("manual_pump", "Phao cạn. Đã tự tắt van xả.");
            }
        } else {
            floatEmptySince = 0;
        }

        if (manualDrainActive && onFor >= MAX_MANUAL_PUMP_TIME) {
            setDrainValve(false);
            manualDrainActive = false;
            floatEmptySince = 0;
            logFloatPins("hết 60s → tắt van xả");
            LOGLN("[Sampling] [Manual] Hết 60s → tắt van xả (an toàn).");
            emitEvent("manual_pump_timeout", "Van xả thủ công quá 60s. Đã tự tắt.");
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
        emitEvent("idle", "Đã đo xong. Queue rỗng. Sẵn sàng.");
    } else {
        emitEvent("idle", "Đã đo xong. Còn lệnh trong queue, chạy tiếp...");
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
        floatFullSince = 0;
    } else if (isWaterFull()) {
        if (floatFullSince == 0) {
            floatFullSince = millis();
        }
        if (millis() - floatFullSince >= FLOAT_DEBOUNCE_TIME) {
            fillFailCount = 0;
            logFloatPins("chu trình đo: phao đầy ổn định");
            LOGLN("[Sampling] [Phao] NƯỚC ĐẦY. Tắt bơm.");
            transitionTo(STATE_IDLE);
            emitEvent("filled", "Phao báo NƯỚC ĐẦY. Tắt bơm, tiếp tục hàng đợi đo.");
            return;
        }
    } else {
        floatFullSince = 0;
    }

    if (elapsed >= MAX_FILL_TIME) {
        fillFailCount++;
        LOGF("[Sampling] LỖI: Bơm quá %lus chưa đầy.\n", MAX_FILL_TIME / 1000UL);
        powerOffAllSensors();

        if (fillFailCount >= MAX_FILL_FAILS) {
            size_t dropped = commandQueue.size();
            clearCommandQueue();
            fillFailCount = 0;
            LOGF(
                "[Sampling] ABORT: timeout bơm %u lần liên tiếp → hủy %u lệnh còn lại.\n",
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
                String(MAX_FILL_FAILS) + "). Hủy lệnh đo hiện tại."
        );
        transitionTo(STATE_IDLE);
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
    // Đo thực hiện một lần trong handleStateStabilizing rồi chuyển PUBLISHING.
}

void SamplingManager::handleStateDraining(unsigned long elapsed) {
    if (elapsed < PUMP_FLOAT_GRACE_MS) {
        floatEmptySince = 0;
    } else if (isWaterEmpty()) {
        if (floatEmptySince == 0) {
            floatEmptySince = millis();
        }
        if (millis() - floatEmptySince >= FLOAT_DEBOUNCE_TIME) {
            logFloatPins("chu trình đo: phao cạn ổn định");
            LOGLN("[Sampling] [Phao] NƯỚC CẠN. Tắt van.");
            transitionTo(STATE_IDLE);
            emitEvent("drained", "Phao báo nước cạn. Đã tắt van xả.");
            if (commandQueue.empty()) {
                emitEvent("idle", "Đã xả xong. Queue rỗng. Sẵn sàng.");
            } else {
                emitEvent("idle", "Đã xả xong. Còn lệnh trong queue, chạy tiếp...");
            }
            return;
        }
    } else {
        floatEmptySince = 0;
    }

    if (elapsed >= MAX_DRAIN_TIME) {
        LOGF("[Sampling] LỖI: Xả quá %lus chưa cạn.\n", MAX_DRAIN_TIME / 1000UL);
        emitEvent("drain_timeout", "Quá thời gian xả nước. Đã tắt van.");
        transitionTo(STATE_IDLE);
        emitEvent("idle", "Hết timeout xả. Hệ thống sẵn sàng.");
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
