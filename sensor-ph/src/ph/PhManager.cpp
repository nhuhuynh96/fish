#include "PhManager.h"

#include <Preferences.h>

#include "../config/ConfigManager.h"
#include "../log/RemoteLog.h"
#include "../mqtt/MQTTHandler.h"
#include "../tds/TdsManager.h"

PhManager phManager;

static HardwareSerial &bus = Serial1;
static Preferences modbusPrefs;

enum {
    MB_OK = 0,
    MB_TIMEOUT = 1,
    MB_SHORT = 2,
    MB_CRC = 3,
    MB_FRAME = 4,
    MB_EXCEPTION = 0x80,
};

static uint16_t crc16(const uint8_t *buf, size_t len) {
    uint16_t crc = 0xFFFF;
    for (size_t i = 0; i < len; i++) {
        crc ^= buf[i];
        for (int b = 0; b < 8; b++) {
            crc = (crc & 1) ? (crc >> 1) ^ 0xA001 : crc >> 1;
        }
    }
    return crc;
}

void PhManager::begin() {
    loadConfig();
    if (cfg.dePin >= 0) {
        pinMode(cfg.dePin, OUTPUT);
        digitalWrite(cfg.dePin, LOW);
    }
    openBus(cfg.baud);
    LOGF("[PH] RS485 RX=GPIO%d TX=GPIO%d DE=%d | addr=%u baud=%lu fc=%u ph_reg=%u x%.4f temp_reg=%ld\n",
         RX_PIN, TX_PIN, cfg.dePin, cfg.addr, (unsigned long)cfg.baud, cfg.fc, cfg.phReg, cfg.phScale,
         (long)cfg.tempReg);
}

void PhManager::loadConfig() {
    modbusPrefs.begin("ph_modbus", true);
    cfg.addr = modbusPrefs.getUChar("addr", cfg.addr);
    cfg.baud = modbusPrefs.getULong("baud", cfg.baud);
    cfg.fc = modbusPrefs.getUChar("fc", cfg.fc);
    cfg.phReg = modbusPrefs.getUShort("ph_reg", cfg.phReg);
    cfg.phScale = modbusPrefs.getFloat("ph_scale", cfg.phScale);
    cfg.tempReg = modbusPrefs.getInt("temp_reg", cfg.tempReg);
    cfg.tempScale = modbusPrefs.getFloat("temp_scale", cfg.tempScale);
    cfg.dePin = modbusPrefs.getChar("de_pin", cfg.dePin);
    modbusPrefs.end();
}

void PhManager::saveConfig() {
    modbusPrefs.begin("ph_modbus", false);
    modbusPrefs.putUChar("addr", cfg.addr);
    modbusPrefs.putULong("baud", cfg.baud);
    modbusPrefs.putUChar("fc", cfg.fc);
    modbusPrefs.putUShort("ph_reg", cfg.phReg);
    modbusPrefs.putFloat("ph_scale", cfg.phScale);
    modbusPrefs.putInt("temp_reg", cfg.tempReg);
    modbusPrefs.putFloat("temp_scale", cfg.tempScale);
    modbusPrefs.putChar("de_pin", cfg.dePin);
    modbusPrefs.end();
}

void PhManager::openBus(uint32_t baud) {
    bus.end();
    bus.begin(baud, SERIAL_8N1, RX_PIN, TX_PIN);
}

void PhManager::setDE(bool transmit) {
    if (cfg.dePin >= 0) digitalWrite(cfg.dePin, transmit ? HIGH : LOW);
}

const char *PhManager::errorText(int code) {
    switch (code) {
    case MB_OK: return "ok";
    case MB_TIMEOUT: return "không trả lời (kiểm tra nguồn, A/B, địa chỉ, baud)";
    case MB_SHORT: return "trả lời thiếu byte";
    case MB_CRC: return "sai CRC (nhiễu hoặc sai baud)";
    case MB_FRAME: return "khung trả lời sai địa chỉ hoặc chức năng";
    default: return code >= MB_EXCEPTION ? "cảm biến báo lỗi Modbus (sai thanh ghi hoặc sai fc)" : "lỗi không rõ";
    }
}

int PhManager::readRegisters(uint8_t addr, uint8_t fc, uint16_t start, uint16_t count, uint16_t *out, uint32_t timeoutMs) {
    if (count == 0 || count > MAX_REGS) return MB_FRAME;
    uint8_t req[8] = {addr, fc, (uint8_t)(start >> 8), (uint8_t)(start & 0xFF), (uint8_t)(count >> 8), (uint8_t)(count & 0xFF), 0, 0};
    uint16_t crc = crc16(req, 6);
    req[6] = crc & 0xFF;
    req[7] = crc >> 8;

    while (bus.available()) bus.read();
    setDE(true);
    bus.write(req, sizeof(req));
    bus.flush();
    setDE(false);

    uint8_t resp[5 + 2 * MAX_REGS];
    size_t expected = 5 + 2 * count;
    size_t n = 0;
    uint32_t started = millis();
    while (millis() - started < timeoutMs && n < expected) {
        while (bus.available() && n < sizeof(resp)) {
            resp[n++] = bus.read();
            if (n == 2 && (resp[1] & 0x80)) expected = 5;
        }
        delay(1);
    }
    if (n == 0) return MB_TIMEOUT;
    if (n < expected) return MB_SHORT;
    uint16_t got = resp[expected - 2] | (resp[expected - 1] << 8);
    if (crc16(resp, expected - 2) != got) return MB_CRC;
    if (resp[0] != addr) return MB_FRAME;
    if (resp[1] == (fc | 0x80)) return MB_EXCEPTION | resp[2];
    if (resp[1] != fc || resp[2] != 2 * count) return MB_FRAME;
    for (uint16_t i = 0; i < count; i++) {
        out[i] = (resp[3 + 2 * i] << 8) | resp[4 + 2 * i];
    }
    return MB_OK;
}

void PhManager::publishEvent(const char *stage, const String &message, JsonDocument *extra) {
    LOGF("[PH] %s: %s\n", stage, message.c_str());
    if (!mqttHandler.isConnected()) return;
    JsonDocument doc;
    if (extra) doc.set(*extra);
    doc["device_id"] = currentConfig.device_id;
    doc["stage"] = stage;
    doc["message"] = message;
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    String payload;
    serializeJson(doc, payload);
    mqttHandler.publish("event", payload);
}

void PhManager::waitMs(uint32_t ms) {
    uint32_t start = millis();
    while (millis() - start < ms) {
        mqttHandler.handle();
        remoteLog.handle();
        delay(10);
    }
}

void PhManager::requestRead(int samples, int intervalMs) {
    if (busy || pending != JOB_NONE) {
        publishEvent("sensor_error", "Đang đọc, chưa nhận thêm lệnh.");
        return;
    }
    pending = JOB_READ;
    pendingA = samples;
    pendingB = intervalMs;
}

void PhManager::requestRegs(int start, int count, int fc) {
    if (busy || pending != JOB_NONE) {
        publishEvent("command_error", "Đang đọc, chưa nhận thêm lệnh.");
        return;
    }
    pending = JOB_REGS;
    pendingA = start;
    pendingB = count;
    pendingC = fc;
}

void PhManager::requestScan() {
    if (busy || pending != JOB_NONE) {
        publishEvent("command_error", "Đang đọc, chưa nhận thêm lệnh.");
        return;
    }
    pending = JOB_SCAN;
}

void PhManager::handle() {
    if (busy || pending == JOB_NONE) return;
    Job job = pending;
    pending = JOB_NONE;
    busy = true;
    if (job == JOB_READ) runRead(pendingA, pendingB);
    if (job == JOB_REGS) runRegs(pendingA, pendingB, pendingC);
    if (job == JOB_SCAN) runScan();
    busy = false;
}

static int clampInt(int v, int fallback, int lo, int hi) {
    if (v <= 0) v = fallback;
    return v < lo ? lo : (v > hi ? hi : v);
}

static float medianOf(float *values, int n) {
    for (int i = 1; i < n; i++) {
        float key = values[i];
        int j = i - 1;
        while (j >= 0 && values[j] > key) {
            values[j + 1] = values[j];
            j--;
        }
        values[j + 1] = key;
    }
    return (n % 2) ? values[n / 2] : (values[n / 2 - 1] + values[n / 2]) / 2.0f;
}

void PhManager::runRead(int samples, int intervalMs) {
    // Que TDS có điện thì dòng xoay chiều trong nước làm lệch pH.
    if (tdsManager.isPowered()) {
        publishEvent("sensor_error", "TDS đang bật nguồn, tắt TDS trước khi đọc pH.");
        return;
    }
    samples = clampInt(samples, 10, 1, MAX_SAMPLES);
    intervalMs = clampInt(intervalMs, 500, 100, 5000);
    uint32_t started = millis();

    float phValues[MAX_SAMPLES];
    float tempValues[MAX_SAMPLES];
    uint16_t phRegs[MAX_SAMPLES];
    int ok = 0;
    int tempOk = 0;
    int lastErr = MB_OK;
    for (int i = 0; i < samples; i++) {
        uint16_t reg = 0;
        int err = readRegisters(cfg.addr, cfg.fc, cfg.phReg, 1, &reg, RESPONSE_TIMEOUT_MS);
        if (err == MB_OK) {
            phRegs[ok] = reg;
            phValues[ok++] = reg * cfg.phScale;
        } else {
            lastErr = err;
        }
        if (cfg.tempReg >= 0) {
            uint16_t treg = 0;
            if (readRegisters(cfg.addr, cfg.fc, (uint16_t)cfg.tempReg, 1, &treg, RESPONSE_TIMEOUT_MS) == MB_OK) {
                tempValues[tempOk++] = (int16_t)treg * cfg.tempScale;
            }
        }
        if (i + 1 < samples) waitMs(intervalMs);
    }

    lastOk = ok > 0;
    if (ok == 0) {
        publishEvent("sensor_error", String("Modbus pH: ") + errorText(lastErr) + " (mã " + String(lastErr) + ").");
        return;
    }

    float ph = medianOf(phValues, ok);
    lastPhValue = ph;
    if (ph < 0 || ph > 14) {
        publishEvent("sensor_error", "pH đọc ra " + String(ph, 2) + ", ngoài 0-14. Kiểm tra ph_reg và ph_scale.");
        return;
    }

    JsonDocument doc;
    doc["device_id"] = currentConfig.device_id;
    doc["status"] = "success";
    doc["sensors_measured"].add("ph");
    doc["data"]["ph"] = ph;
    doc["raw"]["ph_reg"] = phRegs[0];
    doc["raw"]["ok"] = ok;
    doc["raw"]["fail"] = samples - ok;
    if (tempOk > 0) doc["raw"]["temp_c"] = medianOf(tempValues, tempOk);
    doc["timestamp"] = (unsigned long)(millis() / 1000);
    doc["duration_ms"] = (unsigned long)(millis() - started);
    String payload;
    serializeJson(doc, payload);
    LOGF("[PH] pH=%.2f (%d/%d lần đọc được)\n", ph, ok, samples);
    mqttHandler.publish("sensor_data", payload);
}

bool PhManager::readTempC(float &celsius) {
    if (cfg.tempReg < 0) return false;
    uint16_t reg = 0;
    if (readRegisters(cfg.addr, cfg.fc, (uint16_t)cfg.tempReg, 1, &reg, RESPONSE_TIMEOUT_MS) != MB_OK) return false;
    celsius = (int16_t)reg * cfg.tempScale;
    return true;
}

void PhManager::runRegs(int start, int count, int fc) {
    if (start < 0) start = 0;
    count = clampInt(count, 10, 1, MAX_REGS);
    uint8_t function = (fc == 4) ? 4 : (fc == 3 ? 3 : cfg.fc);
    uint16_t regs[MAX_REGS];
    int err = readRegisters(cfg.addr, function, (uint16_t)start, (uint16_t)count, regs, RESPONSE_TIMEOUT_MS);
    if (err != MB_OK) {
        publishEvent("sensor_error", String("Đọc thanh ghi: ") + errorText(err) + " (mã " + String(err) + ").");
        return;
    }
    JsonDocument extra;
    extra["addr"] = cfg.addr;
    extra["fc"] = function;
    extra["start"] = start;
    JsonArray values = extra["values"].to<JsonArray>();
    for (int i = 0; i < count; i++) values.add(regs[i]);
    publishEvent("regs", "Đọc " + String(count) + " thanh ghi từ " + String(start) + ".", &extra);
}

// Thử địa chỉ 1-10 với ba tốc độ baud; trả lời lỗi Modbus vẫn tính là tìm thấy thiết bị.
void PhManager::runScan() {
    static const uint32_t bauds[] = {4800, 9600, 19200};
    JsonDocument extra;
    JsonArray found = extra["found"].to<JsonArray>();
    publishEvent("scan_started", "Đang dò địa chỉ 1-10, baud 4800/9600/19200...");
    for (uint32_t baud : bauds) {
        openBus(baud);
        for (uint8_t addr = 1; addr <= 10; addr++) {
            uint16_t reg = 0;
            int err = readRegisters(addr, cfg.fc, 0, 1, &reg, SCAN_TIMEOUT_MS);
            if (err == MB_OK || err >= MB_EXCEPTION) {
                JsonObject f = found.add<JsonObject>();
                f["addr"] = addr;
                f["baud"] = baud;
                if (err == MB_OK) f["reg0"] = reg;
                else f["exception"] = err & 0x7F;
            }
            waitMs(20);
        }
    }
    openBus(cfg.baud);
    publishEvent("scan_result", "Tìm thấy " + String((unsigned)found.size()) + " thiết bị.", &extra);
}

void PhManager::applyConfig(JsonDocument &doc) {
    if (busy) {
        publishEvent("command_error", "Đang đọc, chưa đổi cấu hình.");
        return;
    }
    if (!doc["addr"].isNull()) cfg.addr = constrain(doc["addr"].as<int>(), 1, 247);
    if (!doc["baud"].isNull()) cfg.baud = doc["baud"].as<uint32_t>();
    if (!doc["fc"].isNull()) cfg.fc = doc["fc"].as<int>() == 4 ? 4 : 3;
    if (!doc["ph_reg"].isNull()) cfg.phReg = doc["ph_reg"].as<uint16_t>();
    if (!doc["ph_scale"].isNull()) cfg.phScale = doc["ph_scale"].as<float>();
    if (!doc["temp_reg"].isNull()) cfg.tempReg = doc["temp_reg"].as<int>();
    if (!doc["temp_scale"].isNull()) cfg.tempScale = doc["temp_scale"].as<float>();
    if (!doc["de_pin"].isNull()) {
        int pin = doc["de_pin"].as<int>();
        // Chân khởi động (2, 8, 9) và chân UART không dùng làm DE.
        if (pin == 2 || pin == 8 || pin == 9 || pin == RX_PIN || pin == TX_PIN) pin = -1;
        cfg.dePin = pin;
        if (cfg.dePin >= 0) {
            pinMode(cfg.dePin, OUTPUT);
            digitalWrite(cfg.dePin, LOW);
        }
    }
    if (cfg.baud != 2400 && cfg.baud != 4800 && cfg.baud != 9600 && cfg.baud != 19200 && cfg.baud != 38400) {
        cfg.baud = 9600;
    }
    saveConfig();
    openBus(cfg.baud);
    publishConfig();
}

void PhManager::publishConfig() {
    JsonDocument extra;
    extra["addr"] = cfg.addr;
    extra["baud"] = cfg.baud;
    extra["fc"] = cfg.fc;
    extra["ph_reg"] = cfg.phReg;
    extra["ph_scale"] = cfg.phScale;
    extra["temp_reg"] = cfg.tempReg;
    extra["temp_scale"] = cfg.tempScale;
    extra["de_pin"] = cfg.dePin;
    publishEvent("modbus_config", "Cấu hình Modbus hiện tại.", &extra);
}
