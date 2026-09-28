#ifndef MQTT_HANDLER_H
#define MQTT_HANDLER_H

#include <Arduino.h>
#include <WiFi.h>
#include <PubSubClient.h>
#include <functional>
#include "../config/ConfigManager.h"

typedef std::function<void(const String &payload)> CommandCallback;

class MQTTHandler {
public:
    MQTTHandler();
    void begin(const DeviceConfig &config);
    void handle();
    bool isConnected();
    // subTopic: "status" | "event" | "telemetry" | "log" -> <topic>/<subTopic>
    bool publish(const String &subTopic, const String &payload, bool retained = false);
    bool publishStatus(const String &status);
    void setCommandCallback(CommandCallback cb) { commandCallback = cb; }
    String topicFor(const String &subTopic) const;

private:
    WiFiClient espClient;
    PubSubClient mqttClient;
    DeviceConfig config;
    String clientId;
    unsigned long lastReconnectAttempt = 0;
    CommandCallback commandCallback = nullptr;

    bool reconnect();
};

extern MQTTHandler mqttHandler;

#endif // MQTT_HANDLER_H
