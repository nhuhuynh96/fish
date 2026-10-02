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
    pinMode(TEMP_POWER_PIN, OUTPUT);
    pinMode(PH_POWER_PIN, OUTPUT);
    pinMode(TDS_POWER_PIN, OUTPUT);
    digitalWrite(TEMP_POWER_PIN, LOW);
    digitalWrite(PH_POWER_PIN, LOW);
    digitalWrite(TDS_POWER_PIN, LOW);

    analogReadResolution(12);
    analogSetPinAttenuation(PH_ADC_PIN, ADC_11db);
    analogSetPinAttenuation(TDS_ADC_PIN, ADC_11db);

    oneWire.begin(TEMP_DATA_PIN);
    tempSensor.setWaitForConversion(false);

    LOGLN("[Sensor] DS18B20 GPIO4/5 | pH GPIO6/7 | TDS GPIO8/9");
}

void SensorManager::request(const String &action) {
    if (busy || pending.length() > 0) {
        publishEvent("sensor_error", "Đang đo, chưa nhận thêm lệnh.");
        return;
    }
    pending = action;
    pending.toLowerCase();
    pending.trim();
}

void SensorManager::handle() {
    if (busy || pending.length() == 0) return;
    String action = pending;
    pending = "";
    busy = true;
    run(action);
    busy = false;
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

int SensorManager::median(int *samples, int count) {
    for (int i = 1; i < count; i++) {
        int key = samples[i];
        int j = i - 1;
        while (j >= 0 && samples[j] > key) {
            samples[j + 1] = samples[j];
            j--;
        }
        samples[j + 1] = key;
    }
    return samples[count / 2];
}

bool SensorManager::readTemperature(float &celsius) {
    digitalWrite(TEMP_POWER_PIN, HIGH);
    waitMs(TEMP_SETTLE_MS);

    tempSensor.begin();
    int found = tempSensor.getDeviceCount();
    if (found < 1) {
        digitalWrite(TEMP_POWER_PIN, LOW);
        return false;
    }

    // GPIO 5 vẫn HIGH suốt lúc chuyển đổi. Đọc sớm sẽ nhận +85°C (giá trị reset).
    tempSensor.requestTemperatures();
    waitMs(TEMP_CONVERSION_MS);

    float value = tempSensor.getTempCByIndex(0);
    digitalWrite(TEMP_POWER_PIN, LOW);
    if (value == DEVICE_DISCONNECTED_C) return false;
    celsius = value;
    return true;
}

bool SensorManager::readAnalog(uint8_t adcPin, uint8_t powerPin, int samples, int &adc, float &voltage) {
    digitalWrite(powerPin, HIGH);
    waitMs(ANALOG_SETTLE_MS);

    int buf[40];
    if (samples > 40) samples = 40;
    for (int i = 0; i < samples; i++) {
        buf[i] = analogRead(adcPin);
        waitMs(SAMPLE_INTERVAL_MS);
    }
    digitalWrite(powerPin, LOW);

    adc = median(buf, samples);
    voltage = (adc * ADC_VREF) / ADC_MAX;
    return true;
}

static void addAnalog(JsonDocument &doc, const char *key, int adc, float voltage, int samples) {
    JsonObject raw = doc["raw"][key].to<JsonObject>();
    raw["adc"] = adc;
    raw["voltage"] = voltage;
    raw["sample_count"] = samples;
    doc["sensors_measured"].add(key);
}

void SensorManager::run(const String &action) {
    bool wantTemp = action == "temp" || action == "temperature" || action == "test_temp" || action == "test_temperature";
    bool wantPh = action == "ph" || action == "test_ph";
    bool wantTds = action == "tds" || action == "test_tds";
    bool wantAll = action == "measure_all" || action == "measure" || action == "test" || action == "all";

    if (!wantTemp && !wantPh && !wantTds && !wantAll) {
        publishEvent("command_error", "Action không được hỗ trợ: " + action);
        return;
    }
    if (wantAll) {
        wantTemp = true;
        wantPh = true;
        wantTds = true;
    }

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
        if (wantPh || wantTds) waitMs(200);
    }

    if (wantPh) {
        int adc = 0;
        float voltage = 0;
        if (readAnalog(PH_ADC_PIN, PH_POWER_PIN, PH_SAMPLES, adc, voltage)) {
            addAnalog(doc, "ph", adc, voltage, PH_SAMPLES);
            any = true;
            LOGF("[Sensor] pH ADC=%d V=%.3f\n", adc, voltage);
        }
        if (wantTds) waitMs(200);
    }

    if (wantTds) {
        int adc = 0;
        float voltage = 0;
        if (readAnalog(TDS_ADC_PIN, TDS_POWER_PIN, TDS_SAMPLES, adc, voltage)) {
            addAnalog(doc, "tds", adc, voltage, TDS_SAMPLES);
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
