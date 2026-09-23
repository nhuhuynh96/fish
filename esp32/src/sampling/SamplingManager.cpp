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
    pinMode(TDS_SENSOR_PIN, INPUT);
    analogReadResolution(12);
    analogSetPinAttenuation(TDS_SENSOR_PIN, ADC_11db);
    pinMode(TURBIDITY_SENSOR_PIN, INPUT);
    analogSetPinAttenuation(TURBIDITY_SENSOR_PIN, ADC_11db);
    pinMode(PH_SENSOR_PIN, INPUT);
    analogSetPinAttenuation(PH_SENSOR_PIN, ADC_11db);
    pinMode(PH_POWER_PIN, OUTPUT);
    pinMode(TURBIDITY_POWER_PIN, OUTPUT);
    pinMode(TDS_POWER_PIN, OUTPUT);
    powerOffAllSensors();
    LOGLN("[Sampling] Relay GPIO18/19 | Phao 1 thấp=GPIO17 | Phao 2 cao=GPIO4");
    LOGLN("[Sampling] Queue MQTT: ph|turb → phao 1 (đầy phao 2 thì xả xuống); tds → phao 2; inlet|drain");
    LOGF("[Phao] lúc khởi động GPIO17=%d (%s) GPIO4=%d (%s)\n",
         digitalRead(FLOAT_LOW_PIN), isWaterLow() ? "phao1 ĐỦ" : "phao1 chưa",
         digitalRead(FLOAT_HIGH_PIN), isWaterHigh() ? "phao2 ĐỦ" : "phao2 chưa");
}

String SamplingManager::getStateName() const {
    if (isInletOn()) return "FILLING_WATER";
    if (isDrainOn()) return "DRAINING_WATER";
    if (!commandQueue.empty()) return "BUSY";
    return "IDLE";
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

void SamplingManager::stopHardware() {
    setInletPump(false);
    setDrainValve(false);
    powerOffAllSensors();
}

FillLevel SamplingManager::requiredLevel(const String &action, FillLevel parsed) const {
    if (action == "ph" || action == "turb") return FILL_LEVEL_LOW;
    if (action == "tds") return FILL_LEVEL_HIGH;
    return parsed;
}

void SamplingManager::enqueue(const PendingCommand &cmd) {
    PendingCommand next = cmd;
    next.action.toLowerCase();
    next.action.trim();
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
    fillLevel = requiredLevel(head.action, head.fillLevel);
}

void SamplingManager::startInlet() {
    setInletPump(true);
    inletSince = millis();
    if (inletAttempt == 0) inletAttempt = 1;
    emitEvent("manual_pump", String("Bơm nạp: BẬT (") + fillLevelName(fillLevel) + ")");
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
            // setInletPump(false);ß
            // inletAttempt = 0;
            // inletSince = 0;
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
    // if (isWaterLow()) {
    //     emitEvent("draining", "Đủ phao 1. Không xả nữa.");
    //     return;
    // }
    setDrainValve(true);
    drainSince = millis();
    emitEvent("draining", "Van xả: BẬT");
}

void SamplingManager::handlePump() {
    applyHeadFillLevel();
    unsigned long now = millis();
    unsigned long onFor = now - inletSince;
    if (isInletOn()) {
        // 1s đầu: nhiễu inrush, không tin phao. Sau đó phải ACTIVE ổn định 800ms.
        if (onFor < PUMP_FLOAT_GRACE_MS) {
            floatLowSince = 0;
            floatHighSince = 0;
        } else if (fillLevel == FILL_LEVEL_LOW) {
            if (isWaterLow()) {
                if (floatLowSince == 0) {
                    floatLowSince = now;
                    LOGF("[Phao] debounce phao1 gpio17=%d gpio4=%d\n",
                         digitalRead(FLOAT_LOW_PIN), digitalRead(FLOAT_HIGH_PIN));
                }
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
                if (floatHighSince == 0) {
                    floatHighSince = now;
                    LOGF("[Phao] debounce phao2 gpio17=%d gpio4=%d\n",
                         digitalRead(FLOAT_LOW_PIN), digitalRead(FLOAT_HIGH_PIN));
                }
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
    if (commandQueue.empty() && isWaterLow() && !isInletOn() && !isDrainOn()) {
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

bool SamplingManager::measureAfterFill(FillLevel level, SensorType type) {
    bool pumping = isInletOn();
    bool full = (level == FILL_LEVEL_LOW) ? isWaterLow() : isWaterHigh();
    if (pumping) {
        unsigned long now = millis();
        if (now - inletSince < PUMP_FLOAT_GRACE_MS) {
            return false;
        }
        unsigned long since = (level == FILL_LEVEL_LOW) ? floatLowSince : floatHighSince;
        full = full && since != 0 && (now - since >= FLOAT_DEBOUNCE_TIME);
    }
    if (full) {
        measureSensor(type);
        setInletPump(false);
        return true;
    }
    if (!pumping) {
        hanldeInletOn(level);
    }
    return false;
}

void SamplingManager::processHead() {
    if (commandQueue.empty()) return;

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
        if (measureAfterFill(FILL_LEVEL_LOW, SENSOR_PH)) finishCommand();
        return;
    }
    if (action == "turb") {
        if (measureAfterFill(FILL_LEVEL_LOW, SENSOR_TURBIDITY)) finishCommand();
        return;
    }
    if (action == "tds") {
        if (measureAfterFill(FILL_LEVEL_HIGH, SENSOR_TDS)) finishCommand();
        return;
    }
    emitEvent("command_error", "action không hỗ trợ: " + action);
    finishCommand();
}

void SamplingManager::handle() {
    handlePump();
    processHead();
}

void SamplingManager::measureSensor(SensorType type) {
    measureStartedAt = millis();
    RawReading raw;
    if (type == SENSOR_PH) {
        setSensorPower(SENSOR_PH, true);
        raw = readRawADC(PH_SENSOR_PIN, PH_SAMPLE_COUNT);
        setSensorPower(SENSOR_PH, false);
    } else if (type == SENSOR_TURBIDITY) {
        setSensorPower(SENSOR_TURBIDITY, true);
        raw = readRawADC(TURBIDITY_SENSOR_PIN, TURBIDITY_SAMPLE_COUNT);
        setSensorPower(SENSOR_TURBIDITY, false);
    } else if (type == SENSOR_TDS) {
        setSensorPower(SENSOR_TDS, true);
        raw = readRawADC(TDS_SENSOR_PIN, TDS_SAMPLE_COUNT);
        setSensorPower(SENSOR_TDS, false);
    } else {
        return;
    }
    publishSensorReading(type, raw);
}

void SamplingManager::publishSensorReading(SensorType type, const RawReading &raw) {
    unsigned long durationMs = millis() - measureStartedAt;
    String name = getSensorName(type);
    String payload = "{";
    payload += "\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"timestamp\":" + String(millis() / 1000) + ",";
    payload += "\"status\":\"success\",";
    payload += "\"duration_ms\":" + String(durationMs) + ",";
    payload += "\"sensors_measured\":[\"" + name + "\"],";
    payload += "\"raw\":{\"" + name + "\":" + rawReadingJson(raw) + "}";
    payload += "}";

    LOGLN("\n[Sampling] >>> GỬI sensor_data <<<");
    LOGLN(payload);
    if (mqttHandler.isConnected()) {
        mqttHandler.publish("sensor_data", payload);
    }
    String extra = "\"sensor\":\"" + name + "\",\"adc\":" + String(raw.adc) +
                   ",\"voltage\":" + String(raw.voltage, 3);
    emitEvent("measuring_sensor", "Đã đo " + name + " raw, đã gửi MQTT.", extra);
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
