#include <Arduino.h>
#include <WiFi.h>
#include <ArduinoJson.h>
#include "config/ConfigManager.h"
#include "portal/WebPortal.h"
#include "mqtt/MQTTHandler.h"
#include "log/RemoteLog.h"
#include "water/WaterManager.h"

static const int BUTTON_PIN = 0; // Nút BOOT
static const unsigned long BUTTON_HOLD_MS = 3000;
static const unsigned long WIFI_TIMEOUT_MS = 20000;
static const unsigned long TELEMETRY_INTERVAL_MS = 30000;

static unsigned long buttonPressedAt = 0;
static bool buttonIsPressed = false;
static unsigned long lastTelemetry = 0;
static bool wasWifiConnected = false;

static uint8_t parseLevel(JsonDocument &doc) {
    JsonVariant v = doc["level"];
    if (v.isNull()) v = doc["fillLevel"];
    if (v.isNull()) return 1;
    if (v.is<const char *>()) {
        String s = v.as<const char *>();
        s.toLowerCase();
        s.trim();
        return (s == "2" || s == "high" || s == "cao") ? 2 : 1;
    }
    return v.as<int>() >= 2 ? 2 : 1;
}

// {"action":"inlet_on","level":1|2} | inlet_off | drain_on | drain_off | clear_queue
static void handleCommand(const String &raw) {
    LOGF("[Command] %s\n", raw.c_str());

    JsonDocument doc;
    if (deserializeJson(doc, raw)) {
        waterManager.emitEvent("command_error", "JSON không hợp lệ. Ví dụ: {\"action\":\"inlet_on\",\"level\":1}");
        return;
    }
    String action = doc["action"] | "";
    action.toLowerCase();
    action.trim();

    if (action == "inlet_on" || action == "inlet") {
        waterManager.enqueueInlet(parseLevel(doc));
    } else if (action == "inlet_off") {
        waterManager.inletOff();
    } else if (action == "drain_on") {
        waterManager.drainOn();
    } else if (action == "drain_off") {
        waterManager.drainOff();
    } else if (action == "clear_queue") {
        waterManager.clearQueue();
    } else {
        waterManager.emitEvent("command_error", "Action không hỗ trợ: " + action);
    }
}

static void enterPortal() {
    waterManager.stopAll();
    webPortal.startPortal();
}

static bool connectWifi() {
    LOGF("[WiFi] Đang kết nối: %s\n", currentConfig.wifi_ssid.c_str());
    WiFi.mode(WIFI_STA);
    WiFi.setAutoReconnect(true);
    WiFi.setHostname(("fish-water-" + currentConfig.device_id).c_str());
    WiFi.begin(currentConfig.wifi_ssid.c_str(), currentConfig.wifi_pass.c_str());

    unsigned long start = millis();
    while (WiFi.status() != WL_CONNECTED && millis() - start < WIFI_TIMEOUT_MS) {
        delay(500);
        Serial.print(".");
    }
    Serial.println();
    return WiFi.status() == WL_CONNECTED;
}

static void checkButtonHold() {
    if (digitalRead(BUTTON_PIN) == LOW) {
        if (!buttonIsPressed) {
            buttonIsPressed = true;
            buttonPressedAt = millis();
        } else if (millis() - buttonPressedAt >= BUTTON_HOLD_MS) {
            buttonIsPressed = false;
            LOGLN("[Button] Giữ BOOT 3 giây -> vào chế độ cấu hình AP");
            enterPortal();
        }
    } else {
        buttonIsPressed = false;
    }
}

static void checkWifi() {
    bool connected = WiFi.status() == WL_CONNECTED;
    if (connected != wasWifiConnected) {
        wasWifiConnected = connected;
        if (connected) {
            LOGF("[WiFi] Đã kết nối lại, IP %s\n", WiFi.localIP().toString().c_str());
        } else {
            Serial.println("[WiFi] Mất kết nối, đang tự kết nối lại...");
        }
    }
}

static void publishTelemetry() {
    if (millis() - lastTelemetry < TELEMETRY_INTERVAL_MS) return;
    lastTelemetry = millis();
    if (!mqttHandler.isConnected()) return;

    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["rssi"] = WiFi.RSSI();
    doc["uptime"] = millis() / 1000;
    doc["state"] = waterManager.stateName();
    doc["queue_size"] = (unsigned)waterManager.queueSize();
    doc["inlet_on"] = waterManager.isInletOn();
    doc["drain_on"] = waterManager.isDrainOn();
    doc["float_low"] = waterManager.float1();
    doc["float_high"] = waterManager.float2();
    doc["free_heap"] = ESP.getFreeHeap();
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("telemetry", payload);
}

void setup() {
    Serial.begin(115200);
    delay(1000);

    LOGLN("\n==========================================");
    LOGLN("      ESP32-S3 FISH WATER CONTROLLER      ");
    LOGLN("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);
    waterManager.begin();

    bool hasConfig = configManager.loadConfig(currentConfig);
    if (!hasConfig || digitalRead(BUTTON_PIN) == LOW) {
        LOGLN(hasConfig ? "[Main] Đang giữ BOOT -> phát AP cấu hình" : "[Main] Chưa có cấu hình -> phát AP cấu hình");
        enterPortal();
        return;
    }

    if (!connectWifi()) {
        LOGLN("[Main] Không kết nối được WiFi -> phát AP cấu hình");
        enterPortal();
        return;
    }
    wasWifiConnected = true;
    LOGF("[WiFi] Đã kết nối, IP %s, RSSI %d dBm\n", WiFi.localIP().toString().c_str(), WiFi.RSSI());

    mqttHandler.begin(currentConfig);
    mqttHandler.setCommandCallback(handleCommand);
    LOGF("[Main] Device ID: %s | Subscribe: %s\n",
         currentConfig.device_id.c_str(), mqttHandler.topicFor("command").c_str());
}

void loop() {
    if (webPortal.isRunning()) {
        webPortal.handlePortal();
        return;
    }

    checkButtonHold();
    checkWifi();
    mqttHandler.handle();
    remoteLog.handle();
    waterManager.handle();
    publishTelemetry();
}
