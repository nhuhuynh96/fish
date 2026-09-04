#include <Arduino.h>
#include <WiFi.h>
#include <vector>
#include "config/ConfigManager.h"
#include "portal/WebPortal.h"
#include "mqtt/MQTTHandler.h"
#include "sampling/SamplingManager.h"

// Chân nút nhấn BOOT trên ESP32 để kích hoạt lại chế độ cấu hình
const int BUTTON_PIN = 0; // GPIO 0 (Nút BOOT có sẵn trên board ESP32)
const unsigned long BUTTON_HOLD_TIME = 3000; // Giữ 3 giây để vào chế độ cấu hình

unsigned long buttonPressStartTime = 0;
bool buttonIsPressed = false;
unsigned long lastHeartbeat = 0;

// Hàm phân tích lệnh từ MQTT nhận được
void handleMqttCommand(const String &msg) {
    String payload = msg;
    payload.trim();

    Serial.printf("[Command] Nhận payload: %s\n", payload.c_str());

    // Xử lý lệnh dạng JSON hoặc chuỗi văn bản đơn giản
    // 1. Lệnh đo lường (Measure)
    if (payload.indexOf("measure") >= 0 || payload.indexOf("do") >= 0) {
        std::vector<String> sensors;

        if (payload.indexOf("\"all\"") >= 0 || payload.indexOf("all") >= 0) {
            sensors.push_back("all");
        } else {
            if (payload.indexOf("temp") >= 0 || payload.indexOf("nhiet_do") >= 0) sensors.push_back("temp");
            if (payload.indexOf("ph") >= 0) sensors.push_back("ph");
            if (payload.indexOf("turbidity") >= 0 || payload.indexOf("turb") >= 0 || payload.indexOf("do_duc") >= 0) sensors.push_back("turbidity");
            if (payload.indexOf("tds") >= 0) sensors.push_back("tds");
        }

        // Nếu chỉ gửi "measure" hoặc "do" mà không chỉ định cảm biến -> Mặc định đo tất cả
        if (sensors.empty()) {
            sensors.push_back("all");
        }

        samplingManager.requestMeasurement(sensors);
    }
    // 2. Lệnh điều khiển bơm thủ công (Manual Pump)
    else if (payload.indexOf("pump") >= 0 || payload.indexOf("bom") >= 0) {
        bool state = (payload.indexOf("ON") >= 0 || payload.indexOf("on") >= 0 || payload.indexOf("1") >= 0 || payload.indexOf("bat") >= 0);
        String target = "inlet";
        if (payload.indexOf("drain") >= 0 || payload.indexOf("xa") >= 0) {
            target = "drain";
        }
        samplingManager.setManualPump(target, state);
    }
    // 3. Lệnh lấy trạng thái hiện tại (Status query)
    else if (payload.indexOf("status") >= 0) {
        String statusJson = "{\"state\":\"" + samplingManager.getStateName() + "\",\"is_busy\":" + (samplingManager.isBusy() ? "true" : "false") + "}";
        mqttHandler.publish("status", statusJson);
    }
    else {
        Serial.println("[Command] Lệnh không nhận diện được. Gợi ý: {\"action\":\"measure\",\"sensors\":[\"all\"]}");
    }
}

void setup() {
    Serial.begin(115200);
    delay(1000);

    Serial.println("\n==========================================");
    Serial.println("         ESP32 IOT FISH CONTROLLER        ");
    Serial.println("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);

    // Khởi tạo module lấy mẫu (Bật chế độ mô phỏng demo)
    samplingManager.begin(true);

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
            Serial.printf("[App] Lệnh nhận từ MQTT: %s\n", msg.c_str());

            // Chuyển cho bộ điều phối lệnh
            handleMqttCommand(msg);
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

    // Xử lý máy trạng thái lấy mẫu & đo chỉ số (Bơm -> Đo tuần tự -> Xả)
    samplingManager.handle();

    // Gửi tin nhắn định kỳ (Heartbeat telemetry) mỗi 30 giây
    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            String telemetry = "{\"rssi\":" + String(WiFi.RSSI()) + ",\"uptime\":" + String(millis() / 1000) + ",\"state\":\"" + samplingManager.getStateName() + "\"}";
            mqttHandler.publish("telemetry", telemetry);
            Serial.println("[Main] Đã gửi telemetry lên MQTT.");
        }
    }
}