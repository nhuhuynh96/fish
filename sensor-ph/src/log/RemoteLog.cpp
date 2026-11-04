#include "RemoteLog.h"
#include "../mqtt/MQTTHandler.h"
#include "../config/ConfigManager.h"

RemoteLog remoteLog;

RemoteLog::RemoteLog() {}

void RemoteLog::printf(const char *fmt, ...) {
    char buf[LOG_LINE_MAX];
    va_list args;
    va_start(args, fmt);
    vsnprintf(buf, sizeof(buf), fmt, args);
    va_end(args);

    // Serial luôn in (kể cả khi chưa có \n)
    Serial.print(buf);

    // Gom theo dòng rồi mới enqueue (giống Serial Monitor)
    for (const char *p = buf; *p; ++p) {
        if (*p == '\n') {
            flushLineBuffer();
        } else if (*p != '\r') {
            if (lineBuf.length() < (unsigned)(LOG_LINE_MAX - 1)) {
                lineBuf += *p;
            }
        }
    }
}

void RemoteLog::println(const String &msg) {
    Serial.println(msg);
    enqueueLine(msg.c_str());
}

void RemoteLog::println(const char *msg) {
    Serial.println(msg);
    enqueueLine(msg ? msg : "");
}

void RemoteLog::print(const String &msg) {
    printf("%s", msg.c_str());
}

void RemoteLog::print(const char *msg) {
    printf("%s", msg ? msg : "");
}

void RemoteLog::flushLineBuffer() {
    if (lineBuf.length() > 0) {
        enqueueLine(lineBuf.c_str());
        lineBuf = "";
    } else {
        // dòng trống vẫn bỏ qua
    }
}

void RemoteLog::enqueueLine(const char *line) {
    if (!line || line[0] == '\0') return;

    // Bỏ qua log nội bộ MQTT / RemoteLog để tránh vòng lặp
    if (strncmp(line, "[MQTT]", 6) == 0) return;
    if (strncmp(line, "[RemoteLog]", 11) == 0) return;

    if (count >= QUEUE_SIZE) {
        // Drop oldest
        head = (head + 1) % QUEUE_SIZE;
        count--;
    }

    strncpy(queue[tail], line, LOG_LINE_MAX - 1);
    queue[tail][LOG_LINE_MAX - 1] = '\0';
    tail = (tail + 1) % QUEUE_SIZE;
    count++;
}

String RemoteLog::escapeJson(const char *raw) const {
    String out;
    out.reserve(strlen(raw) + 8);
    for (const char *p = raw; *p; ++p) {
        char c = *p;
        if (c == '"' || c == '\\') {
            out += '\\';
            out += c;
        } else if (c == '\n') {
            out += "\\n";
        } else if (c == '\r') {
            // skip
        } else if ((uint8_t)c < 0x20) {
            // skip control chars
        } else {
            out += c;
        }
    }
    return out;
}

bool RemoteLog::publishOne(const char *line) {
    if (!mqttHandler.isConnected()) return false;
    if (publishing) return false;

    publishing = true;
    String payload = "{";
    payload += "\"device_id\":\"" + currentConfig.device_id + "\",";
    payload += "\"uptime_ms\":" + String(millis()) + ",";
    payload += "\"msg\":\"" + escapeJson(line) + "\"";
    payload += "}";

    bool ok = mqttHandler.publish("log", payload, false);
    publishing = false;
    return ok;
}

void RemoteLog::handle() {
    // Nếu còn sót buffer chưa có \n, sau một lúc vẫn đẩy (đoạn Serial.print không kết thúc dòng)
    // — không flush tự động để tránh cắt giữa chừng; chỉ flush khi có \n hoặc println.

    if (!mqttHandler.isConnected() || count == 0) return;

    int published = 0;
    while (count > 0 && published < PUBLISH_PER_TICK) {
        const char *line = queue[head];
        if (!publishOne(line)) {
            // MQTT bận / disconnect — giữ lại queue, thử sau
            break;
        }
        head = (head + 1) % QUEUE_SIZE;
        count--;
        published++;
    }
}
