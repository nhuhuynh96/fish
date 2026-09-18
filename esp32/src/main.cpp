#include <Arduino.h>
#include <WiFi.h>
#include <ArduinoJson.h>
#include <vector>
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

static bool parsePumpState(JsonVariantConst stateVar, bool &outState) {
    if (stateVar.is<bool>()) {
        outState = stateVar.as<bool>();
        return true;
    }

    if (stateVar.is<int>() || stateVar.is<long>() || stateVar.is<float>()) {
        outState = stateVar.as<int>() != 0;
        return true;
    }

    if (stateVar.is<const char*>()) {
        String s = stateVar.as<const char*>();
        s.toLowerCase();
        s.trim();
        if (s == "on" || s == "1" || s == "true" || s == "bat") {
            outState = true;
            return true;
        }
        if (s == "off" || s == "0" || s == "false" || s == "tat" || s == "of") {
            outState = false;
            return true;
        }
    }

    return false;
}

// {"action":"pump","target":"inlet"|"drain","state":"ON"|"OFF"}
// {"action":"status"}
// {"action":"clear_queue"}
void handleMqttCommand(const String &msg) {
    String payload = msg;
    payload.trim();

    LOGF("[Command] Nhận payload: %s\n", payload.c_str());

    JsonDocument doc;
    DeserializationError err = deserializeJson(doc, payload);
    if (err) {
        publishCommandError("JSON không hợp lệ. Ví dụ: {\"action\":\"measure\",\"sensors\":[\"tds\"]}");
        return;
    }

    const char *actionRaw = doc["action"] | "";
    String action = actionRaw;
    action.toLowerCase();
    action.trim();

    if (action == "measure" || action == "do") {
        std::vector<String> sensors;
        JsonVariantConst sensorsVar = doc["sensors"];

        if (sensorsVar.is<JsonArrayConst>()) {
            for (JsonVariantConst item : sensorsVar.as<JsonArrayConst>()) {
                if (item.is<const char*>()) {
                    sensors.push_back(String(item.as<const char*>()));
                }
            }
        } else if (sensorsVar.is<const char*>()) {
            sensors.push_back(String(sensorsVar.as<const char*>()));
        }

        if (sensors.empty()) {
            sensors.push_back("all");
        }

        samplingManager.enqueueMeasure(sensors);
        return;
    }

    if (action == "pump") {
        String target = doc["target"] | "";
        target.toLowerCase();
        target.trim();
        if (target != "inlet" && target != "drain") {
            publishCommandError("pump target phải là \"inlet\" hoặc \"drain\".");
            return;
        }

        bool state = false;
        if (!parsePumpState(doc["state"], state)) {
            publishCommandError("Thiếu/sai state cho pump. Dùng \"ON\"/\"OFF\" hoặc true/false.");
            return;
        }

        samplingManager.enqueuePump(target, state);
        return;
    }

    if (action == "status") {
        samplingManager.enqueueStatus();
        return;
    }

    if (action == "clear_queue" || action == "clear" || action == "abort") {
        samplingManager.clearQueue();
        return;
    }

    // schedule / auto_toggle từ backend: nhận nhưng chưa điều khiển phần cứng
    if (action == "schedule" || action == "auto_toggle") {
        LOGF("[Command] Nhận action '%s' (chưa triển khai phần cứng).\n", action.c_str());
        return;
    }

    publishCommandError("action không hỗ trợ. Dùng measure | pump | status | clear_queue.");
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
