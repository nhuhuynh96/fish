#include "MQTTHandler.h"

MQTTHandler mqttHandler;

MQTTHandler::MQTTHandler() : mqttClient(espClient) {}

void MQTTHandler::begin(const DeviceConfig &config) {
    currentConfig = config;

    if (currentConfig.mqtt_host.isEmpty()) {
        Serial.println("[MQTT] MQTT Host trống, không khởi tạo MQTT Client.");
        return;
    }

    mqttClient.setServer(currentConfig.mqtt_host.c_str(), currentConfig.mqtt_port);
    mqttClient.setCallback([this](char* topic, byte* payload, unsigned int length) {
        this->onMessageReceived(topic, payload, length);
    });

    Serial.printf("[MQTT] Cấu hình Broker: %s:%u, Device ID: %s\n", 
                  currentConfig.mqtt_host.c_str(), currentConfig.mqtt_port, currentConfig.device_id.c_str());
}

bool MQTTHandler::isConnected() {
    return mqttClient.connected();
}

bool MQTTHandler::reconnect() {
    if (WiFi.status() != WL_CONNECTED || currentConfig.mqtt_host.isEmpty()) {
        return false;
    }

    Serial.print("[MQTT] Đang kết nối tới MQTT Broker... ");

    String willTopic = "fish/" + currentConfig.device_id + "/status";
    String willMessage = "{\"status\":\"offline\",\"device_id\":\"" + currentConfig.device_id + "\"}";

    bool connected = false;
    if (currentConfig.mqtt_user.length() > 0) {
        connected = mqttClient.connect(currentConfig.device_id.c_str(), 
                                      currentConfig.mqtt_user.c_str(), 
                                      currentConfig.mqtt_pass.c_str(),
                                      willTopic.c_str(), 1, true, willMessage.c_str());
    } else {
        connected = mqttClient.connect(currentConfig.device_id.c_str(),
                                      willTopic.c_str(), 1, true, willMessage.c_str());
    }

    if (connected) {
        Serial.println("Thành công!");

        // Đăng ký nhận lệnh theo Device ID
        String cmdTopic = "fish/" + currentConfig.device_id + "/command";
        mqttClient.subscribe(cmdTopic.c_str());
        Serial.printf("[MQTT] Đã Subscribe topic: %s\n", cmdTopic.c_str());

        // Gửi thông báo trạng thái online
        publishStatus("online");
        return true;
    } else {
        Serial.printf("Thất bại, rc=%d\n", mqttClient.state());
        return false;
    }
}

void MQTTHandler::handle() {
    if (currentConfig.mqtt_host.isEmpty() || WiFi.status() != WL_CONNECTED) {
        return;
    }

    if (!mqttClient.connected()) {
        unsigned long now = millis();
        if (now - lastReconnectAttempt > 5000) { // Thử kết nối lại mỗi 5 giây
            lastReconnectAttempt = now;
            if (reconnect()) {
                lastReconnectAttempt = 0;
            }
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
    if (!mqttClient.connected()) return false;
    String topic = "fish/" + currentConfig.device_id + "/status";
    String payload = "{\"status\":\"" + statusMessage + "\",\"ip\":\"" + WiFi.localIP().toString() + "\",\"device_id\":\"" + currentConfig.device_id + "\"}";
    return mqttClient.publish(topic.c_str(), payload.c_str(), true);
}

void MQTTHandler::setCallback(MessageCallback cb) {
    userCallback = cb;
}

void MQTTHandler::onMessageReceived(char* topic, byte* payload, unsigned int length) {
    String message;
    for (unsigned int i = 0; i < length; i++) {
        message += (char)payload[i];
    }
    Serial.printf("[MQTT] Nhận tin từ topic [%s]: %s\n", topic, message.c_str());

    if (userCallback) {
        userCallback(topic, payload, length);
    }
}
