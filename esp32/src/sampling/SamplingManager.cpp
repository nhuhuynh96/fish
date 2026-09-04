#include "SamplingManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"

SamplingManager samplingManager;

SamplingManager::SamplingManager() {}

void SamplingManager::begin(bool simulation) {
    simulationMode = simulation;
    randomSeed(ESP.getEfuseMac() + micros());
    currentState = STATE_IDLE;
    activeQueue.clear();
    completedInCycle.clear();
    nextPendingQueue.clear();

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
    } else if (n == "turbidity" || n == "turb" || n == "do_duc") {
        outList.push_back(SENSOR_TURBIDITY);
        return true;
    } else if (n == "tds" || n == "chat_ran") {
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

void SamplingManager::transitionTo(SamplingState newState) {
    currentState = newState;
    stateTimer = millis();
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

    // TÌNH HUỐNG 1: Thiết bị đang rảnh -> Bắt đầu chu trình chuẩn từ bơm nước
    if (currentState == STATE_IDLE) {
        activeQueue = uniqueTargets;
        completedInCycle.clear();
        currentResult = SensorResult();
        currentResult.startTime = millis();
        currentMeasuringIndex = -1;

        Serial.println("\n[Sampling] >>> BẮT ĐẦU CHU TRÌNH ĐO MỚI <<<");
        emitEvent("filling", "Nhận lệnh đo. Đang kích hoạt BƠM NẠP NƯỚC vào buồng đo...");
        transitionTo(STATE_FILLING);
        return true;
    }

    // TÌNH HUỐNG 2: Thiết bị đang BƠM hoặc đang ĐO (Nước đang có sẵn trong buồng)
    // -> Gộp (Append) các cảm biến mới vào danh sách đo của lượt hiện tại
    if (currentState == STATE_FILLING || currentState == STATE_STABILIZING || currentState == STATE_MEASURING) {
        String appendedNames = "";
        for (SensorType t : uniqueTargets) {
            // Kiểm tra xem cảm biến này đã có trong activeQueue hoặc đã đo xong trong lượt này chưa
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
            Serial.printf("[Sampling] >>> GỘP LỆNH THÀNH CÔNG: Đã thêm [%s] vào buồng đo đang có nước! <<<\n", appendedNames.c_str());
            emitEvent("appended", "Đã gộp thêm cảm biến [" + appendedNames + "] vào lượt đo hiện tại.");
        } else {
            Serial.println("[Sampling] Các cảm biến yêu cầu đã nằm trong hàng đợi hoặc vừa được đo.");
        }
        return true;
    }

    // TÌNH HUỐNG 3: Thiết bị đang XẢ NƯỚC hoặc đang PUBLISH (Nước đang rút đi)
    // -> Xếp hàng (Queue) cho chu trình kế tiếp
    if (currentState == STATE_DRAINING || currentState == STATE_PUBLISHING) {
        for (SensorType t : uniqueTargets) {
            bool inNext = false;
            for (SensorType n : nextPendingQueue) {
                if (n == t) { inNext = true; break; }
            }
            if (!inNext) nextPendingQueue.push_back(t);
        }
        Serial.println("[Sampling] Đang xả nước. Đã đưa yêu cầu mới vào hàng đợi lượt tiếp theo!");
        emitEvent("queued", "Hệ thống đang xả nước. Yêu cầu mới sẽ tự động chạy sau khi xả xong.");
        return true;
    }

    return false;
}

void SamplingManager::setManualPump(const String &target, bool state) {
    String t = target;
    t.toLowerCase();
    if (t == "inlet" || t == "nap" || t == "vao") {
        Serial.printf("[Sampling] [Manual] Bơm nạp: %s\n", state ? "BẬT" : "TẮT");
        emitEvent("manual_pump", String("Bơm nạp nước: ") + (state ? "BẬT" : "TẮT"));
    } else if (t == "drain" || t == "xa" || t == "ra") {
        Serial.printf("[Sampling] [Manual] Bơm xả: %s\n", state ? "BẬT" : "TẮT");
        emitEvent("manual_pump", String("Bơm xả nước: ") + (state ? "BẬT" : "TẮT"));
    }
}

void SamplingManager::measureSensor(SensorType type) {
    stepDuration = SENSOR_SAMPLE_TIME;

    switch (type) {
        case SENSOR_TEMP: {
            Serial.println("[Sampling] [Relay Temp ON] -> Đang cấp nguồn và đọc Cảm biến Nhiệt độ...");
            currentResult.temperature = simTemperature();
            currentResult.hasTemp = true;
            completedInCycle.push_back(SENSOR_TEMP);
            String extra = "\"sensor\":\"temperature\",\"value\":" + String(currentResult.temperature, 2) + ",\"unit\":\"°C\"";
            emitEvent("measuring_sensor", "Đang đo Nhiệt độ...", extra);
            break;
        }
        case SENSOR_PH: {
            Serial.println("[Sampling] [Relay pH ON] -> Đang cấp nguồn và đọc Cảm biến pH...");
            currentResult.ph = simPH();
            currentResult.hasPh = true;
            completedInCycle.push_back(SENSOR_PH);
            String extra = "\"sensor\":\"ph\",\"value\":" + String(currentResult.ph, 2) + ",\"unit\":\"pH\"";
            emitEvent("measuring_sensor", "Đang đo pH...", extra);
            break;
        }
        case SENSOR_TURBIDITY: {
            Serial.println("[Sampling] [Relay Turbidity ON] -> Đang cấp nguồn và đọc Cảm biến Độ đục...");
            currentResult.turbidity = simTurbidity();
            currentResult.hasTurbidity = true;
            completedInCycle.push_back(SENSOR_TURBIDITY);
            String extra = "\"sensor\":\"turbidity\",\"value\":" + String(currentResult.turbidity, 1) + ",\"unit\":\"NTU\"";
            emitEvent("measuring_sensor", "Đang đo Độ đục...", extra);
            break;
        }
        case SENSOR_TDS: {
            Serial.println("[Sampling] [Relay TDS ON] -> Đang cấp nguồn và đọc Cảm biến TDS...");
            currentResult.tds = simTDS();
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
        // Đã đo xong tất cả
        Serial.println("[Sampling] Hoàn tất đo tất cả các cảm biến trong lượt này.");
        emitEvent("measuring_complete", "Hoàn tất đo các cảm biến. Bắt đầu XẢ NƯỚC...");
        transitionTo(STATE_DRAINING);
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
        Serial.println("[Sampling] Tự động bắt đầu chu trình tiếp theo từ hàng đợi...");
        activeQueue = nextPendingQueue;
        nextPendingQueue.clear();
        completedInCycle.clear();
        currentResult = SensorResult();
        currentResult.startTime = millis();
        currentMeasuringIndex = -1;

        emitEvent("filling", "Bắt đầu chu trình đo mới cho hàng đợi kế tiếp...");
        transitionTo(STATE_FILLING);
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
            // Không làm gì khi rảnh
            break;

        case STATE_FILLING:
            // Mô phỏng bơm nước vào buồng đo đến khi phao báo đầy
            if (elapsed >= FILL_SIM_TIME) {
                Serial.println("[Sampling] [Phao Nước] -> Báo mức NƯỚC ĐẦY! Tắt bơm nạp.");
                emitEvent("filled", "Phao báo NƯỚC ĐẦY. Đang chờ nước ổn định...");
                transitionTo(STATE_STABILIZING);
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
            // Khi hết thời gian lấy mẫu của 1 cảm biến -> Tắt nguồn cảm biến đó và chuyển sang cảm biến tiếp theo
            if (elapsed >= stepDuration) {
                if (currentMeasuringIndex >= 0 && currentMeasuringIndex < (int)activeQueue.size()) {
                    SensorType prevType = activeQueue[currentMeasuringIndex];
                    Serial.printf("[Sampling] [Relay %s OFF] -> Đã ngắt nguồn cảm biến!\n", getSensorName(prevType).c_str());
                }
                processNextSensor();
            }
            break;

        case STATE_DRAINING:
            // Mô phỏng xả nước khỏi buồng đo đến khi cạn
            if (elapsed >= DRAIN_SIM_TIME) {
                Serial.println("[Sampling] [Phao Nước] -> Báo NƯỚC ĐÃ CẠN! Tắt bơm xả.");
                emitEvent("drained", "Đã xả nước xong.");
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
