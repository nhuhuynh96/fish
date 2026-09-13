#include "SamplingManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin(bool simulation) {
    simulationMode = simulation;
    randomSeed(ESP.getEfuseMac() + micros());
    currentState = STATE_IDLE;
    stateTimer = millis();
    manualInletStartedAt = 0;
    manualDrainStartedAt = 0;
    manualInletActive = false;
    manualDrainActive = false;
    activeQueue.clear();
    completedInCycle.clear();
    nextPendingQueue.clear();
    while (!commandQueue.empty()) commandQueue.pop();

    if (!simulationMode) {
        pinMode(INLET_PUMP_PIN, OUTPUT);
        digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
        pinMode(DRAIN_VALVE_PIN, OUTPUT);
        digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
        pinMode(FLOAT_FULL_PIN, INPUT_PULLUP);
        pinMode(FLOAT_EMPTY_PIN, INPUT_PULLUP);
        Serial.println("[Sampling] Relay: bơm=GPIO18, van=GPIO19 | Phao: đầy=GPIO4, cạn=GPIO17");

        // TDS Meter V1.0: analog 0~2.3V, đọc bằng ADC 12-bit
        pinMode(TDS_SENSOR_PIN, INPUT);
        analogReadResolution(12);
        analogSetPinAttenuation(TDS_SENSOR_PIN, ADC_11db);
        Serial.println("[Sampling] TDS Meter V1.0 gắn GPIO35 (ADC1).");

        // Cảm biến độ đục/cặn analog
        pinMode(TURBIDITY_SENSOR_PIN, INPUT);
        analogSetPinAttenuation(TURBIDITY_SENSOR_PIN, ADC_11db);
        Serial.println("[Sampling] Turbidity analog gắn GPIO34 (ADC1).");

        // PH-4502C analog Po
        pinMode(PH_SENSOR_PIN, INPUT);
        analogSetPinAttenuation(PH_SENSOR_PIN, ADC_11db);
        Serial.println("[Sampling] PH-4502C gắn GPIO32 (Po). V+ dùng 5V, hiệu chuẩn POT gần BNC.");

        // Transistor 2N3904: mặc định TẮT nguồn từng cảm biến
        pinMode(PH_POWER_PIN, OUTPUT);
        pinMode(TURBIDITY_POWER_PIN, OUTPUT);
        pinMode(TDS_POWER_PIN, OUTPUT);
        powerOffAllSensors();
        Serial.println("[Sampling] Power: pH=GPIO14, Turbidity=GPIO33, TDS=GPIO25 (2N3904).");
    }

    Serial.println("[Sampling] Đã khởi tạo SamplingManager FSM (Simulation Mode: " + String(simulationMode ? "ON" : "OFF") + ")");
    Serial.println("[Sampling] Sẵn sàng nhận lệnh đo từ Backend qua MQTT.");
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

    if (n == "all" || n == "tat_ca") {
        outList.push_back(SENSOR_TEMP);
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
    if (simulationMode) return;

    if (enabled) {
        // Tắt van xả trực tiếp (tránh đệ quy setDrainValve -> setInletPump)
        digitalWrite(DRAIN_VALVE_PIN, PUMP_OFF_LEVEL);
        manualDrainActive = false;
        manualDrainStartedAt = 0;
    }

    digitalWrite(INLET_PUMP_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    if (!enabled) {
        manualInletActive = false;
        manualInletStartedAt = 0;
    }
    Serial.println(enabled ? "[Pump] BẬT bơm nạp (GPIO18)" : "[Pump] TẮT bơm nạp (GPIO18)");
}

void SamplingManager::setDrainValve(bool enabled) {
    if (simulationMode) return;

    if (enabled) {
        digitalWrite(INLET_PUMP_PIN, PUMP_OFF_LEVEL);
        manualInletActive = false;
        manualInletStartedAt = 0;
    }

    digitalWrite(DRAIN_VALVE_PIN, enabled ? PUMP_ON_LEVEL : PUMP_OFF_LEVEL);
    if (!enabled) {
        manualDrainActive = false;
        manualDrainStartedAt = 0;
    }
    Serial.println(enabled ? "[Valve] BẬT van xả (GPIO19)" : "[Valve] TẮT van xả (GPIO19)");
}

void SamplingManager::powerOffAllSensors() {
    if (simulationMode) return;
    digitalWrite(PH_POWER_PIN, SENSOR_POWER_OFF);
    digitalWrite(TURBIDITY_POWER_PIN, SENSOR_POWER_OFF);
    digitalWrite(TDS_POWER_PIN, SENSOR_POWER_OFF);
}

void SamplingManager::setSensorPower(SensorType type, bool enabled) {
    if (simulationMode) return;

    // Chỉ bật đúng 1 cảm biến để giảm nhiễu pH <-> TDS.
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
    // Luôn tắt bơm khi rời trạng thái nạp nước.
    if (!simulationMode && currentState == STATE_FILLING && newState != STATE_FILLING) {
        setInletPump(false);
    }

    // Luôn tắt van xả khi rời trạng thái xả.
    if (!simulationMode && currentState == STATE_DRAINING && newState != STATE_DRAINING) {
        setDrainValve(false);
    }

    // Rảnh hoặc rời đo: đảm bảo tắt hết transistor cảm biến.
    // Không tắt bơm/van thủ công khi về IDLE (để user giữ bơm chạy sau khi đo).
    if (!simulationMode && newState == STATE_PUBLISHING) {
        powerOffAllSensors();
        setInletPump(false);
        setDrainValve(false);
    }
    if (!simulationMode && newState == STATE_IDLE) {
        powerOffAllSensors();
    }

    currentState = newState;
    stateTimer = millis();

    // Khi bắt đầu nạp, chỉ bật bơm nếu phao chưa báo đầy.
    if (!simulationMode && newState == STATE_FILLING) {
        floatFullSince = 0;
        setInletPump(!isWaterFull());
        if (!isWaterFull()) {
            manualInletActive = true;
            manualInletStartedAt = millis();
        }
    }

    // Khi bắt đầu xả, chỉ mở van nếu chưa cạn.
    if (!simulationMode && newState == STATE_DRAINING) {
        floatEmptySince = 0;
        setDrainValve(!isWaterEmpty());
        if (!isWaterEmpty()) {
            manualDrainActive = true;
            manualDrainStartedAt = millis();
        }
    }
}

void SamplingManager::enqueueMeasure(const std::vector<String> &sensors) {
    // Đang đo -> gộp cảm biến ngay, không xếp hàng chờ.
    if (currentState == STATE_STABILIZING || currentState == STATE_MEASURING) {
        requestMeasurement(sensors);
        return;
    }

    PendingCommand cmd;
    cmd.type = CMD_MEASURE;
    cmd.sensors = sensors;
    commandQueue.push(cmd);
    Serial.printf("[Sampling] Đã xếp hàng MEASURE (queue=%u)\n", (unsigned)commandQueue.size());
    emitEvent("queued", "Đã xếp hàng lệnh đo. Sẽ chạy tuần tự khi rảnh.");
}

void SamplingManager::enqueuePump(const String &target, bool state) {
    // Bơm/status chờ đến khi IDLE để không chen giữa chu trình đo.
    if (currentState == STATE_IDLE && commandQueue.empty()) {
        setManualPump(target, state);
        return;
    }

    PendingCommand cmd;
    cmd.type = CMD_PUMP;
    cmd.pumpTarget = target;
    cmd.pumpState = state;
    commandQueue.push(cmd);
    Serial.printf("[Sampling] Đã xếp hàng PUMP (queue=%u)\n", (unsigned)commandQueue.size());
    emitEvent("queued", "Đã xếp hàng lệnh bơm. Sẽ chạy sau khi chu trình hiện tại xong.");
}

void SamplingManager::enqueueStatus() {
    if (currentState == STATE_IDLE && commandQueue.empty()) {
        publishStatus();
        return;
    }

    PendingCommand cmd;
    cmd.type = CMD_STATUS;
    commandQueue.push(cmd);
    Serial.printf("[Sampling] Đã xếp hàng STATUS (queue=%u)\n", (unsigned)commandQueue.size());
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
            Serial.println("[Sampling] Chạy lệnh MEASURE từ hàng đợi");
            requestMeasurement(cmd.sensors);
            break;
        case CMD_PUMP:
            Serial.println("[Sampling] Chạy lệnh PUMP từ hàng đợi");
            setManualPump(cmd.pumpTarget, cmd.pumpState);
            break;
        case CMD_STATUS:
            Serial.println("[Sampling] Chạy lệnh STATUS từ hàng đợi");
            publishStatus();
            break;
    }
}

bool SamplingManager::requestMeasurement(const std::vector<String> &sensors) {
    std::vector<SensorType> targetSensors;

    for (const String &s : sensors) {
        parseSensorName(s, targetSensors);
    }

    // Loại bỏ các cảm biến trùng lặp trong danh sách yêu cầu
    std::vector<SensorType> uniqueTargets;
    for (SensorType t : targetSensors) {
        bool exists = false;
        for (SensorType u : uniqueTargets) {
            if (u == t) { exists = true; break; }
        }
        if (!exists) uniqueTargets.push_back(t);
    }

    if (uniqueTargets.empty()) {
        Serial.println("[Sampling] Yêu cầu đo không có cảm biến hợp lệ!");
        return false;
    }

    // TÌNH HUỐNG 1: Rảnh -> đo ngay (KHÔNG tự bơm). Bơm chỉ qua action "pump".
    if (currentState == STATE_IDLE) {
        activeQueue = uniqueTargets;
        completedInCycle.clear();
        currentResult = SensorResult();
        currentResult.startTime = millis();
        currentMeasuringIndex = -1;

        Serial.println("\n[Sampling] >>> BẮT ĐẦU ĐO (không bơm) <<<");
        emitEvent("measuring", "Nhận lệnh đo. Bắt đầu đọc cảm biến (không kích bơm)...");
        transitionTo(STATE_MEASURING);
        processNextSensor();
        return true;
    }

    // TÌNH HUỐNG 2: Đang đo -> gộp thêm cảm biến vào lượt hiện tại
    if (currentState == STATE_STABILIZING || currentState == STATE_MEASURING) {
        String appendedNames = "";
        for (SensorType t : uniqueTargets) {
            bool inQueue = false;
            for (SensorType q : activeQueue) {
                if (q == t) { inQueue = true; break; }
            }
            bool alreadyDone = false;
            for (SensorType c : completedInCycle) {
                if (c == t) { alreadyDone = true; break; }
            }

            if (!inQueue && !alreadyDone) {
                activeQueue.push_back(t);
                if (appendedNames.length() > 0) appendedNames += ", ";
                appendedNames += getSensorName(t);
            }
        }

        if (appendedNames.length() > 0) {
            Serial.printf("[Sampling] >>> GỘP LỆNH: Đã thêm [%s] vào lượt đo hiện tại <<<\n", appendedNames.c_str());
            emitEvent("appended", "Đã gộp thêm cảm biến [" + appendedNames + "] vào lượt đo hiện tại.");
        } else {
            Serial.println("[Sampling] Các cảm biến yêu cầu đã nằm trong hàng đợi hoặc vừa được đo.");
        }
        return true;
    }

    // TÌNH HUỐNG 3: Đang publish kết quả -> xếp cho lượt kế
    if (currentState == STATE_PUBLISHING || currentState == STATE_DRAINING || currentState == STATE_FILLING) {
        for (SensorType t : uniqueTargets) {
            bool inNext = false;
            for (SensorType n : nextPendingQueue) {
                if (n == t) { inNext = true; break; }
            }
            if (!inNext) nextPendingQueue.push_back(t);
        }
        Serial.println("[Sampling] Đang bận. Đã xếp yêu cầu đo vào hàng đợi lượt tiếp theo!");
        emitEvent("queued", "Hệ thống đang bận. Yêu cầu đo mới sẽ chạy sau khi xong.");
        return true;
    }

    return false;
}

void SamplingManager::setManualPump(const String &target, bool state) {
    String t = target;
    t.toLowerCase();
    if (t == "inlet" || t == "nap" || t == "vao") {
        if (!simulationMode) {
            if (state && isWaterFull()) {
                setInletPump(false);
                Serial.println("[Sampling] [Manual] Từ chối bật bơm: phao đang báo đầy.");
                emitEvent("manual_pump", "Không bật bơm vì phao đang báo nước đầy.");
                return;
            }

            setInletPump(state);
            if (state) {
                manualInletActive = true;
                manualInletStartedAt = millis();
                Serial.printf("[Sampling] [Manual] Bắt đầu đếm timeout bơm từ 0/%lu ms\n", MAX_FILL_TIME);
            }
        }
        Serial.printf("[Sampling] [Manual] Bơm nạp: %s\n", state ? "BẬT" : "TẮT");
        emitEvent("manual_pump", String("Bơm nạp nước: ") + (state ? "BẬT" : "TẮT"));
    } else if (t == "drain" || t == "xa" || t == "ra" || t == "valve") {
        if (!simulationMode) {
            if (state && isWaterEmpty()) {
                setDrainValve(false);
                Serial.println("[Sampling] [Manual] Từ chối mở van: phao đang báo cạn.");
                emitEvent("manual_pump", "Không mở van xả vì phao đang báo nước cạn.");
                return;
            }

            setDrainValve(state);
            if (state) {
                manualDrainActive = true;
                manualDrainStartedAt = millis();
                Serial.printf("[Sampling] [Manual] Bắt đầu đếm timeout van từ 0/%lu ms\n", MAX_DRAIN_TIME);
            }
        }
        Serial.printf("[Sampling] [Manual] Van xả: %s\n", state ? "BẬT" : "TẮT");
        emitEvent("manual_pump", String("Van xả nước: ") + (state ? "BẬT" : "TẮT"));
    }
}

void SamplingManager::measureSensor(SensorType type) {
    stepDuration = SENSOR_SAMPLE_TIME;
    stateTimer = millis(); // Đếm thời gian bước từ lúc bắt đầu đo cảm biến này

    switch (type) {
        case SENSOR_TEMP: {
            Serial.println("[Sampling] [Temp] -> Đang đọc cảm biến Nhiệt độ (sim)...");
            currentResult.temperature = simTemperature();
            currentResult.hasTemp = true;
            completedInCycle.push_back(SENSOR_TEMP);
            String extra = "\"sensor\":\"temperature\",\"value\":" + String(currentResult.temperature, 2) + ",\"unit\":\"°C\"";
            emitEvent("measuring_sensor", "Đang đo Nhiệt độ...", extra);
            break;
        }
        case SENSOR_PH: {
            Serial.println("[Sampling] [PH-4502C] -> Bật transistor GPIO14, đọc ADC GPIO32...");
            if (!simulationMode) setSensorPower(SENSOR_PH, true);
            currentResult.ph = simulationMode ? simPH() : readRealPH();
            if (!simulationMode) setSensorPower(SENSOR_PH, false);
            currentResult.hasPh = true;
            completedInCycle.push_back(SENSOR_PH);
            String extra = "\"sensor\":\"ph\",\"value\":" + String(currentResult.ph, 2) + ",\"unit\":\"pH\"";
            emitEvent("measuring_sensor", "Đang đo pH...", extra);
            break;
        }
        case SENSOR_TURBIDITY: {
            Serial.println("[Sampling] [Turbidity] -> Bật transistor GPIO33, đọc ADC GPIO34...");
            if (!simulationMode) setSensorPower(SENSOR_TURBIDITY, true);
            currentResult.turbidity = simulationMode ? simTurbidity() : readRealTurbidity();
            if (!simulationMode) setSensorPower(SENSOR_TURBIDITY, false);
            currentResult.hasTurbidity = true;
            completedInCycle.push_back(SENSOR_TURBIDITY);
            String extra = "\"sensor\":\"turbidity\",\"value\":" + String(currentResult.turbidity, 1) + ",\"unit\":\"NTU\"";
            emitEvent("measuring_sensor", "Đang đo Độ đục/cặn...", extra);
            break;
        }
        case SENSOR_TDS: {
            Serial.println("[Sampling] [TDS] -> Bật transistor GPIO25, đọc ADC GPIO35...");
            if (!simulationMode) setSensorPower(SENSOR_TDS, true);
            float tempForComp = currentResult.hasTemp ? currentResult.temperature : 25.0f;
            currentResult.tds = simulationMode ? simTDS() : readRealTDS(tempForComp);
            if (!simulationMode) setSensorPower(SENSOR_TDS, false);
            currentResult.hasTds = true;
            completedInCycle.push_back(SENSOR_TDS);
            String extra = "\"sensor\":\"tds\",\"value\":" + String(currentResult.tds, 0) + ",\"unit\":\"ppm\"";
            emitEvent("measuring_sensor", "Đang đo TDS...", extra);
            break;
        }
    }
}

void SamplingManager::processNextSensor() {
    currentMeasuringIndex++;
    if (currentMeasuringIndex < (int)activeQueue.size()) {
        measureSensor(activeQueue[currentMeasuringIndex]);
    } else {
        // Đã đo xong tất cả -> publish ngay (không tự xả/bơm)
        powerOffAllSensors();
        Serial.println("[Sampling] Hoàn tất đo tất cả các cảm biến trong lượt này.");
        emitEvent("measuring_complete", "Hoàn tất đo các cảm biến. Đang gửi kết quả...");
        transitionTo(STATE_PUBLISHING);
    }
}

void SamplingManager::finishCycle() {
    currentResult.durationMs = millis() - currentResult.startTime;

    // Đóng gói JSON gửi dữ liệu đo được
    String payload = "{";
    payload += "\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"timestamp\":" + String(millis() / 1000) + ",";
    payload += "\"status\":\"success\",";
    payload += "\"duration_ms\":" + String(currentResult.durationMs) + ",";

    // Danh sách các cảm biến đã đo
    payload += "\"sensors_measured\":[";
    for (size_t i = 0; i < completedInCycle.size(); ++i) {
        if (i > 0) payload += ",";
        payload += "\"" + getSensorName(completedInCycle[i]) + "\"";
    }
    payload += "],";

    // Dữ liệu chi tiết
    payload += "\"data\":{";
    payload += "\"temperature\":" + (currentResult.hasTemp ? String(currentResult.temperature, 2) : "null") + ",";
    payload += "\"ph\":" + (currentResult.hasPh ? String(currentResult.ph, 2) : "null") + ",";
    payload += "\"turbidity\":" + (currentResult.hasTurbidity ? String(currentResult.turbidity, 1) : "null") + ",";
    payload += "\"tds\":" + (currentResult.hasTds ? String(currentResult.tds, 0) : "null");
    payload += "},";

    // Đơn vị
    payload += "\"units\":{";
    payload += "\"temperature\":\"°C\",";
    payload += "\"ph\":\"pH\",";
    payload += "\"turbidity\":\"NTU\",";
    payload += "\"tds\":\"ppm\"";
    payload += "}";
    payload += "}";

    Serial.println("\n[Sampling] >>> KẾT QUẢ ĐO HOÀN CHỈNH <<<");
    Serial.println(payload);

    if (mqttHandler.isConnected()) {
        mqttHandler.publish("sensor_data", payload);
    }

    // Kiểm tra xem có yêu cầu đang chờ ở chu trình tiếp theo không
    if (!nextPendingQueue.empty()) {
        Serial.println("[Sampling] Tự động bắt đầu đo tiếp theo từ hàng đợi...");
        activeQueue = nextPendingQueue;
        nextPendingQueue.clear();
        completedInCycle.clear();
        currentResult = SensorResult();
        currentResult.startTime = millis();
        currentMeasuringIndex = -1;

        emitEvent("measuring", "Bắt đầu đo tiếp theo từ hàng đợi (không bơm)...");
        transitionTo(STATE_MEASURING);
        processNextSensor();
    } else {
        transitionTo(STATE_IDLE);
        emitEvent("idle", "Chu trình đo hoàn tất. Hệ thống sẵn sàng.");
    }
}

void SamplingManager::handle() {
    // Xử lý FSM State Machine
    unsigned long elapsed = millis() - stateTimer;

    switch (currentState) {
        case STATE_IDLE:
            // Bảo vệ bơm nạp / van xả thủ công — dùng timer riêng, không dùng stateTimer FSM.
            if (!simulationMode && manualInletActive) {
                unsigned long pumpElapsed = millis() - manualInletStartedAt;
                if (isWaterFull() || pumpElapsed >= MAX_FILL_TIME) {
                    bool full = isWaterFull();
                    Serial.printf(
                        "[Sampling] Tắt bơm thủ công: %s (elapsed=%lu ms)\n",
                        full ? "phao đầy" : "timeout",
                        pumpElapsed
                    );
                    setInletPump(false);
                    emitEvent(
                        full ? "filled" : "fill_timeout",
                        full
                            ? "Phao đầy. Đã tự tắt bơm thủ công."
                            : "Bơm thủ công quá 60 giây. Đã tự tắt để bảo vệ."
                    );
                }
            } else if (!simulationMode && manualDrainActive) {
                unsigned long drainElapsed = millis() - manualDrainStartedAt;
                if (isWaterEmpty() || drainElapsed >= MAX_DRAIN_TIME) {
                    bool empty = isWaterEmpty();
                    Serial.printf(
                        "[Sampling] Tắt van thủ công: %s (elapsed=%lu ms)\n",
                        empty ? "phao cạn" : "timeout",
                        drainElapsed
                    );
                    setDrainValve(false);
                    emitEvent(
                        empty ? "drained" : "drain_timeout",
                        empty
                            ? "Phao cạn. Đã tự tắt van xả thủ công."
                            : "Van xả thủ công quá 60 giây. Đã tự tắt để bảo vệ."
                    );
                }
            } else {
                // Rảnh và tải đang tắt -> lấy lệnh tiếp theo trong hàng đợi FIFO
                processCommandQueue();
            }
            break;

        case STATE_FILLING:
            if (simulationMode) {
                if (elapsed >= FILL_SIM_TIME) {
                    Serial.println("[Sampling] [Phao Nước] -> Mô phỏng mức NƯỚC ĐẦY!");
                    emitEvent("filled", "Mô phỏng phao báo NƯỚC ĐẦY. Đang chờ nước ổn định...");
                    transitionTo(STATE_STABILIZING);
                }
                break;
            }

            if (isWaterFull()) {
                if (floatFullSince == 0) {
                    floatFullSince = millis();
                }

                // Chỉ công nhận đầy khi phao giữ trạng thái liên tục để tránh nhiễu.
                if (millis() - floatFullSince >= FLOAT_DEBOUNCE_TIME) {
                    Serial.println("[Sampling] [Phao Nước] -> Báo mức NƯỚC ĐẦY! Tắt bơm nạp.");
                    emitEvent("filled", "Phao báo NƯỚC ĐẦY. Đang chờ nước ổn định...");
                    transitionTo(STATE_STABILIZING);
                    break;
                }
            } else {
                floatFullSince = 0;
            }

            // Chống tràn/cháy bơm nếu phao bị kẹt hoặc nguồn nước hết.
            if (elapsed >= MAX_FILL_TIME) {
                Serial.println("[Sampling] LỖI: Bơm quá 60 giây nhưng phao chưa báo đầy.");
                emitEvent("fill_timeout", "Quá thời gian nạp nước. Đã tắt bơm để bảo vệ.");
                activeQueue.clear();
                completedInCycle.clear();
                powerOffAllSensors();
                transitionTo(STATE_IDLE);
            }
            break;

        case STATE_STABILIZING:
            // Chờ 1 giây để nước lắng và bọt khí tan
            if (elapsed >= STABILIZE_TIME) {
                transitionTo(STATE_MEASURING);
                currentMeasuringIndex = -1;
                processNextSensor();
            }
            break;

        case STATE_MEASURING:
            // Đã đọc xong trong measureSensor (bật transistor -> đọc -> tắt).
            // Chờ stepDuration rồi chuyển cảm biến tiếp theo.
            if (elapsed >= stepDuration) {
                if (currentMeasuringIndex >= 0 && currentMeasuringIndex < (int)activeQueue.size()) {
                    SensorType prevType = activeQueue[currentMeasuringIndex];
                    Serial.printf("[Sampling] Đã xong bước đo %s.\n", getSensorName(prevType).c_str());
                }
                processNextSensor();
            }
            break;

        case STATE_DRAINING:
            if (simulationMode) {
                if (elapsed >= DRAIN_SIM_TIME) {
                    Serial.println("[Sampling] [Phao Nước] -> Mô phỏng NƯỚC ĐÃ CẠN!");
                    emitEvent("drained", "Mô phỏng đã xả nước xong.");
                    transitionTo(STATE_PUBLISHING);
                }
                break;
            }

            if (isWaterEmpty()) {
                if (floatEmptySince == 0) {
                    floatEmptySince = millis();
                }

                if (millis() - floatEmptySince >= FLOAT_DEBOUNCE_TIME) {
                    Serial.println("[Sampling] [Phao Cạn] -> Báo NƯỚC ĐÃ CẠN! Tắt van xả.");
                    emitEvent("drained", "Phao báo nước cạn. Đã tắt van xả.");
                    transitionTo(STATE_PUBLISHING);
                    break;
                }
            } else {
                floatEmptySince = 0;
            }

            if (elapsed >= MAX_DRAIN_TIME) {
                Serial.println("[Sampling] LỖI: Xả quá 60 giây nhưng phao chưa báo cạn.");
                emitEvent("drain_timeout", "Quá thời gian xả nước. Đã tắt van để bảo vệ.");
                transitionTo(STATE_PUBLISHING);
            }
            break;

        case STATE_PUBLISHING:
            finishCycle();
            break;
    }
}

// -------------------------------------------------------------
// CÁC HÀM MÔ PHỎNG DỮ LIỆU CHÂN THỰC THEO CHUẨN HỒ CÁ
// -------------------------------------------------------------

float SamplingManager::simTemperature() {
    // Nhiệt độ dao động 25.5°C - 28.5°C
    int base = 255 + random(0, 31);
    return (float)base / 10.0f;
}

float SamplingManager::simPH() {
    // Độ pH dao động 6.85 - 7.65
    int base = 685 + random(0, 81);
    return (float)base / 100.0f;
}

float SamplingManager::simTurbidity() {
    // Độ đục dao động 5.0 - 22.0 NTU
    int base = 50 + random(0, 171);
    return (float)base / 10.0f;
}

float SamplingManager::simTDS() {
    // TDS dao động 140 - 260 ppm
    return (float)(140 + random(0, 121));
}

int SamplingManager::medianFilter(int *buffer, int count) const {
    // Sắp xếp chèn đơn giản cho bộ mẫu nhỏ
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

float SamplingManager::readRealTDS(float temperatureC) {
    int samples[TDS_SAMPLE_COUNT];

    for (int i = 0; i < TDS_SAMPLE_COUNT; i++) {
        samples[i] = analogRead(TDS_SENSOR_PIN);
        delay(20);
    }

    int medianAdc = medianFilter(samples, TDS_SAMPLE_COUNT);
    float voltage = (medianAdc * TDS_VREF) / (float)TDS_ADC_MAX;

    // Bù nhiệt theo công thức DFRobot / Keyestudio TDS Meter V1.0
    float compensationCoefficient = 1.0f + 0.02f * (temperatureC - 25.0f);
    if (compensationCoefficient < 0.1f) {
        compensationCoefficient = 0.1f;
    }
    float compensationVoltage = voltage / compensationCoefficient;

    float tdsValue =
        (133.42f * compensationVoltage * compensationVoltage * compensationVoltage
         - 255.86f * compensationVoltage * compensationVoltage
         + 857.39f * compensationVoltage) * 0.5f;

    if (tdsValue < 0.0f) tdsValue = 0.0f;
    if (tdsValue > 1000.0f) tdsValue = 1000.0f;

    Serial.printf(
        "[TDS] ADC=%d | V=%.3fV | Temp=%.1f°C | TDS=%.0f ppm\n",
        medianAdc, voltage, temperatureC, tdsValue
    );

    return tdsValue;
}

float SamplingManager::readRealTurbidity() {
    int samples[TURBIDITY_SAMPLE_COUNT];

    for (int i = 0; i < TURBIDITY_SAMPLE_COUNT; i++) {
        samples[i] = analogRead(TURBIDITY_SENSOR_PIN);
        delay(20);
    }

    int medianAdc = medianFilter(samples, TURBIDITY_SAMPLE_COUNT);
    float voltage = (medianAdc * TURBIDITY_VREF) / (float)TDS_ADC_MAX;

    // Map tuyến tính cho nguồn 3V3:
    // Điện áp cao hơn -> nước trong hơn -> NTU thấp hơn.
    // (Công thức Arduino 5V cũ không dùng được với 3V3)
    float ntu;
    if (voltage >= TURB_V_CLEAR) {
        ntu = 0.0f;
    } else if (voltage <= TURB_V_DIRTY) {
        ntu = TURB_NTU_MAX;
    } else {
        ntu = (TURB_V_CLEAR - voltage) / (TURB_V_CLEAR - TURB_V_DIRTY) * TURB_NTU_MAX;
    }

    if (ntu < 0.0f) ntu = 0.0f;
    if (ntu > TURB_NTU_MAX) ntu = TURB_NTU_MAX;

    Serial.printf(
        "[TURB] ADC=%d | V=%.3fV | Turbidity=%.1f NTU (3V3 cal)\n",
        medianAdc, voltage, ntu
    );

    return ntu;
}

float SamplingManager::readRealPH() {
    int samples[PH_SAMPLE_COUNT];

    for (int i = 0; i < PH_SAMPLE_COUNT; i++) {
        samples[i] = analogRead(PH_SENSOR_PIN);
        delay(20);
    }

    int medianAdc = medianFilter(samples, PH_SAMPLE_COUNT);
    float voltage = (medianAdc * PH_VREF) / (float)TDS_ADC_MAX;

    // pH = 7 + ((V_neutral - V) / slope)
    // V_neutral ≈ 2.50V sau khi chỉnh POT offset gần BNC.
    float ph = 7.0f + ((PH_NEUTRAL_VOLTAGE - voltage) / PH_SLOPE);
    if (ph < 0.0f) ph = 0.0f;
    if (ph > 14.0f) ph = 14.0f;

    Serial.printf(
        "[PH] ADC=%d | V=%.3fV | pH=%.2f\n",
        medianAdc, voltage, ph
    );

    return ph;
}
