#ifndef PH_MANAGER_H
#define PH_MANAGER_H

#include <Arduino.h>
#include <ArduinoJson.h>

// Cấu hình Modbus RTU của cảm biến pH, lưu NVS. temp_reg = -1 là không đọc nhiệt độ.
// Mặc định theo cảm biến SN-30030PH-N01-201T.
struct ModbusSettings {
    uint8_t addr = 1;
    uint32_t baud = 4800;
    uint8_t fc = 3;
    uint16_t phReg = 0;
    float phScale = 0.01f;
    int32_t tempReg = 1;
    float tempScale = 0.1f;
    int8_t dePin = -1;
};

// Cảm biến pH RS485 cấp điện liên tục; firmware chỉ đọc khi có lệnh.
class PhManager {
public:
    void begin();
    void handle();

    void requestRead(int samples, int intervalMs);
    void requestRegs(int start, int count, int fc);
    void requestScan();
    void applyConfig(JsonDocument &doc);
    void publishConfig();

    bool isBusy() const { return busy; }
    bool lastReadOk() const { return lastOk; }
    float lastPh() const { return lastPhValue; }

private:
    enum Job { JOB_NONE, JOB_READ, JOB_REGS, JOB_SCAN };

    static const uint8_t RX_PIN = 4;
    static const uint8_t TX_PIN = 5;
    static const int MAX_SAMPLES = 30;
    static const int MAX_REGS = 20;
    static const uint32_t RESPONSE_TIMEOUT_MS = 500;
    static const uint32_t SCAN_TIMEOUT_MS = 250;

    ModbusSettings cfg;
    Job pending = JOB_NONE;
    int pendingA = 0;
    int pendingB = 0;
    int pendingC = 0;
    bool busy = false;
    bool lastOk = false;
    float lastPhValue = NAN;

    void loadConfig();
    void saveConfig();
    void openBus(uint32_t baud);
    void setDE(bool transmit);

    // 0 = thành công; còn lại là mã lỗi, xem errorText().
    int readRegisters(uint8_t addr, uint8_t fc, uint16_t start, uint16_t count, uint16_t *out, uint32_t timeoutMs);
    static const char *errorText(int code);

    void runRead(int samples, int intervalMs);
    void runRegs(int start, int count, int fc);
    void runScan();

    void publishEvent(const char *stage, const String &message, JsonDocument *extra = nullptr);
    void waitMs(uint32_t ms);
};

extern PhManager phManager;

#endif // PH_MANAGER_H
