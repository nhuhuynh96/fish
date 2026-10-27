#include <Arduino.h>
#include <ArduinoJson.h>
#include <WiFi.h>

#include "config/ConfigManager.h"
#include "log/RemoteLog.h"
#include "mqtt/MQTTHandler.h"
#include "portal/WebPortal.h"
#include "sensor/SensorManager.h"

const int BUTTON_PIN = 0;
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
    String payload = "{\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"stage\":\"command_error\",";
    payload += "\"message\":\"" + message + "\"}";
    mqttHandler.publish("event", payload);
}

static bool parseState(JsonVariantConst v, bool &on) {
    if (v.is<bool>()) {
        on = v.as<bool>();
        return true;
    }
    if (v.is<int>()) {
        on = v.as<int>() != 0;
        return true;
    }
    if (v.is<const char *>()) {
        String s = v.as<const char *>();
        s.toLowerCase();
        s.trim();
        if (s == "on" || s == "true" || s == "1") {
            on = true;
            return true;
        }
        if (s == "off" || s == "false" || s == "0") {
            on = false;
            return true;
        }
    }
    return false;
}

// {"action":"power","sensor":"ph"|"temp"|"tds"|"all","state":true|false,"max_on_s":300}
// {"action":"power_status"}
// {"action":"ph"|"temp"|"tds"|"measure_all","samples":40,"interval_ms":100}
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
    if (action == "power") {
        String sensor = doc["sensor"] | "";
        sensor.toLowerCase();
        sensor.trim();
        bool on = false;
        if (sensor.length() == 0 || !parseState(doc["state"], on)) {
            publishCommandError("Lệnh power cần sensor và state. Ví dụ: {\"action\":\"power\",\"sensor\":\"ph\",\"state\":true}");
            return;
        }
        int maxOnS = doc["max_on_s"] | 0;
        sensorManager.setPower(sensor, on, maxOnS > 0 ? (uint32_t)maxOnS : 0);
        return;
    }
    if (action == "power_status") {
        sensorManager.publishPowerStatus();
        return;
    }
    sensorManager.request(action, doc["samples"] | 0, doc["interval_ms"] | 0);
}

void setup() {
    Serial.begin(115200);
    neopixelWrite(RGB_BUILTIN, 0, 0, 0);
    delay(1000);

    LOGLN("\n==========================================");
    LOGLN("       ESP32-S3 SENSOR NODE               ");
    LOGLN("==========================================");

    pinMode(BUTTON_PIN, INPUT_PULLUP);
    sensorManager.begin();

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
    sensorManager.handle();

    if (millis() - lastHeartbeat > 30000) {
        lastHeartbeat = millis();
        if (mqttHandler.isConnected()) {
            String telemetry = "{\"device_id\":\"" + currentConfig.device_id + "\",";
            telemetry += "\"rssi\":" + String(WiFi.RSSI()) + ",";
            telemetry += "\"uptime\":" + String(millis() / 1000) + ",";
            telemetry += "\"state\":\"" + String(sensorManager.isBusy() ? "MEASURING" : "IDLE") + "\",";
            telemetry += "\"power\":" + sensorManager.powerJson() + "}";
            mqttHandler.publish("telemetry", telemetry);
        }
    }
}
