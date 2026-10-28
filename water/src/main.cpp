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
const unsigned long WIFI_RETRY_MS = 30000;
const unsigned long MQTT_RETRY_MIN_MS = 5000;
const unsigned long MQTT_RETRY_MAX_MS = 60000;

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

const uint32_t DEFAULT_DURATION_S = 60;
const uint32_t MIN_DURATION_S = 1;
const uint32_t MAX_DURATION_S = 300;

// until_full / until_empty quyết định có dừng theo phao không. Lệnh cũ gửi "level" > 0 cũng hiểu là dừng theo phao.
static bool parseUntilFloat(JsonDocument &doc, const char *key, bool fallback) {
    if (!doc[key].isNull()) return doc[key].as<bool>();
    if (!doc["until_float"].isNull()) return doc["until_float"].as<bool>();
    if (!doc["level"].isNull()) return doc["level"].as<int>() > 0;
    return fallback;
}

static uint32_t parseDurationMs(JsonDocument &doc) {
    long seconds = DEFAULT_DURATION_S;
    if (!doc["duration"].isNull()) {
        seconds = doc["duration"].as<long>();
    }
    if (seconds < (long)MIN_DURATION_S) seconds = MIN_DURATION_S;
    if (seconds > (long)MAX_DURATION_S) seconds = MAX_DURATION_S;
    return (uint32_t)seconds * 1000UL;
}

// {"action":"inlet_on","until_full":true,"duration":180}
// {"action":"drain_on","until_empty":true,"duration":120}
// {"action":"inlet_off"|"drain_off"|"stop"|"float"}
void handleMqttCommand(const String &msg) {
    String payload = msg;
    payload.trim();
    LOGF("[Command] Nhận payload: %s\n", payload.c_str());

    JsonDocument doc;
    if (deserializeJson(doc, payload)) {
        publishCommandError("JSON không hợp lệ. Ví dụ: {\"action\":\"inlet_on\",\"until_full\":true,\"duration\":180}");
        return;
    }

    String action = doc["action"] | "";
    action.toLowerCase();
    action.trim();
    if (action.length() == 0) {
        publishCommandError("Thiếu action.");
        return;
    }

    if (action == "float" || action == "level") {
        samplingManager.publishWaterLevel();
        return;
    }
    if (action == "inlet_on" || action == "inlet") {
        samplingManager.startInlet(parseUntilFloat(doc, "until_full", true), parseDurationMs(doc));
        return;
    }
    if (action == "drain_on") {
        samplingManager.startDrain(parseUntilFloat(doc, "until_empty", false), parseDurationMs(doc));
        return;
    }
    if (action == "inlet_off") {
        samplingManager.inletOff();
        return;
    }
    if (action == "drain_off") {
        samplingManager.drainOff();
        return;
    }
    if (action == "stop" || action == "clear_queue") {
        samplingManager.stopAll("stopped", "Đã tắt cả hai bơm.");
        return;
    }
    publishCommandError("action không hỗ trợ: " + action);
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
    WiFi.persistent(false);
    WiFi.setAutoReconnect(true);
    WiFi.begin(currentConfig.wifi_ssid.c_str(), currentConfig.wifi_pass.c_str());

    unsigned long startAttempt = millis();
    const unsigned long WIFI_TIMEOUT = 15000; // Timeout 15 giây

    while (WiFi.status() != WL_CONNECTED && (millis() - startAttempt) < WIFI_TIMEOUT) {
        delay(500);
        Serial.print(".");
    }
    Serial.println();

    mqttHandler.begin(currentConfig);
    mqttHandler.setCallback([](char* topic, byte* payload, unsigned int length) {
        String msg;
        for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
        LOGF("[App] Lệnh nhận từ MQTT: %s\n", msg.c_str());
        handleMqttCommand(msg);
    });

    if (WiFi.status() == WL_CONNECTED) {
        LOGLN("[Main] WiFi đã kết nối thành công!");
        LOGF("[Main] IP Address: %s\n", WiFi.localIP().toString().c_str());
        mqttHandler.reconnect();
    } else {
        LOGLN("[Main] WiFi chưa lên. Giữ STA, thử lại mỗi 30 giây.");
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

static unsigned long lastWifiTry = 0;
static unsigned long lastMqttTry = 0;
static unsigned long mqttRetryMs = MQTT_RETRY_MIN_MS;

static void checkLinks() {
    unsigned long now = millis();

    if (WiFi.status() != WL_CONNECTED) {
        if (now - lastWifiTry >= WIFI_RETRY_MS) {
            lastWifiTry = now;
            LOGLN("[WiFi] Mất kết nối. Đang nối lại...");
            WiFi.mode(WIFI_STA);
            WiFi.begin(currentConfig.wifi_ssid.c_str(), currentConfig.wifi_pass.c_str());
        }
        return;
    }

    if (mqttHandler.isConnected()) {
        mqttRetryMs = MQTT_RETRY_MIN_MS;
        return;
    }

    if (now - lastMqttTry < mqttRetryMs) return;
    lastMqttTry = now;
    if (mqttHandler.reconnect()) {
        mqttRetryMs = MQTT_RETRY_MIN_MS;
    } else {
        mqttRetryMs = min(mqttRetryMs * 2, MQTT_RETRY_MAX_MS);
        LOGF("[MQTT] Thử lại sau %lus\n", mqttRetryMs / 1000);
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
    checkLinks();

    // Giữ phiên MQTT đang sống. Nối lại nằm trong checkLinks.
    mqttHandler.handle();

    // Đẩy Serial log đã xếp hàng lên MQTT topic fish/<id>/log
    remoteLog.handle();

    samplingManager.handle();

    // Gửi tin nhắn định kỳ (Heartbeat telemetry) mỗi 30 giây
    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            String telemetry = "{\"rssi\":" + String(WiFi.RSSI()) + ",\"uptime\":" + String(millis() / 1000) + ",\"state\":\"" + samplingManager.getStateName() + "\",\"float\":" + String(samplingManager.isFull() ? "true" : "false") + "}";
            mqttHandler.publish("telemetry", telemetry);
            LOGLN("[Main] Đã gửi telemetry lên MQTT.");
        }
    }
}
