#include "TdsManager.h"

#include <ArduinoJson.h>

#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"
#include "../mqtt/MQTTHandler.h"
#include "../ph/PhManager.h"

TdsManager tdsManager;

void TdsManager::begin() {
    pinMode(TDS_POWER_PIN, OUTPUT);
    digitalWrite(TDS_POWER_PIN, LOW);
    analogReadResolution(12);
    analogSetPinAttenuation(TDS_ADC_PIN, ADC_11db);
    LOGF("[TDS] A=GPIO%d VCC=GPIO%d. Nguồn tắt, chờ lệnh power.\n", TDS_ADC_PIN, TDS_POWER_PIN);
}

void TdsManager::writePower(bool on, uint32_t maxOnS) {
    digitalWrite(TDS_POWER_PIN, on ? HIGH : LOW);
    powered = on;
    onSince = millis();
    maxOnMs = on ? maxOnS * 1000UL : 0;
}

void TdsManager::publishEvent(const char *stage, const String &message) {
    LOGF("[TDS] %s: %s\n", stage, message.c_str());
    if (!mqttHandler.isConnected()) return;
    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = stage;
    doc["message"] = message;
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

void TdsManager::publishPowerEvent(const char *sensor, bool on, uint32_t maxOnS, const String &message, const char *stage) {
    LOGLN("[TDS] " + message);
    if (!mqttHandler.isConnected()) return;
    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = stage;
    doc["sensor"] = sensor;
    doc["power"] = on;
    if (on) doc["max_on_s"] = maxOnS;
    doc["message"] = message;
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

// sensor "tds" hoặc "all" (chỉ tắt). pH RS485 cấp điện liên tục nên không có lệnh nguồn cho pH.
void TdsManager::setPower(const String &sensor, bool on, uint32_t maxOnS) {
    if (busy) {
        publishEvent("command_error", "Đang đọc TDS, chưa đổi nguồn.");
        return;
    }
    if (sensor == "all") {
        if (on) {
            publishEvent("command_error", "Chỉ tắt được tất cả, bật từng cảm biến một.");
            return;
        }
        writePower(false, 0);
        publishPowerEvent("all", false, 0, "Tắt nguồn TDS.");
        return;
    }
    if (sensor != "tds") {
        publishEvent("command_error", "Bo này chỉ bật tắt nguồn tds; pH cấp điện liên tục.");
        return;
    }
    if (maxOnS == 0) maxOnS = DEFAULT_MAX_ON_S;
    if (maxOnS > MAX_MAX_ON_S) maxOnS = MAX_MAX_ON_S;
    writePower(on, maxOnS);
    publishPowerEvent("tds", on, on ? maxOnS : 0,
                      on ? "Bật nguồn TDS, tự tắt sau " + String(maxOnS) + "s." : String("Tắt nguồn TDS."));
}

void TdsManager::publishPowerStatus() {
    if (!mqttHandler.isConnected()) return;
    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = "power_status";
    doc["power"]["tds"] = powered;
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

void TdsManager::requestRead(int samples, int intervalMs) {
    if (busy || pending) {
        publishEvent("sensor_error", "Đang đọc TDS, chưa nhận thêm lệnh.");
        return;
    }
    pending = true;
    pendingSamples = samples;
    pendingIntervalMs = intervalMs;
}

void TdsManager::handle() {
    bool connected = mqttHandler.isConnected();
    if (mqttWasConnected && !connected && powered) {
        writePower(false, 0);
        LOGLN("[TDS] Mất MQTT, tắt nguồn TDS.");
    }
    mqttWasConnected = connected;

    if (powered && maxOnMs > 0 && millis() - onSince >= maxOnMs) {
        uint32_t limitS = maxOnMs / 1000UL;
        writePower(false, 0);
        publishPowerEvent("tds", false, 0, "Bật TDS quá " + String(limitS) + "s, tự tắt.", "power_timeout");
    }

    if (busy || !pending) return;
    pending = false;
    busy = true;
    run(pendingSamples, pendingIntervalMs);
    busy = false;
}

void TdsManager::waitMs(uint32_t ms) {
    uint32_t start = millis();
    while (millis() - start < ms) {
        mqttHandler.handle();
        remoteLog.handle();
        delay(10);
    }
}

static int clampInt(int v, int fallback, int lo, int hi) {
    if (v <= 0) v = fallback;
    return v < lo ? lo : (v > hi ? hi : v);
}

// Trung bình sau khi bỏ 20% mẫu thấp nhất và 20% cao nhất. Điện áp dùng analogReadMilliVolts (đã bù eFuse của C3).
void TdsManager::run(int samples, int intervalMs) {
    if (!powered) {
        publishEvent("sensor_error", "TDS chưa bật nguồn, gửi lệnh power trước.");
        return;
    }
    samples = clampInt(samples, DEFAULT_SAMPLES, 1, MAX_SAMPLES);
    intervalMs = clampInt(intervalMs, DEFAULT_INTERVAL_MS, 20, 1000);
    uint32_t started = millis();

    static int mv[MAX_SAMPLES];
    long adcSum = 0;
    for (int i = 0; i < samples; i++) {
        adcSum += analogRead(TDS_ADC_PIN);
        mv[i] = analogReadMilliVolts(TDS_ADC_PIN);
        if (i + 1 < samples) waitMs(intervalMs);
    }
    for (int i = 1; i < samples; i++) {
        int key = mv[i];
        int j = i - 1;
        while (j >= 0 && mv[j] > key) {
            mv[j + 1] = mv[j];
            j--;
        }
        mv[j + 1] = key;
    }
    int trim = samples / 5;
    long sum = 0;
    for (int i = trim; i < samples - trim; i++) sum += mv[i];
    float meanMv = (float)sum / (samples - 2 * trim);

    float tempC = NAN;
    bool hasTemp = phManager.readTempC(tempC);

    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["status"] = "success";
    doc["sensors_measured"].add("tds");
    JsonObject raw = doc["raw"]["tds"].to<JsonObject>();
    raw["adc"] = (int)lroundf((float)adcSum / samples);
    raw["voltage"] = meanMv / 1000.0f;
    raw["sample_count"] = samples;
    if (hasTemp) doc["raw"]["temp_c"] = tempC;
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    doc["duration_ms"] = (unsigned long)(millis() - started);
    String payload;
    serializeJson(doc, payload);
    LOGF("[TDS] %d mẫu, %.0f mV (min %d max %d)%s\n", samples, meanMv, mv[0], mv[samples - 1],
         hasTemp ? "" : ", không đọc được nhiệt độ");
    mqttHandler.publish("sensor_data", payload);
}
