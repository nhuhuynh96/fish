#ifndef MQTT_HANDLER_H
#define MQTT_HANDLER_H

#include <Arduino.h>
#include <WiFi.h>
#include <PubSubClient.h>
#include "../config/ConfigManager.h"

typedef std::function<void(char* topic, byte* payload, unsigned int length)> MessageCallback;

class MQTTHandler {
public:
    MQTTHandler();
    void begin(const DeviceConfig &config);
    void handle();
    bool isConnected();
    bool publish(const String &subTopic, const String &payload, bool retained = false);
    bool publishStatus(const String &statusMessage = "online");
    void setCallback(MessageCallback cb);

private:
    WiFiClient espClient;
    PubSubClient mqttClient;
    DeviceConfig currentConfig;
    unsigned long lastReconnectAttempt = 0;
    MessageCallback userCallback = nullptr;

    bool reconnect();
    void onMessageReceived(char* topic, byte* payload, unsigned int length);
};

extern MQTTHandler mqttHandler;

#endif // MQTT_HANDLER_H
