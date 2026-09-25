#include <Arduino.h>
#include <WiFi.h>
#include <ArduinoJson.h>
#include "config/ConfigManager.h"
#include "portal/WebPortal.h"
#include "mqtt/MQTTHandler.h"
#include "sampling/SamplingManager.h"
#include "log/RemoteLog.h"

// Chân nút nhấn BOOT trên ESP32 để kích hoạt lại chế độ cấu hình
const int BUTTON_PIN = 0; // GPIO 0 (Nút BOOT có sẵn trên board ESP32)
const unsigned long BUTTON_HOLD_TIME = 3000; // Giữ 3 giây để vào chế độ cấu hình

unsigned long buttonPressStartTime = 0;
bool buttonIsPressed = false;
unsigned long lastHeartbeat = 0;

static void publishCommandError(const String &message) {
    LOGLN("[Command] " + message);
    if (mqttHandler.isConnected()) {
        String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
        payload += "\"stage\":\"command_error\",";
        payload += "\"state\":\"" + samplingManager.getStateName() + "\",";
        payload += "\"message\":\"" + message + "\"}";
        mqttHandler.publish("event", payload);
    }
}

static FillLevel parseCommandLevel(JsonDocument &doc) {
    if (!doc["level"].isNull()) {
        if (doc["level"].is<const char *>()) {
            String s = doc["level"].as<const char *>();
            s.toLowerCase();
            s.trim();
            if (s == "high" || s == "hight" || s == "cao" || s == "2") {
                return FILL_LEVEL_HIGH;
            }
            return FILL_LEVEL_LOW;
        }
        int n = doc["level"].as<int>();
        return n >= 2 ? FILL_LEVEL_HIGH : FILL_LEVEL_LOW;
    }
    if (!doc["fillLevel"].isNull()) {
        return doc["fillLevel"].as<int>() >= 1 ? FILL_LEVEL_HIGH : FILL_LEVEL_LOW;
    }
    if (!doc["FillLevel"].isNull()) {
        return doc["FillLevel"].as<int>() >= 1 ? FILL_LEVEL_HIGH : FILL_LEVEL_LOW;
    }
    return FILL_LEVEL_LOW;
}

// {"action":"ph"|"temp"|"tds"|"test"|"test_ph"|"test_temp"|"test_tds"|"inlet_on"|"inlet_off"|"drain_on"|"drain_off","level":1|2}
void handleMqttCommand(const String &msg) {
    String payload = msg;
    payload.trim();
    LOGF("[Command] Nhận payload: %s\n", payload.c_str());

    JsonDocument doc;
    if (deserializeJson(doc, payload)) {
        publishCommandError("JSON không hợp lệ. Ví dụ: {\"action\":\"ph\"} hoặc {\"action\":\"inlet\",\"level\":1}");
        return;
    }

    String action = doc["action"] | "";
    action.toLowerCase();
    action.trim();
    if (action.length() == 0) {
        publishCommandError("Thiếu action.");
        return;
    }

    if (action == "inlet_off") {
        samplingManager.hanldeInletOff();
        return;
    }

    if (action == "drain_off") {
        samplingManager.hanldeDrainOff();
        return;
    }

    if (action == "clear_queue") {
        samplingManager.clearQueue();
        return;
    }

    PendingCommand cmd;
    cmd.action = action;
    cmd.fillLevel = parseCommandLevel(doc);
    samplingManager.enqueue(cmd);
}

void setup() {
    Serial.begin(115200);
    delay(1000);

    LOGLN("\n==========================================");
    LOGLN("         ESP32 IOT FISH CONTROLLER        ");
    LOGLN("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);

    // Khởi tạo SamplingManager (phần cứng thật)
    samplingManager.begin();

    // Đọc cấu hình đã lưu
    bool hasConfig = configManager.loadConfig(currentConfig);

    // Kiểm tra nếu chưa cấu hình hoặc người dùng đang giữ nút BOOT khi cắm nguồn
    if (!hasConfig || digitalRead(BUTTON_PIN) == LOW) {
        if (!hasConfig) {
            LOGLN("[Main] Chưa có cấu hình WiFi/MQTT. Bắt đầu phát AP...");
        } else {
            LOGLN("[Main] Phát hiện giữ nút BOOT. Bắt đầu phát AP cấu hình...");
        }
        webPortal.startPortal();
        return;
    }

    // Tiến hành kết nối WiFi đã cấu hình
    LOGF("[Main] Đang kết nối WiFi: %s\n", currentConfig.wifi_ssid.c_str());
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
        LOGLN("[Main] WiFi đã kết nối thành công!");
        LOGF("[Main] IP Address: %s\n", WiFi.localIP().toString().c_str());

        // Khởi động MQTT Handler
        mqttHandler.begin(currentConfig);
        mqttHandler.setCallback([](char* topic, byte* payload, unsigned int length) {
            String msg;
            for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
            LOGF("[App] Lệnh nhận từ MQTT: %s\n", msg.c_str());

            // Chuyển cho bộ điều phối lệnh
            handleMqttCommand(msg);
        });
    } else {
        LOGLN("[Main] Kết nối WiFi thất bại (Sai pass hoặc mất sóng)!");
        LOGLN("[Main] Tự động chuyển sang chế độ AP cấu hình...");
        webPortal.startPortal();
    }
}

void checkButtonHold() {
    // Nút BOOT (GPIO 0) nhấn là mức LOW
    if (digitalRead(BUTTON_PIN) == LOW) {
        if (!buttonIsPressed) {
            buttonIsPressed = true;
            buttonPressStartTime = millis();
            LOGLN("[Button] Đang nhấn nút BOOT...");
        } else {
            if (millis() - buttonPressStartTime >= BUTTON_HOLD_TIME) {
                LOGLN("\n[Button] Đã giữ nút 3 giây -> Chuyển sang chế độ Cấu hình AP!");
                buttonIsPressed = false;
                
                // Bật chế độ cấu hình AP
                webPortal.startPortal();
            }
        }
    } else {
        if (buttonIsPressed) {
            buttonIsPressed = false;
            LOGLN("[Button] Đã thả nút BOOT.");
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

    // Đẩy Serial log đã xếp hàng lên MQTT topic fish/<id>/log
    remoteLog.handle();

    // Xử lý máy trạng thái lấy mẫu & đo chỉ số (Bơm -> Đo tuần tự -> Xả)
    samplingManager.handle();

    // Gửi tin nhắn định kỳ (Heartbeat telemetry) mỗi 30 giây
    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            String telemetry = "{\"rssi\":" + String(WiFi.RSSI()) + ",\"uptime\":" + String(millis() / 1000) + ",\"state\":\"" + samplingManager.getStateName() + "\",\"queue_size\":" + String((unsigned)samplingManager.queueSize()) + "}";
            mqttHandler.publish("telemetry", telemetry);
            LOGLN("[Main] Đã gửi telemetry lên MQTT.");
        }
    }
}
