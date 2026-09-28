#include "SensorManager.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"
#include <OneWire.h>
#include <DallasTemperature.h>

SensorManager sensorManager;

SensorManager::SensorManager() {}

void SensorManager::begin() {
    pinMode(TEMP_POWER_PIN, OUTPUT);
    pinMode(PH_POWER_PIN, OUTPUT);
    pinMode(TDS_POWER_PIN, OUTPUT);

    pinMode(PH_ADC_PIN, INPUT);
    pinMode(TDS_ADC_PIN, INPUT);

    analogReadResolution(12);
    analogSetPinAttenuation(PH_ADC_PIN, ADC_11db);
    analogSetPinAttenuation(TDS_ADC_PIN, ADC_11db);

    powerOffAll();
    Serial.println("[Sensor S3] Đã khởi tạo các chân cảm biến: DS18B20=GPIO4/5, pH=GPIO6/7, TDS=GPIO8/9");
}

void SensorManager::powerOffAll() {
    digitalWrite(TEMP_POWER_PIN, LOW);
    digitalWrite(PH_POWER_PIN, LOW);
    digitalWrite(TDS_POWER_PIN, LOW);
}

void SensorManager::setPower(uint8_t powerPin, bool enable) {
    if (enable) powerOffAll();
    digitalWrite(powerPin, enable ? HIGH : LOW);
    if (enable) {
        delay(POWER_SETTLE_MS);
    }
}

int SensorManager::medianFilter(int *buffer, int count) const {
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

RawReading SensorManager::readRawADC(uint8_t pin, int sampleCount) {
    const int maxSamples = 50;
    if (sampleCount > maxSamples) sampleCount = maxSamples;
    int samples[maxSamples];

    for (int i = 0; i < sampleCount; i++) {
        samples[i] = analogRead(pin);
        delay(20);
    }

    RawReading reading;
    reading.sampleCount = sampleCount;
    reading.adc = medianFilter(samples, sampleCount);
    reading.voltage = (reading.adc * ADC_VREF) / (float)ADC_MAX;

    Serial.printf("[ADC S3] GPIO%u | ADC=%d | V=%.3fV (Lấy %d mẫu)\n",
                  pin, reading.adc, reading.voltage, sampleCount);
    return reading;
}

float SensorManager::readDS18B20() {
    OneWire oneWire(TEMP_DATA_PIN);
    DallasTemperature probe(&oneWire);
    probe.begin();

    if (probe.getDeviceCount() == 0) {
        Serial.println("[DS18B20 S3] Không tìm thấy cảm biến trên GPIO 4!");
        return NAN;
    }

    probe.setWaitForConversion(false);
    probe.requestTemperatures();
    delay(800);
    float celsius = probe.getTempCByIndex(0);
    if (celsius == DEVICE_DISCONNECTED_C) return NAN;
    return celsius;
}

void SensorManager::publishSensorData(const String &jsonPayload) {
    Serial.println("\n[Sensor S3] >>> GỬI SENSOR DATA LÊN MQTT <<<");
    Serial.println(jsonPayload);
    mqttHandler.publish("sensor_data", jsonPayload);
}

void SensorManager::emitEvent(const String &stage, const String &message) {
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",\"stage\":\"" + stage + "\",\"message\":\"" + message + "\"}";
    mqttHandler.publish("event", payload);
}

void SensorManager::measureTemperature() {
    unsigned long start = millis();
    emitEvent("measuring", "Đang đo nhiệt độ DS18B20...");
    setPower(TEMP_POWER_PIN, true);
    float temp = readDS18B20();
    setPower(TEMP_POWER_PIN, false);

    if (isnan(temp)) {
        emitEvent("sensor_error", "Lỗi đọc DS18B20");
        return;
    }

    unsigned long duration = millis() - start;
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",\"timestamp\":" + String(millis() / 1000) +
                     ",\"status\":\"success\",\"duration_ms\":" + String(duration) +
                     ",\"sensors_measured\":[\"temperature\"],\"data\":{\"temperature\":" + String(temp, 2) + "}}";
    publishSensorData(payload);
}

void SensorManager::measurePH() {
    unsigned long start = millis();
    emitEvent("measuring", "Đang đo cảm biến pH...");
    setPower(PH_POWER_PIN, true);
    RawReading raw = readRawADC(PH_ADC_PIN, PH_SAMPLE_COUNT);
    setPower(PH_POWER_PIN, false);

    unsigned long duration = millis() - start;
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",\"timestamp\":" + String(millis() / 1000) +
                     ",\"status\":\"success\",\"duration_ms\":" + String(duration) +
                     ",\"sensors_measured\":[\"ph\"],\"raw\":{\"ph\":{\"adc\":" + String(raw.adc) +
                     ",\"voltage\":" + String(raw.voltage, 3) + ",\"sample_count\":" + String(raw.sampleCount) + "}}}";
    publishSensorData(payload);
}

void SensorManager::measureTDS() {
    unsigned long start = millis();
    emitEvent("measuring", "Đang đo cảm biến TDS...");
    setPower(TDS_POWER_PIN, true);
    RawReading raw = readRawADC(TDS_ADC_PIN, TDS_SAMPLE_COUNT);
    setPower(TDS_POWER_PIN, false);

    unsigned long duration = millis() - start;
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",\"timestamp\":" + String(millis() / 1000) +
                     ",\"status\":\"success\",\"duration_ms\":" + String(duration) +
                     ",\"sensors_measured\":[\"tds\"],\"raw\":{\"tds\":{\"adc\":" + String(raw.adc) +
                     ",\"voltage\":" + String(raw.voltage, 3) + ",\"sample_count\":" + String(raw.sampleCount) + "}}}";
    publishSensorData(payload);
}

void SensorManager::measureAll() {
    unsigned long start = millis();
    emitEvent("measuring", "Bắt đầu chu trình đo toàn bộ (Nhiệt độ -> pH -> TDS)...");

    // 1. Đo nhiệt độ
    setPower(TEMP_POWER_PIN, true);
    float temp = readDS18B20();
    setPower(TEMP_POWER_PIN, false);
    delay(200);

    // 2. Đo pH
    setPower(PH_POWER_PIN, true);
    RawReading phRaw = readRawADC(PH_ADC_PIN, PH_SAMPLE_COUNT);
    setPower(PH_POWER_PIN, false);
    delay(200);

    // 3. Đo TDS
    setPower(TDS_POWER_PIN, true);
    RawReading tdsRaw = readRawADC(TDS_ADC_PIN, TDS_SAMPLE_COUNT);
    setPower(TDS_POWER_PIN, false);

    unsigned long duration = millis() - start;

    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",\"timestamp\":" + String(millis() / 1000) +
                     ",\"status\":\"success\",\"duration_ms\":" + String(duration) +
                     ",\"sensors_measured\":[\"temperature\",\"ph\",\"tds\"],"
                     "\"raw\":{\"ph\":{\"adc\":" + String(phRaw.adc) + ",\"voltage\":" + String(phRaw.voltage, 3) + ",\"sample_count\":" + String(phRaw.sampleCount) + "},"
                     "\"tds\":{\"adc\":" + String(tdsRaw.adc) + ",\"voltage\":" + String(tdsRaw.voltage, 3) + ",\"sample_count\":" + String(tdsRaw.sampleCount) + "}},";
    
    if (!isnan(temp)) {
        payload += "\"data\":{\"temperature\":" + String(temp, 2) + "}";
    } else {
        payload += "\"data\":{}";
    }
    payload += "}";

    publishSensorData(payload);
}
