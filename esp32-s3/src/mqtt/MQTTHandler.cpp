#include "MQTTHandler.h"

MQTTHandler mqttHandler;

MQTTHandler::MQTTHandler() : mqttClient(espClient) {}

void MQTTHandler::begin(const DeviceConfig &config) {
    currentConfig = config;
    mqttClient.setServer(currentConfig.mqtt_host.c_str(), currentConfig.mqtt_port);
    mqttClient.setBufferSize(1024);
    mqttClient.setCallback([this](char* topic, byte* payload, unsigned int length) {
        this->onMessageReceived(topic, payload, length);
    });
    reconnect();
}

bool MQTTHandler::isConnected() {
    return mqttClient.connected();
}

bool MQTTHandler::reconnect() {
    if (WiFi.status() != WL_CONNECTED) return false;

    String clientId = "fish_s3_" + currentConfig.device_id + "_" + String((uint16_t)(ESP.getEfuseMac() & 0xFFFF), HEX);
    String statusTopic = "fish/" + currentConfig.device_id + "/status";

    Serial.printf("[MQTT S3] Đang kết nối Broker %s:%d...\n", currentConfig.mqtt_host.c_str(), currentConfig.mqtt_port);

    bool success = false;
    if (currentConfig.mqtt_user.length() > 0) {
        success = mqttClient.connect(clientId.c_str(), currentConfig.mqtt_user.c_str(), currentConfig.mqtt_pass.c_str(),
                                     statusTopic.c_str(), 1, true, "offline");
    } else {
        success = mqttClient.connect(clientId.c_str(), statusTopic.c_str(), 1, true, "offline");
    }

    if (success) {
        Serial.println("[MQTT S3] Kết nối Broker thành công!");
        publishStatus("online");

        // Đăng ký nhận lệnh từ Backend / App
        String commandTopic = "fish/" + currentConfig.device_id + "/command";
        mqttClient.subscribe(commandTopic.c_str());
        Serial.printf("[MQTT S3] Đã Subscribe: %s\n", commandTopic.c_str());
    } else {
        Serial.printf("[MQTT S3] Kết nối thất bại, rc=%d\n", mqttClient.state());
    }
    return success;
}

void MQTTHandler::handle() {
    if (WiFi.status() != WL_CONNECTED) return;

    if (!mqttClient.connected()) {
        unsigned long now = millis();
        if (now - lastReconnectAttempt > 5000) {
            lastReconnectAttempt = now;
            reconnect();
        }
    } else {
        mqttClient.loop();
    }
}

bool MQTTHandler::publish(const String &subTopic, const String &payload, bool retained) {
    if (!mqttClient.connected()) return false;
    String fullTopic = "fish/" + currentConfig.device_id + "/" + subTopic;
    return mqttClient.publish(fullTopic.c_str(), payload.c_str(), retained);
}

bool MQTTHandler::publishStatus(const String &statusMessage) {
    return publish("status", statusMessage, true);
}

void MQTTHandler::setCallback(MessageCallback cb) {
    userCallback = cb;
}

void MQTTHandler::onMessageReceived(char* topic, byte* payload, unsigned int length) {
    if (userCallback) {
        userCallback(topic, payload, length);
    }
}
