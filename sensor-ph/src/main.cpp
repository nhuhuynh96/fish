#include <Arduino.h>
#include <ArduinoJson.h>
#include <WiFi.h>

#include "config/ConfigManager.h"
#include "log/RemoteLog.h"
#include "mqtt/MQTTHandler.h"
#include "ph/PhManager.h"
#include "portal/WebPortal.h"

const int BUTTON_PIN = 9;  // nút BOOT trên ESP32-C3 Super Mini
const int LED_PIN = 8;     // LED xanh, mức LOW là sáng
const unsigned long BUTTON_HOLD_TIME = 3000;
const unsigned long WIFI_RETRY_MS = 30000;
const unsigned long MQTT_RETRY_MIN_MS = 5000;
const unsigned long MQTT_RETRY_MAX_MS = 60000;

unsigned long buttonPressStartTime = 0;
bool buttonIsPressed = false;
unsigned long lastHeartbeat = 0;

static void publishCommandError(const String &message) {
    LOGLN("[Command] " + message);
    if (!mqttHandler.isConnected()) return;
    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = "command_error";
    doc["message"] = message;
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

// {"action":"ph","samples":10,"interval_ms":500}
// {"action":"modbus_config","addr":1,"baud":9600,"fc":3,"ph_reg":0,"ph_scale":0.01,"temp_reg":-1,"temp_scale":0.1,"de_pin":-1}
// {"action":"get_config"} | {"action":"read_regs","start":0,"count":10,"fc":3} | {"action":"scan"}
void handleMqttCommand(const String &msg) {
    JsonDocument doc;
    if (deserializeJson(doc, msg)) {
        publishCommandError("JSON không hợp lệ. Ví dụ: {\"action\":\"ph\"}");
        return;
    }

    String action = doc["action"] | "";
    action.toLowerCase();
    action.trim();
    if (action.length() == 0) {
        publishCommandError("Thiếu action.");
        return;
    }

    LOGF("[Command] action=%s\n", action.c_str());
    if (action == "ph" || action == "measure" || action == "measure_all" || action == "all") {
        phManager.requestRead(doc["samples"] | 0, doc["interval_ms"] | 0);
        return;
    }
    if (action == "modbus_config") {
        phManager.applyConfig(doc);
        return;
    }
    if (action == "get_config") {
        phManager.publishConfig();
        return;
    }
    if (action == "read_regs") {
        phManager.requestRegs(doc["start"] | 0, doc["count"] | 10, doc["fc"] | 0);
        return;
    }
    if (action == "scan") {
        phManager.requestScan();
        return;
    }
    publishCommandError("Action không hỗ trợ: " + action);
}

void setup() {
    Serial.begin(115200);
    pinMode(LED_PIN, OUTPUT);
    digitalWrite(LED_PIN, HIGH);
    delay(1000);

    LOGLN("\n==========================================");
    LOGLN("       ESP32-C3 pH RS485 NODE             ");
    LOGLN("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);
    phManager.begin();

    bool hasConfig = configManager.loadConfig(currentConfig);
    if (!hasConfig || digitalRead(BUTTON_PIN) == LOW) {
        if (!hasConfig) {
            LOGLN("[Main] Chưa có cấu hình WiFi/MQTT. Bắt đầu phát AP...");
        } else {
            LOGLN("[Main] Giữ nút BOOT. Bắt đầu phát AP cấu hình...");
        }
        webPortal.startPortal();
        return;
    }

    LOGF("[Main] Đang kết nối WiFi: %s\n", currentConfig.wifi_ssid.c_str());
    WiFi.mode(WIFI_STA);
    WiFi.setTxPower(WIFI_POWER_8_5dBm);  // anten bo C3 Super Mini kém, công suất cao làm rớt WiFi
    WiFi.persistent(false);
    WiFi.setAutoReconnect(true);
    WiFi.begin(currentConfig.wifi_ssid.c_str(), currentConfig.wifi_pass.c_str());

    unsigned long startAttempt = millis();
    while (WiFi.status() != WL_CONNECTED && millis() - startAttempt < 15000) {
        delay(500);
        Serial.print(".");
    }
    Serial.println();

    mqttHandler.begin(currentConfig);
    mqttHandler.setCallback([](char *topic, byte *payload, unsigned int length) {
        (void)topic;
        String msg;
        for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
        handleMqttCommand(msg);
    });

    if (WiFi.status() == WL_CONNECTED) {
        LOGLN("[Main] WiFi đã kết nối.");
        LOGF("[Main] IP: %s\n", WiFi.localIP().toString().c_str());
        mqttHandler.reconnect();
    } else {
        LOGLN("[Main] WiFi chưa lên. Giữ STA, thử lại mỗi 30 giây.");
    }
}

void checkButtonHold() {
    if (digitalRead(BUTTON_PIN) == LOW) {
        if (!buttonIsPressed) {
            buttonIsPressed = true;
            buttonPressStartTime = millis();
        } else if (millis() - buttonPressStartTime >= BUTTON_HOLD_TIME) {
            LOGLN("[Button] Giữ BOOT 3 giây. Vào AP cấu hình.");
            buttonIsPressed = false;
            webPortal.startPortal();
        }
    } else if (buttonIsPressed) {
        buttonIsPressed = false;
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
            WiFi.setTxPower(WIFI_POWER_8_5dBm);
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
    if (webPortal.isRunning()) {
        webPortal.handlePortal();
        return;
    }

    checkButtonHold();
    checkLinks();
    mqttHandler.handle();
    remoteLog.handle();
    phManager.handle();
    digitalWrite(LED_PIN, mqttHandler.isConnected() ? LOW : HIGH);

    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            JsonDocument doc;
            doc["device_id"] = currentConfig.device_id;
            doc["rssi"] = WiFi.RSSI();
            doc["uptime"] = millis() / 1000;
            doc["state"] = phManager.isBusy() ? "MEASURING" : "IDLE";
            doc["modbus_ok"] = phManager.lastReadOk();
            if (!isnan(phManager.lastPh())) doc["last_ph"] = phManager.lastPh();
            String telemetry;
            serializeJson(doc, telemetry);
            mqttHandler.publish("telemetry", telemetry);
        }
    }
}
