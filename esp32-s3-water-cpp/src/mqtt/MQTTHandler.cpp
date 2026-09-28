#include "MQTTHandler.h"
#include <ArduinoJson.h>

MQTTHandler mqttHandler;

MQTTHandler::MQTTHandler() : mqttClient(espClient) {}

String MQTTHandler::topicFor(const String &subTopic) const {
    return config.topic + "/" + subTopic;
}

void MQTTHandler::begin(const DeviceConfig &cfg) {
    config = cfg;
    if (config.mqtt_host.isEmpty()) {
        Serial.println("[MQTT] MQTT Host trống, bỏ qua MQTT.");
        return;
    }

    // Client ID riêng cho board nước để không đá board ESP32 khác đang dùng cùng device_id
    uint64_t chipid = ESP.getEfuseMac();
    char suffix[8];
    snprintf(suffix, sizeof(suffix), "%04x", (uint16_t)(chipid >> 32));
    clientId = "water-" + config.device_id + "-" + suffix;

    mqttClient.setBufferSize(1024);
    mqttClient.setKeepAlive(30);
    mqttClient.setServer(config.mqtt_host.c_str(), config.mqtt_port);
    mqttClient.setCallback([this](char *topic, byte *payload, unsigned int length) {
        String msg;
        msg.reserve(length);
        for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
        Serial.printf("[MQTT] Nhận [%s]: %s\n", topic, msg.c_str());
        if (commandCallback) commandCallback(msg);
    });

    Serial.printf("[MQTT] Broker %s:%u | client=%s | topic=%s/...\n",
                  config.mqtt_host.c_str(), config.mqtt_port, clientId.c_str(), config.topic.c_str());
}

bool MQTTHandler::isConnected() {
    return mqttClient.connected();
}

bool MQTTHandler::reconnect() {
    if (WiFi.status() != WL_CONNECTED || config.mqtt_host.isEmpty()) return false;

    Serial.printf("[MQTT] Đang kết nối %s:%u ... ", config.mqtt_host.c_str(), config.mqtt_port);

    String willTopic = topicFor("status");
    JsonDocument will;
    will["device_id"] = config.device_id;
    will["status"] = "offline";
    String willMessage;
    serializeJson(will, willMessage);

    bool ok;
    if (config.mqtt_user.length() > 0) {
        ok = mqttClient.connect(clientId.c_str(), config.mqtt_user.c_str(), config.mqtt_pass.c_str(),
                                willTopic.c_str(), 1, true, willMessage.c_str());
    } else {
        ok = mqttClient.connect(clientId.c_str(), willTopic.c_str(), 1, true, willMessage.c_str());
    }

    if (!ok) {
        Serial.printf("thất bại, rc=%d\n", mqttClient.state());
        return false;
    }

    Serial.println("thành công!");
    String cmdTopic = topicFor("command");
    mqttClient.subscribe(cmdTopic.c_str(), 1);
    Serial.printf("[MQTT] Subscribe: %s\n", cmdTopic.c_str());
    publishStatus("online");
    return true;
}

void MQTTHandler::handle() {
    if (config.mqtt_host.isEmpty() || WiFi.status() != WL_CONNECTED) return;

    if (mqttClient.connected()) {
        mqttClient.loop();
        return;
    }
    unsigned long now = millis();
    if (lastReconnectAttempt == 0 || now - lastReconnectAttempt > 5000) {
        lastReconnectAttempt = now;
        if (reconnect()) lastReconnectAttempt = 0;
    }
}

bool MQTTHandler::publish(const String &subTopic, const String &payload, bool retained) {
    if (!mqttClient.connected()) return false;
    return mqttClient.publish(topicFor(subTopic).c_str(), payload.c_str(), retained);
}

bool MQTTHandler::publishStatus(const String &status) {
    JsonDocument doc;
    doc["device_id"] = config.device_id;
    doc["status"] = status;
    doc["ip"] = WiFi.localIP().toString();
    doc["role"] = "water";
    doc["topic"] = config.topic;
    String payload;
    serializeJson(doc, payload);
    return publish("status", payload, true);
}
