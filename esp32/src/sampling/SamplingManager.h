#ifndef SAMPLING_MANAGER_H
#define SAMPLING_MANAGER_H

#include <Arduino.h>
#include <vector>

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

    // Tiếp nhận yêu cầu đo từ MQTT (Backend điều phối)
    // Ví dụ: {"all"}, {"temp"}, {"temp", "ph"}, {"tds"}
    bool requestMeasurement(const std::vector<String> &sensors);

    // Điều khiển bơm thủ công ("inlet" / "drain", true = ON, false = OFF)
    void setManualPump(const String &target, bool state);

    bool isBusy() const { return currentState != STATE_IDLE; }
    SamplingState getState() const { return currentState; }
    String getStateName() const;

private:
    SamplingState currentState = STATE_IDLE;
    bool simulationMode = true;

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

    // Cấu hình thời gian mô phỏng / thực tế (ms)
    const unsigned long FILL_SIM_TIME = 4000;       // Thời gian bơm nước (mô phỏng 4s)
    const unsigned long STABILIZE_TIME = 1000;      // Chờ nước lắng (1s)
    const unsigned long SENSOR_SAMPLE_TIME = 1500;  // Thời gian cấp nguồn & đọc 1 cảm biến (1.5s)
    const unsigned long DRAIN_SIM_TIME = 3000;      // Thời gian xả nước (mô phỏng 3s)

    void transitionTo(SamplingState newState);
    void processNextSensor();
    void measureSensor(SensorType type);
    void finishCycle();
    
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
