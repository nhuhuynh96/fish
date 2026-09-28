#ifndef REMOTE_LOG_H
#define REMOTE_LOG_H

#include <Arduino.h>
#include <stdarg.h>

// Ghi log ra Serial + xếp hàng publish MQTT <topic>/log
class RemoteLog {
public:
    void printf(const char *fmt, ...) __attribute__((format(printf, 2, 3)));
    void println(const String &msg);
    void println(const char *msg);

    // Gọi trong loop() để đẩy hàng đợi lên MQTT
    void handle();

private:
    static const int QUEUE_SIZE = 24;
    static const int LOG_LINE_MAX = 220;
    static const int PUBLISH_PER_TICK = 4;

    char queue[QUEUE_SIZE][LOG_LINE_MAX];
    uint8_t head = 0;
    uint8_t tail = 0;
    uint8_t count = 0;
    String lineBuf;

    void enqueueLine(const char *line);
    bool publishOne(const char *line);
};

extern RemoteLog remoteLog;

#define LOGF(...) remoteLog.printf(__VA_ARGS__)
#define LOGLN(msg) remoteLog.println(msg)

#endif // REMOTE_LOG_H
