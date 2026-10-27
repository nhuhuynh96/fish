#include "SensorManager.h"

#include <ArduinoJson.h>
#include <DallasTemperature.h>
#include <OneWire.h>

#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"
#include "../mqtt/MQTTHandler.h"

SensorManager sensorManager;

static OneWire oneWire(4);
static DallasTemperature tempSensor(&oneWire);

SensorManager::SensorManager() {}

void SensorManager::begin() {
    for (int i = 0; i < COUNT; i++) {
        pinMode(powerPin(i), OUTPUT);
        digitalWrite(powerPin(i), LOW);
    }

    analogReadResolution(12);
    analogSetPinAttenuation(PH_ADC_PIN, ADC_11db);
    analogSetPinAttenuation(TDS_ADC_PIN, ADC_11db);

    oneWire.begin(TEMP_DATA_PIN);
    tempSensor.setWaitForConversion(false);

    LOGLN("[Sensor] DS18B20 GPIO4/5 | pH GPIO6/7 | TDS GPIO8/9. Nguồn tắt, chờ lệnh power.");
}

int SensorManager::sensorIndex(const String &name) {
    if (name == "temp" || name == "temperature") return TEMP;
    if (name == "ph") return PH;
    if (name == "tds") return TDS;
    return -1;
}

uint8_t SensorManager::powerPin(int idx) {
    switch (idx) {
    case TEMP: return TEMP_POWER_PIN;
    case PH: return PH_POWER_PIN;
    default: return TDS_POWER_PIN;
    }
}

const char *SensorManager::sensorKey(int idx) {
    switch (idx) {
    case TEMP: return "temp";
    case PH: return "ph";
    default: return "tds";
    }
}

const char *SensorManager::sensorLabel(int idx) {
    switch (idx) {
    case TEMP: return "nhiệt độ";
    case PH: return "pH";
    default: return "TDS";
    }
}

void SensorManager::request(const String &action, int samples, int intervalMs) {
    if (busy || pending.length() > 0) {
        publishEvent("sensor_error", "Đang đo, chưa nhận thêm lệnh.");
        return;
    }
    pending = action;
    pending.toLowerCase();
    pending.trim();
    pendingSamples = samples;
    pendingIntervalMs = intervalMs;
}

void SensorManager::handle() {
    bool connected = mqttHandler.isConnected();
    if (mqttWasConnected && !connected) {
        allOff("Mất MQTT, tắt nguồn mọi cảm biến.", false);
    }
    mqttWasConnected = connected;
    checkPowerTimeouts();

    if (busy || pending.length() == 0) return;
    String action = pending;
    pending = "";
    busy = true;
    run(action, pendingSamples, pendingIntervalMs);
    busy = false;
}

void SensorManager::setPower(const String &sensor, bool on, uint32_t maxOnS) {
    if (busy) {
        publishEvent("command_error", "Đang đo, chưa đổi nguồn.");
        return;
    }
    if (sensor == "all") {
        if (on) {
            publishEvent("command_error", "Chỉ tắt được tất cả, bật từng cảm biến một.");
            return;
        }
        allOff("Tắt nguồn mọi cảm biến.", true);
        return;
    }

    int idx = sensorIndex(sensor);
    if (idx < 0) {
        publishEvent("command_error", "Sensor không hỗ trợ: " + sensor);
        return;
    }
    // pH và TDS cùng có điện trong một khối nước sẽ đẩy dòng qua nước, làm lệch pH và mòn điện cực.
    if (on && idx == PH && powered[TDS]) {
        publishEvent("command_error", "TDS đang bật. Tắt TDS trước khi bật pH.");
        return;
    }
    if (on && idx == TDS && powered[PH]) {
        publishEvent("command_error", "pH đang bật. Tắt pH trước khi bật TDS.");
        return;
    }

    if (maxOnS == 0) maxOnS = DEFAULT_MAX_ON_S;
    if (maxOnS > MAX_MAX_ON_S) maxOnS = MAX_MAX_ON_S;
    writePower(idx, on, maxOnS);
    String label = sensorLabel(idx);
    String message = on ? "Bật nguồn " + label + ", tự tắt sau " + String(maxOnS) + "s." : "Tắt nguồn " + label + ".";
    publishPowerEvent(sensorKey(idx), on, on ? maxOnS : 0, message);
}

void SensorManager::writePower(int idx, bool on, uint32_t maxOnS) {
    digitalWrite(powerPin(idx), on ? HIGH : LOW);
    powered[idx] = on;
    onSince[idx] = millis();
    maxOnMs[idx] = on ? maxOnS * 1000UL : 0;
}

void SensorManager::allOff(const String &reason, bool notify) {
    for (int i = 0; i < COUNT; i++) writePower(i, false, 0);
    LOGLN("[Sensor] " + reason);
    if (notify) publishPowerEvent("all", false, 0, reason);
}

void SensorManager::checkPowerTimeouts() {
    uint32_t now = millis();
    for (int i = 0; i < COUNT; i++) {
        if (!powered[i] || maxOnMs[i] == 0) continue;
        if (now - onSince[i] < maxOnMs[i]) continue;
        uint32_t limitS = maxOnMs[i] / 1000UL;
        writePower(i, false, 0);
        String message = "Bật " + String(sensorLabel(i)) + " quá " + String(limitS) + "s, tự tắt.";
        LOGLN("[Sensor] " + message);
        if (!mqttHandler.isConnected()) continue;
        JsonDocument doc;
        doc["device_id"] = currentConfig.device_id;
        doc["stage"] = "power_timeout";
        doc["sensor"] = sensorKey(i);
        doc["power"] = false;
        doc["message"] = message;
        doc["timestamp"] = (unsigned long)(millis() / 1000);
        String payload;
        serializeJson(doc, payload);
        mqttHandler.publish("event", payload);
    }
}

String SensorManager::powerJson() const {
    String out = "{";
    for (int i = 0; i < COUNT; i++) {
        if (i > 0) out += ",";
        out += "\"" + String(sensorKey(i)) + "\":" + (powered[i] ? "true" : "false");
    }
    return out + "}";
}

void SensorManager::publishPowerStatus() {
    if (!mqttHandler.isConnected()) return;
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"stage\":\"power_status\",";
    payload += "\"power\":" + powerJson() + ",";
    payload += "\"timestamp\":" + String((unsigned long)(millis() / 1000)) + "}";
    mqttHandler.publish("event", payload);
}

void SensorManager::publishPowerEvent(const String &sensor, bool on, uint32_t maxOnS, const String &message) {
    LOGLN("[Sensor] " + message);
    if (!mqttHandler.isConnected()) return;
    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = "power";
    doc["sensor"] = sensor;
    doc["power"] = on;
    if (on) doc["max_on_s"] = maxOnS;
    doc["message"] = message;
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

void SensorManager::waitMs(uint32_t ms) {
    uint32_t start = millis();
    while (millis() - start < ms) {
        mqttHandler.handle();
        remoteLog.handle();
        delay(10);
    }
}

void SensorManager::publishEvent(const String &stage, const String &message) {
    LOGF("[Sensor] %s: %s\n", stage.c_str(), message.c_str());
    if (!mqttHandler.isConnected()) return;
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"stage\":\"" + stage + "\",";
    payload += "\"message\":\"" + message + "\",";
    payload += "\"timestamp\":" + String((unsigned long)(millis() / 1000)) + "}";
    mqttHandler.publish("event", payload);
}

void SensorManager::publishSensorData(const String &json) {
    LOGF("[Sensor] sensor_data %s\n", json.c_str());
    mqttHandler.publish("sensor_data", json);
}

bool SensorManager::readTemperature(float &celsius) {
    tempSensor.begin();
    if (tempSensor.getDeviceCount() < 1) return false;

    // Đọc trước khi chuyển đổi xong sẽ nhận +85°C (giá trị reset).
    tempSensor.requestTemperatures();
    waitMs(TEMP_CONVERSION_MS);

    float value = tempSensor.getTempCByIndex(0);
    if (value == DEVICE_DISCONNECTED_C) return false;
    celsius = value;
    return true;
}

// Trung bình sau khi bỏ 20% mẫu thấp nhất và 20% mẫu cao nhất.
bool SensorManager::readAnalog(const char *name, uint8_t adcPin, int samples, int intervalMs, int &adc, float &voltage) {
    static int buf[MAX_SAMPLES];
    for (int i = 0; i < samples; i++) {
        buf[i] = analogRead(adcPin);
        if (i + 1 < samples) waitMs(intervalMs);
    }

    for (int i = 1; i < samples; i++) {
        int key = buf[i];
        int j = i - 1;
        while (j >= 0 && buf[j] > key) {
            buf[j + 1] = buf[j];
            j--;
        }
        buf[j + 1] = key;
    }

    int trim = samples / 5;
    long sum = 0;
    for (int i = trim; i < samples - trim; i++) sum += buf[i];
    float mean = (float)sum / (samples - 2 * trim);

    adc = (int)lroundf(mean);
    voltage = (mean * ADC_VREF) / ADC_MAX;
    LOGF("[Sensor] %s %d mẫu, ADC min=%d max=%d trung bình=%.1f\n", name, samples, buf[0], buf[samples - 1], mean);
    return true;
}

static void addAnalog(JsonDocument &doc, const char *key, int adc, float voltage, int samples) {
    JsonObject raw = doc["raw"][key].to<JsonObject>();
    raw["adc"] = adc;
    raw["voltage"] = voltage;
    raw["sample_count"] = samples;
    doc["sensors_measured"].add(key);
}

static int clampInt(int value, int fallback, int lo, int hi) {
    if (value <= 0) value = fallback;
    if (value < lo) value = lo;
    if (value > hi) value = hi;
    return value;
}

void SensorManager::run(const String &action, int samples, int intervalMs) {
    bool wantTemp = action == "temp" || action == "temperature" || action == "test_temp" || action == "test_temperature";
    bool wantPh = action == "ph" || action == "test_ph";
    bool wantTds = action == "tds" || action == "test_tds";
    bool wantAll = action == "measure_all" || action == "measure" || action == "test" || action == "all";

    if (!wantTemp && !wantPh && !wantTds && !wantAll) {
        publishEvent("command_error", "Action không được hỗ trợ: " + action);
        return;
    }
    if (wantAll) {
        wantTemp = powered[TEMP];
        wantPh = powered[PH];
        wantTds = powered[TDS];
        if (!wantTemp && !wantPh && !wantTds) {
            publishEvent("sensor_error", "Chưa bật nguồn cảm biến nào, gửi lệnh power trước.");
            return;
        }
    } else {
        int idx = wantTemp ? TEMP : (wantPh ? PH : TDS);
        if (!powered[idx]) {
            publishEvent("sensor_error", String(sensorLabel(idx)) + " chưa bật nguồn, gửi lệnh power trước.");
            return;
        }
    }

    int interval = clampInt(intervalMs, DEFAULT_INTERVAL_MS, MIN_INTERVAL_MS, MAX_INTERVAL_MS);

    publishEvent("measuring", "Đang đo " + action);
    uint32_t started = millis();

    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["status"] = "success";
    bool any = false;
    if (wantTemp) {
        float celsius = 0;
        if (readTemperature(celsius)) {
            doc["data"]["temperature"] = celsius;
            doc["sensors_measured"].add("temperature");
            any = true;
            LOGF("[Sensor] Nhiệt độ %.2f C\n", celsius);
        } else {
            publishEvent("sensor_error", "Lỗi đo nhiệt độ: không đọc được DS18B20");
            if (!wantPh && !wantTds) return;
        }
    }

    if (wantPh) {
        int n = clampInt(samples, PH_SAMPLES, 1, MAX_SAMPLES);
        int adc = 0;
        float voltage = 0;
        if (readAnalog("pH", PH_ADC_PIN, n, interval, adc, voltage)) {
            addAnalog(doc, "ph", adc, voltage, n);
            any = true;
            LOGF("[Sensor] pH ADC=%d V=%.3f\n", adc, voltage);
        }
    }

    if (wantTds) {
        int n = clampInt(samples, TDS_SAMPLES, 1, MAX_SAMPLES);
        int adc = 0;
        float voltage = 0;
        if (readAnalog("TDS", TDS_ADC_PIN, n, interval, adc, voltage)) {
            addAnalog(doc, "tds", adc, voltage, n);
            any = true;
            LOGF("[Sensor] TDS ADC=%d V=%.3f\n", adc, voltage);
        }
    }

    if (!any) {
        publishEvent("sensor_error", "Không đo được cảm biến nào");
        return;
    }

    doc["timestamp"] = (unsigned long)(millis() / 1000);
    doc["duration_ms"] = (unsigned long)(millis() - started);
    String payload;
    serializeJson(doc, payload);
    publishSensorData(payload);
}
