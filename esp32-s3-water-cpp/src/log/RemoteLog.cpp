#include "RemoteLog.h"
#include <ArduinoJson.h>
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"

RemoteLog remoteLog;

void RemoteLog::printf(const char *fmt, ...) {
    char buf[LOG_LINE_MAX];
    va_list args;
    va_start(args, fmt);
    vsnprintf(buf, sizeof(buf), fmt, args);
    va_end(args);

    Serial.print(buf);

    for (const char *p = buf; *p; ++p) {
        if (*p == '\n') {
            if (lineBuf.length() > 0) {
                enqueueLine(lineBuf.c_str());
                lineBuf = "";
            }
        } else if (*p != '\r' && lineBuf.length() < (unsigned)(LOG_LINE_MAX - 1)) {
            lineBuf += *p;
        }
    }
}

void RemoteLog::println(const String &msg) {
    Serial.println(msg);
    enqueueLine(msg.c_str());
}

void RemoteLog::println(const char *msg) {
    Serial.println(msg ? msg : "");
    enqueueLine(msg ? msg : "");
}

void RemoteLog::enqueueLine(const char *line) {
    if (!line || line[0] == '\0') return;
    if (strncmp(line, "[MQTT]", 6) == 0) return;

    if (count >= QUEUE_SIZE) {
        head = (head + 1) % QUEUE_SIZE;
        count--;
    }
    strncpy(queue[tail], line, LOG_LINE_MAX - 1);
    queue[tail][LOG_LINE_MAX - 1] = '\0';
    tail = (tail + 1) % QUEUE_SIZE;
    count++;
}

bool RemoteLog::publishOne(const char *line) {
    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["uptime_ms"] = millis();
    doc["msg"] = line;
    String payload;
    serializeJson(doc, payload);
    return mqttHandler.publish("log", payload, false);
}

void RemoteLog::handle() {
    if (!mqttHandler.isConnected()) return;
    int published = 0;
    while (count > 0 && published < PUBLISH_PER_TICK) {
        if (!publishOne(queue[head])) break;
        head = (head + 1) % QUEUE_SIZE;
        count--;
        published++;
    }
}
