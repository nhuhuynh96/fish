#include <Arduino.h>
#include <WiFi.h>
#include <ArduinoJson.h>
#include "config/ConfigManager.h"
#include "portal/WebPortal.h"
#include "mqtt/MQTTHandler.h"
#include "sensor/SensorManager.h"

const int BUTTON_PIN = 0; // Nút BOOT trên ESP32-S3 (GPIO 0)
const unsigned long BUTTON_HOLD_TIME = 3000;

unsigned long buttonPressStartTime = 0;
bool buttonIsPressed = false;
unsigned long lastHeartbeat = 0;

void handleMqttCommand(const String &msg) {
    String payload = msg;
    payload.trim();
    Serial.printf("[Command S3] Nhận payload: %s\n", payload.c_str());

    JsonDocument doc;
    if (deserializeJson(doc, payload)) {
        Serial.println("[Command S3] JSON không hợp lệ!");
        return;
    }

    String action = doc["action"] | "";
    action.toLowerCase();
    action.trim();

    if (action == "ph" || action == "test_ph") {
        sensorManager.measurePH();
    } else if (action == "temp" || action == "temperature" || action == "test_temp") {
        sensorManager.measureTemperature();
    } else if (action == "tds" || action == "test_tds") {
        sensorManager.measureTDS();
    } else if (action == "measure_all" || action == "test" || action == "all") {
        sensorManager.measureAll();
    } else {
        Serial.printf("[Command S3] Lệnh không hỗ trợ: %s\n", action.c_str());
    }
}

void checkButtonHold() {
    if (digitalRead(BUTTON_PIN) == LOW) {
        if (!buttonIsPressed) {
            buttonIsPressed = true;
            buttonPressStartTime = millis();
            Serial.println("[Button] Đang nhấn nút BOOT...");
        } else {
            if (millis() - buttonPressStartTime >= BUTTON_HOLD_TIME) {
                Serial.println("\n[Button] Đã giữ nút 3 giây -> Chuyển sang chế độ Access Point!");
                buttonIsPressed = false;
                webPortal.startPortal();
            }
        }
    } else {
        if (buttonIsPressed) {
            buttonIsPressed = false;
        }
    }
}

void setup() {
    Serial.begin(115200);
    delay(1000);

    Serial.println("\n==========================================");
    Serial.println("     ESP32-S3 SENSORS NODE (Standalone)   ");
    Serial.println("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);

    // Khởi tạo cảm biến
    sensorManager.begin();

    // Khởi tạo và đọc cấu hình từ NVS
    configManager.begin();
    bool hasConfig = configManager.loadConfig(currentConfig);

    // Kiểm tra nếu chưa cấu hình hoặc giữ nút BOOT lúc cắm nguồn
    if (!hasConfig || digitalRead(BUTTON_PIN) == LOW) {
        if (!hasConfig) {
            Serial.println("[Main S3] Chưa có cấu hình WiFi/MQTT. Bắt đầu phát Access Point...");
        } else {
            Serial.println("[Main S3] Phát hiện giữ nút BOOT. Bắt đầu phát Access Point...");
        }
        webPortal.startPortal();
        return;
    }

    // Kết nối WiFi
    Serial.printf("[Main S3] Đang kết nối WiFi: %s\n", currentConfig.wifi_ssid.c_str());
    WiFi.mode(WIFI_STA);
    WiFi.begin(currentConfig.wifi_ssid.c_str(), currentConfig.wifi_pass.c_str());

    unsigned long startAttempt = millis();
    const unsigned long WIFI_TIMEOUT = 15000;

    while (WiFi.status() != WL_CONNECTED && (millis() - startAttempt) < WIFI_TIMEOUT) {
        delay(500);
        Serial.print(".");
    }
    Serial.println();

    if (WiFi.status() == WL_CONNECTED) {
        Serial.println("[Main S3] WiFi đã kết nối thành công!");
        Serial.printf("[Main S3] IP Address: %s\n", WiFi.localIP().toString().c_str());

        // Khởi động MQTT
        mqttHandler.begin(currentConfig);
        mqttHandler.setCallback([](char* topic, byte* payload, unsigned int length) {
            String msg;
            for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
            Serial.printf("[App S3] Lệnh nhận từ MQTT: %s\n", msg.c_str());
            handleMqttCommand(msg);
        });
    } else {
        Serial.println("[Main S3] Kết nối WiFi thất bại! Tự động chuyển sang AP Mode...");
        webPortal.startPortal();
    }
}

void loop() {
    if (webPortal.isRunning()) {
        webPortal.handlePortal();
        return;
    }

    checkButtonHold();
    mqttHandler.handle();

    // Gửi Telemetry định kỳ 30 giây
    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            String telemetry = "{\"device_id\":\"" + currentConfig.device_id +
                               "\",\"rssi\":" + String(WiFi.RSSI()) +
                               ",\"uptime\":" + String(millis() / 1000) +
                               ",\"state\":\"READY\"}";
            mqttHandler.publish("telemetry", telemetry);
            Serial.println("[Main S3] Đã gửi telemetry lên MQTT.");
        }
    }
}
