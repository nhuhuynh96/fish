#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>
#include <vector>
#include <queue>

enum SensorType {
    SENSOR_TEMP = 0,
    SENSOR_PH = 1,
    SENSOR_TURBIDITY = 2,
    SENSOR_TDS = 3
};

enum SamplingState {
    STATE_IDLE,
    STATE_FILLING,      // Bơm nước vào buồng đo
    STATE_STABILIZING,  // Chờ nước ổn định
    STATE_MEASURING,    // Đang cấp nguồn và đo tuần tự từng cảm biến
    STATE_DRAINING,     // Xả nước khỏi buồng đo
    STATE_PUBLISHING    // Gửi kết quả lên MQTT
};

enum CommandType {
    CMD_MEASURE,
    CMD_PUMP,
    CMD_STATUS
};

struct PendingCommand {
    CommandType type = CMD_STATUS;
    std::vector<String> sensors;
    String pumpTarget = "inlet";
    bool pumpState = false;
};

struct SensorResult {
    float temperature = 0.0;
    bool hasTemp = false;

    float ph = 0.0;
    bool hasPh = false;

    float turbidity = 0.0;
    bool hasTurbidity = false;

    float tds = 0.0;
    bool hasTds = false;

    unsigned long startTime = 0;
    unsigned long durationMs = 0;
};

class SamplingManager {
public:
    SamplingManager();
    void begin(bool simulation = true);
    void handle();

    // Xếp hàng lệnh MQTT JSON để xử lý tuần tự
    void enqueueMeasure(const std::vector<String> &sensors);
    void enqueuePump(const String &target, bool state);
    void enqueueStatus();

    // Tiếp nhận yêu cầu đo (chạy ngay / gộp / xếp chu trình kế)
    bool requestMeasurement(const std::vector<String> &sensors);

    // Điều khiển bơm thủ công ("inlet" / "drain", true = ON, false = OFF)
    void setManualPump(const String &target, bool state);

    void publishStatus();

    bool isBusy() const { return currentState != STATE_IDLE; }
    size_t queueSize() const { return commandQueue.size(); }
    SamplingState getState() const { return currentState; }
    String getStateName() const;

private:
    SamplingState currentState = STATE_IDLE;
    bool simulationMode = true;

    // Hàng đợi lệnh MQTT (FIFO)
    std::queue<PendingCommand> commandQueue;

    // Hàng đợi cảm biến cho lượt đo hiện tại
    std::vector<SensorType> activeQueue;
    // Cảm biến đã hoàn thành trong chu trình hiện tại
    std::vector<SensorType> completedInCycle;
    // Hàng đợi cho chu trình tiếp theo (nếu nhận lệnh khi đang xả nước)
    std::vector<SensorType> nextPendingQueue;

    // Chỉ số cảm biến đang được đo trong activeQueue
    int currentMeasuringIndex = -1;

    // Bộ nhớ kết quả đo
    SensorResult currentResult;

    // Quản lý thời gian FSM
    unsigned long stateTimer = 0;
    unsigned long stepDuration = 0;
    unsigned long floatFullSince = 0;
    unsigned long floatEmptySince = 0;

    // Timer riêng cho bơm/van thủ công (KHÔNG dùng chung stateTimer của FSM)
    unsigned long manualInletStartedAt = 0;
    unsigned long manualDrainStartedAt = 0;
    bool manualInletActive = false;
    bool manualDrainActive = false;

    // Cấu hình thời gian mô phỏng / thực tế (ms)
    const unsigned long FILL_SIM_TIME = 4000;       // Thời gian bơm nước (mô phỏng 4s)
    const unsigned long STABILIZE_TIME = 1000;      // Chờ nước lắng (1s)
    const unsigned long SENSOR_SAMPLE_TIME = 1500;  // Thời gian cấp nguồn & đọc 1 cảm biến (1.5s)
    const unsigned long DRAIN_SIM_TIME = 3000;      // Thời gian xả nước (mô phỏng 3s)
    const unsigned long MAX_FILL_TIME = 60000;      // Tự tắt bơm nếu phao không báo đầy sau 60s
    const unsigned long MAX_DRAIN_TIME = 60000;     // Tự tắt van xả nếu phao không báo cạn sau 60s
    const unsigned long FLOAT_DEBOUNCE_TIME = 300;  // Chống nhiễu tiếp điểm phao

    // Relay 2 kênh + 2 phao (cạnh trái ESP32)
    const uint8_t INLET_PUMP_PIN = 18;     // Relay IN1 - bơm nạp
    const uint8_t DRAIN_VALVE_PIN = 19;    // Relay IN2 - van xả
    const uint8_t FLOAT_FULL_PIN = 4;      // Phao đầy
    const uint8_t FLOAT_EMPTY_PIN = 17;    // Phao cạn
    const uint8_t PUMP_ON_LEVEL = LOW;     // Relay kích mức LOW
    const uint8_t PUMP_OFF_LEVEL = HIGH;
    const uint8_t FLOAT_FULL_LEVEL = LOW;  // Phao đầy: nối GPIO với GND
    const uint8_t FLOAT_EMPTY_LEVEL = LOW; // Phao cạn: nối GPIO với GND

    // TDS Meter V1.0 (analog 0~2.3V) -> GPIO35 (ADC1, dùng được khi WiFi bật)
    const uint8_t TDS_SENSOR_PIN = 35;
    const float TDS_VREF = 3.3f;
    const int TDS_ADC_MAX = 4095;
    static const int TDS_SAMPLE_COUNT = 30;

    // Cảm biến độ đục/cặn analog (A/D/V/G) -> GPIO34 (ADC1), nguồn 3V3
    // Biến trở trên mạch chỉ chỉnh ngưỡng chân D/L1, KHÔNG ảnh hưởng chân A.
    const uint8_t TURBIDITY_SENSOR_PIN = 34;
    const float TURBIDITY_VREF = 3.3f;
    static const int TURBIDITY_SAMPLE_COUNT = 30;
    // Hiệu chuẩn 3V3 theo dải thực tế (~1.0V đục -> ~2.15V trong)
    const float TURB_V_CLEAR = 2.15f;
    const float TURB_V_DIRTY = 1.00f;
    const float TURB_NTU_MAX = 1000.0f;

    // PH-4502C: Po (analog) -> GPIO32 (ADC1). Nguồn module V+ = 5V.
    // POT gần BNC: hiệu chuẩn offset pH7 ≈ 2.5V tại Po.
    // POT còn lại: chỉ ngưỡng chân Do, không dùng khi đọc analog.
    const uint8_t PH_SENSOR_PIN = 32;
    const float PH_VREF = 3.3f;
    static const int PH_SAMPLE_COUNT = 40;
    const float PH_NEUTRAL_VOLTAGE = 2.50f; // pH 7.0 sau khi chỉnh POT offset
    const float PH_SLOPE = 0.18f;           // ~0.18 V / pH

    // Transistor 2N3904 low-side (Base qua 1k): HIGH = cấp nguồn cảm biến
    const uint8_t PH_POWER_PIN = 14;         // 2N3904 #1 -> GND pH
    const uint8_t TURBIDITY_POWER_PIN = 33;  // 2N3904 #2 -> GND turbidity
    const uint8_t TDS_POWER_PIN = 25;        // 2N3904 #3 -> GND TDS
    const uint8_t SENSOR_POWER_ON = HIGH;
    const uint8_t SENSOR_POWER_OFF = LOW;
    const unsigned long SENSOR_POWER_SETTLE_MS = 1200; // Chờ module ổn định sau khi bật

    void transitionTo(SamplingState newState);
    void processNextSensor();
    void measureSensor(SensorType type);
    void finishCycle();
    void processCommandQueue();
    bool isWaterFull() const;
    bool isWaterEmpty() const;
    void setInletPump(bool enabled);
    void setDrainValve(bool enabled);
    void setSensorPower(SensorType type, bool enabled);
    void powerOffAllSensors();
    float readRealTDS(float temperatureC);
    float readRealTurbidity();
    float readRealPH();
    int medianFilter(int *buffer, int count) const;
    
    // Gửi sự kiện log realtime qua MQTT
    void emitEvent(const String &stage, const String &message, const String &extraJson = "");

    // Chuyển đổi tên chuỗi thành Enum Sensor
    bool parseSensorName(const String &name, std::vector<SensorType> &outList);
    String getSensorName(SensorType type) const;

    // Các hàm mô phỏng dữ liệu ngẫu nhiên chân thực
    float simTemperature();
    float simPH();
    float simTurbidity();
    float simTDS();
};

extern SamplingManager samplingManager;

#endif // SAMPLING_MANAGER_H
