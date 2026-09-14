#include "SamplingManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin() {
    currentState = STATE_IDLE;
    stateTimer = millis();
    while (!commandQueue.empty()) commandQueue.pop();

    pinMode(INLET_PUMP_PIN, OUTPUT);
    digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    pinMode(DRAIN_VALVE_PIN, OUTPUT);
    digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
    pinMode(FLOAT_FULL_PIN, INPUT_PULLUP);
    pinMode(FLOAT_EMPTY_PIN, INPUT_PULLUP);
    Serial.println("[Sampling] Relay: bơm=GPIO18, van=GPIO19 | Phao: đầy=GPIO4, cạn=GPIO17");

    pinMode(TDS_SENSOR_PIN, INPUT);
    analogReadResolution(12);
    analogSetPinAttenuation(TDS_SENSOR_PIN, ADC_11db);
    Serial.println("[Sampling] TDS Meter V1.0 gắn GPIO35 (ADC1).");

    pinMode(TURBIDITY_SENSOR_PIN, INPUT);
    analogSetPinAttenuation(TURBIDITY_SENSOR_PIN, ADC_11db);
    Serial.println("[Sampling] Turbidity analog gắn GPIO34 (ADC1).");

    pinMode(PH_SENSOR_PIN, INPUT);
    analogSetPinAttenuation(PH_SENSOR_PIN, ADC_11db);
    Serial.println("[Sampling] PH-4502C gắn GPIO32 (Po). V+ dùng 5V, hiệu chuẩn POT gần BNC.");

    pinMode(PH_POWER_PIN, OUTPUT);
    pinMode(TURBIDITY_POWER_PIN, OUTPUT);
    pinMode(TDS_POWER_PIN, OUTPUT);
    powerOffAllSensors();
    Serial.println("[Sampling] Power: pH=GPIO14, Turbidity=GPIO33, TDS=GPIO25 (2N3904).");
    Serial.println("[Sampling] FSM: 1 sensor = 1 commandQueue | xả khi queue rỗng | raw ADC→backend cal");
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
    Serial.printf("[Sampling Event] [%s] %s\n", stage.c_str(), message.c_str());

    if (mqttHandler.isConnected()) {
        String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
        payload += "\"stage\":\"" + stage + "\",";
        payload += "\"state\":\"" + getStateName() + "\",";
        payload += "\"message\":\"" + message + "\"";
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

void SamplingManager::setInletPump(bool enabled) {
    if (enabled) {
        digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
    }

    digitalWrite(INLET_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    Serial.println(enabled ? "[Pump] BẬT bơm nạp (GPIO18)" : "[Pump] TẮT bơm nạp (GPIO18)");
}

void SamplingManager::setDrainValve(bool enabled) {
    if (enabled) {
        digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
    }

    digitalWrite(DRAIN_VALVE_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    Serial.println(enabled ? "[Valve] BẬT van xả (GPIO19)" : "[Valve] TẮT van xả (GPIO19)");
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
    Serial.printf("[Power] %s %s (GPIO%u)\n", name, enabled ? "BẬT" : "TẮT", pin);

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
        setInletPump(!isWaterFull());
    }

    if (newState == STATE_DRAINING) {
        floatEmptySince = 0;
        setDrainValve(!isWaterEmpty());
    }
}

void SamplingManager::enqueueMeasure(const std::vector<String> &sensors) {
    std::vector<SensorType> list;
    expandSensorTokens(sensors, list);

    if (list.empty()) {
        Serial.println("[Sampling] MEASURE không có cảm biến hợp lệ, bỏ qua.");
        emitEvent("command_error", "sensors không hợp lệ.");
        return;
    }

    for (SensorType t : list) {
        PendingCommand cmd;
        cmd.type = CMD_MEASURE;
        cmd.sensor = t;
        commandQueue.push(cmd);
        Serial.printf(
            "[Sampling] Queue +MEASURE %s (queue=%u)\n",
            getSensorName(t).c_str(),
            (unsigned)commandQueue.size()
        );
    }
    emitEvent("queued", "Đã xếp " + String((unsigned)list.size()) + " lệnh đo vào commandQueue.");
}

void SamplingManager::enqueuePump(const String &target, bool state) {
    PendingCommand cmd;
    cmd.type = CMD_PUMP;
    cmd.pumpTarget = target;
    cmd.pumpState = state;
    commandQueue.push(cmd);
    Serial.printf("[Sampling] Queue +PUMP %s=%s (queue=%u)\n",
                  target.c_str(), state ? "ON" : "OFF", (unsigned)commandQueue.size());
    emitEvent("queued", "Đã xếp lệnh bơm vào commandQueue.");
}

void SamplingManager::enqueueStatus() {
    PendingCommand cmd;
    cmd.type = CMD_STATUS;
    commandQueue.push(cmd);
    Serial.printf("[Sampling] Queue +STATUS (queue=%u)\n", (unsigned)commandQueue.size());
}

void SamplingManager::publishStatus() {
    String statusJson = "{";
    statusJson += "\"device_id\":\"" + currentConfig.device_id + "\",";
    statusJson += "\"state\":\"" + getStateName() + "\",";
    statusJson += "\"is_busy\":" + String(isBusy() ? "true" : "false") + ",";
    statusJson += "\"queue_size\":" + String((unsigned)commandQueue.size());
    statusJson += "}";

    Serial.println("[Sampling] Status: " + statusJson);
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
            Serial.printf("[Sampling] Chạy MEASURE [%s] (còn lại queue=%u)\n",
                          getSensorName(cmd.sensor).c_str(),
                          (unsigned)commandQueue.size());
            startMeasureCycle(cmd.sensor);
            break;
        case CMD_PUMP:
            Serial.println("[Sampling] Chạy PUMP từ hàng đợi");
            setManualPump(cmd.pumpTarget, cmd.pumpState);
            break;
        case CMD_STATUS:
            Serial.println("[Sampling] Chạy STATUS từ hàng đợi");
            publishStatus();
            break;
    }
}

void SamplingManager::startMeasureCycle(SensorType sensor) {
    currentSensor = sensor;
    currentResult = SensorResult();
    currentResult.startTime = millis();

    Serial.printf("\n[Sampling] >>> BẮT ĐẦU ĐO [%s] <<<\n", getSensorName(sensor).c_str());

    if (!isWaterFull()) {
        emitEvent("filling", "Nước chưa đầy. Bắt đầu bơm nạp...");
        transitionTo(STATE_FILLING);
    } else {
        emitEvent("stabilizing", "Nước đã đầy. Chờ ổn định rồi đo...");
        transitionTo(STATE_STABILIZING);
    }
}

void SamplingManager::setManualPump(const String &target, bool state) {
    String t = target;
    t.toLowerCase();
    if (t == "inlet" || t == "nap" || t == "vao") {
        if (state && isWaterFull()) {
            setInletPump(false);
            Serial.println("[Sampling] [Manual] Từ chối bật bơm: phao đang báo đầy.");
            emitEvent("manual_pump", "Không bật bơm vì phao đang báo nước đầy.");
            return;
        }

        setInletPump(state);
        Serial.printf("[Sampling] [Manual] Bơm nạp: %s\n", state ? "BẬT" : "TẮT");
        emitEvent("manual_pump", String("Bơm nạp nước: ") + (state ? "BẬT" : "TẮT"));
    } else if (t == "drain" || t == "xa" || t == "ra" || t == "valve") {
        if (state && isWaterEmpty()) {
            setDrainValve(false);
            Serial.println("[Sampling] [Manual] Từ chối mở van: phao đang báo cạn.");
            emitEvent("manual_pump", "Không mở van xả vì phao đang báo nước cạn.");
            return;
        }

        setDrainValve(state);
        Serial.printf("[Sampling] [Manual] Van xả: %s\n", state ? "BẬT" : "TẮT");
        emitEvent("manual_pump", String("Van xả nước: ") + (state ? "BẬT" : "TẮT"));
    }
}

void SamplingManager::measureSensor(SensorType type) {
    switch (type) {
        case SENSOR_TEMP: {
            Serial.println("[Sampling] [Temp] Bỏ qua: chưa có cảm biến nhiệt độ.");
            emitEvent("measuring_sensor", "Bỏ qua nhiệt độ (chưa có sensor phần cứng).");
            break;
        }
        case SENSOR_PH: {
            Serial.println("[Sampling] [PH-4502C] -> Bật transistor GPIO14, đọc ADC GPIO32...");
            setSensorPower(SENSOR_PH, true);
            currentResult.phRaw = readRawADC(PH_SENSOR_PIN, PH_SAMPLE_COUNT);
            setSensorPower(SENSOR_PH, false);
            currentResult.hasPh = true;
            publishSensorReading(SENSOR_PH);
            break;
        }
        case SENSOR_TURBIDITY: {
            Serial.println("[Sampling] [Turbidity] -> Bật transistor GPIO33, đọc ADC GPIO34...");
            setSensorPower(SENSOR_TURBIDITY, true);
            currentResult.turbidityRaw = readRawADC(TURBIDITY_SENSOR_PIN, TURBIDITY_SAMPLE_COUNT);
            setSensorPower(SENSOR_TURBIDITY, false);
            currentResult.hasTurbidity = true;
            publishSensorReading(SENSOR_TURBIDITY);
            break;
        }
        case SENSOR_TDS: {
            Serial.println("[Sampling] [TDS] -> Bật transistor GPIO25, đọc ADC GPIO35...");
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

    Serial.println("\n[Sampling] >>> GỬI sensor_data NGAY <<<");
    Serial.println(payload);

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
    // Đã publish ngay trong measureSensor — đây chỉ quyết định xả hay lấy lệnh tiếp
    if (commandQueue.empty()) {
        if (!isWaterEmpty()) {
            emitEvent("draining", "Queue rỗng. Bắt đầu xả nước...");
            transitionTo(STATE_DRAINING);
        } else {
            transitionTo(STATE_IDLE);
            emitEvent("idle", "Queue rỗng, nước đã cạn. Sẵn sàng.");
        }
    } else {
        transitionTo(STATE_IDLE);
        emitEvent("idle", "Còn lệnh trong queue. Giữ nước, chạy lệnh tiếp...");
    }
}

void SamplingManager::handleStateIdle() {
    processCommandQueue();
}

void SamplingManager::handleStateFilling(unsigned long elapsed) {
    if (isWaterFull()) {
        if (floatFullSince == 0) {
            floatFullSince = millis();
        }
        if (millis() - floatFullSince >= FLOAT_DEBOUNCE_TIME) {
            Serial.println("[Sampling] [Phao] NƯỚC ĐẦY. Tắt bơm.");
            emitEvent("filled", "Phao báo NƯỚC ĐẦY. Đang chờ nước ổn định...");
            transitionTo(STATE_STABILIZING);
            return;
        }
    } else {
        floatFullSince = 0;
    }

    if (elapsed >= MAX_FILL_TIME) {
        Serial.println("[Sampling] LỖI: Bơm quá 60s chưa đầy.");
        emitEvent("fill_timeout", "Quá thời gian nạp nước. Hủy lệnh đo hiện tại.");
        powerOffAllSensors();
        transitionTo(STATE_IDLE);
    }
}

void SamplingManager::handleStateStabilizing(unsigned long elapsed) {
    if (elapsed >= STABILIZE_TIME) {
        transitionTo(STATE_MEASURING);
        measureSensor(currentSensor);
        powerOffAllSensors();
        Serial.printf("[Sampling] Đã đo xong [%s], đã gửi MQTT.\n", getSensorName(currentSensor).c_str());
        emitEvent("measuring_complete", "Đã gửi kết quả. Tiếp tục chu trình...");
        transitionTo(STATE_PUBLISHING);
    }
}

void SamplingManager::handleStateMeasuring(unsigned long /*elapsed*/) {
    // Đo thực hiện một lần trong handleStateStabilizing rồi chuyển PUBLISHING.
}

void SamplingManager::handleStateDraining(unsigned long elapsed) {
    if (isWaterEmpty()) {
        if (floatEmptySince == 0) {
            floatEmptySince = millis();
        }
        if (millis() - floatEmptySince >= FLOAT_DEBOUNCE_TIME) {
            Serial.println("[Sampling] [Phao] NƯỚC CẠN. Tắt van.");
            emitEvent("drained", "Phao báo nước cạn. Đã tắt van xả.");
            transitionTo(STATE_IDLE);
            emitEvent("idle", "Chu trình hoàn tất. Hệ thống sẵn sàng.");
            return;
        }
    } else {
        floatEmptySince = 0;
    }

    if (elapsed >= MAX_DRAIN_TIME) {
        Serial.println("[Sampling] LỖI: Xả quá 60s chưa cạn.");
        emitEvent("drain_timeout", "Quá thời gian xả nước. Đã tắt van.");
        transitionTo(STATE_IDLE);
        emitEvent("idle", "Hết timeout xả. Hệ thống sẵn sàng.");
    }
}

void SamplingManager::handleStatePublishing() {
    finishCycle();
}

void SamplingManager::handle() {
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

    Serial.printf("[RAW] GPIO%u | ADC=%d | V=%.3fV | samples=%d\n",
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
