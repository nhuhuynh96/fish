#include <Arduino.h>
#include <WiFi.h>
#include "config/ConfigManager.h"
#include "portal/WebPortal.h"
#include "mqtt/MQTTHandler.h"

// Chân nút nhấn BOOT trên ESP32 để kích hoạt lại chế độ cấu hình
const int BUTTON_PIN = 0; // GPIO 0 (Nút BOOT có sẵn trên board ESP32)
const unsigned long BUTTON_HOLD_TIME = 3000; // Giữ 3 giây để vào chế độ cấu hình

unsigned long buttonPressStartTime = 0;
bool buttonIsPressed = false;
unsigned long lastHeartbeat = 0;

void setup() {
    Serial.begin(115200);
    delay(1000);

    Serial.println("\n==========================================");
    Serial.println("         ESP32 IOT FISH CONTROLLER        ");
    Serial.println("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);

    // Đọc cấu hình đã lưu
    bool hasConfig = configManager.loadConfig(currentConfig);

    // Kiểm tra nếu chưa cấu hình hoặc người dùng đang giữ nút BOOT khi cắm nguồn
    if (!hasConfig || digitalRead(BUTTON_PIN) == LOW) {
        if (!hasConfig) {
            Serial.println("[Main] Chưa có cấu hình WiFi/MQTT. Bắt đầu phát AP...");
        } else {
            Serial.println("[Main] Phát hiện giữ nút BOOT. Bắt đầu phát AP cấu hình...");
        }
        webPortal.startPortal();
        return;
    }

    // Tiến hành kết nối WiFi đã cấu hình
    Serial.printf("[Main] Đang kết nối WiFi: %s\n", currentConfig.wifi_ssid.c_str());
    WiFi.mode(WIFI_STA);
    WiFi.begin(currentConfig.wifi_ssid.c_str(), currentConfig.wifi_pass.c_str());

    unsigned long startAttempt = millis();
    const unsigned long WIFI_TIMEOUT = 15000; // Timeout 15 giây

    while (WiFi.status() != WL_CONNECTED && (millis() - startAttempt) < WIFI_TIMEOUT) {
        delay(500);
        Serial.print(".");
    }
    Serial.println();

    if (WiFi.status() == WL_CONNECTED) {
        Serial.println("[Main] WiFi đã kết nối thành công!");
        Serial.printf("[Main] IP Address: %s\n", WiFi.localIP().toString().c_str());

        // Khởi động MQTT Handler
        mqttHandler.begin(currentConfig);
        mqttHandler.setCallback([](char* topic, byte* payload, unsigned int length) {
            String msg;
            for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
            Serial.printf("[App] Lệnh nhận được: %s\n", msg.c_str());

            // Xử lý các lệnh điều khiển tại đây (Ví dụ: bật tắt cho ăn, đèn, máy bơm...)
        });
    } else {
        Serial.println("[Main] Kết nối WiFi thất bại (Sai pass hoặc mất sóng)!");
        Serial.println("[Main] Tự động chuyển sang chế độ AP cấu hình...");
        webPortal.startPortal();
    }
}

void checkButtonHold() {
    // Nút BOOT (GPIO 0) nhấn là mức LOW
    if (digitalRead(BUTTON_PIN) == LOW) {
        if (!buttonIsPressed) {
            buttonIsPressed = true;
            buttonPressStartTime = millis();
            Serial.println("[Button] Đang nhấn nút BOOT...");
        } else {
            if (millis() - buttonPressStartTime >= BUTTON_HOLD_TIME) {
                Serial.println("\n[Button] Đã giữ nút 3 giây -> Chuyển sang chế độ Cấu hình AP!");
                buttonIsPressed = false;
                
                // Bật chế độ cấu hình AP
                webPortal.startPortal();
            }
        }
    } else {
        if (buttonIsPressed) {
            buttonIsPressed = false;
            Serial.println("[Button] Đã thả nút BOOT.");
        }
    }
}

void loop() {
    // Nếu đang ở chế độ AP Portal cấu hình
    if (webPortal.isRunning()) {
        webPortal.handlePortal();
        return;
    }

    // Kiểm tra nút bấm để vào lại chế độ AP nếu cần
    checkButtonHold();

    // Xử lý kết nối và nhận lệnh MQTT
    mqttHandler.handle();

    // Gửi tin nhắn định kỳ (Heartbeat telemetry) mỗi 30 giây
    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            String telemetry = "{\"rssi\":" + String(WiFi.RSSI()) + ",\"uptime\":" + String(millis() / 1000) + "}";
            mqttHandler.publish("telemetry", telemetry);
            Serial.println("[Main] Đã gửi telemetry lên MQTT.");
        }
    }
}