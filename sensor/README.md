# ESP32-S3 Sensor Module (TinyGo) & ESP32 Actuator Controller

Hệ thống điều khiển lấy mẫu & quan trắc nước hồ cá, kiến trúc **Dual-MCU** (ESP32 C++ Master + ESP32-S3 TinyGo Sensor Co-Processor).

---

## 🔌 Sơ đồ đấu nối giữa ESP32 và ESP32-S3 (UART Bridge)

| ESP32 Master (C++) | Chức năng | ESP32-S3 Sensor Node (TinyGo) | Ghi chú |
| :--- | :--- | :--- | :--- |
| **GPIO 23 (TX2)** | Truyền lệnh `CMD:` | **GPIO 44 (RX / Serial)** | ESP32 gửi lệnh đo |
| **GPIO 16 (RX2)** | Nhận dữ liệu `DATA:` | **GPIO 43 (TX / Serial)** | S3 trả kết quả đo JSON |
| **GND** | Mass chung | **GND** | **Bắt buộc nối chung GND** |

---

## 📌 Sơ đồ chân cảm biến trên ESP32-S3

| Cảm biến | Chân tín hiệu S3 | Chân cấp nguồn VCC | Ghi chú |
| :--- | :--- | :--- | :--- |
| **DS18B20 (Nhiệt độ)** | **GPIO 4** (OneWire) | **GPIO 5** (hoặc 3.3V) | Kéo trở 4.7kΩ lên 3.3V |
| **Cảm biến pH** | **GPIO 6** (ADC1) | **GPIO 7** (hoặc 5V) | Lọc trung vị 40 mẫu |
| **Cảm biến TDS** | **GPIO 8** (ADC1) | **GPIO 9** (hoặc 3.3V) | Lọc trung vị 30 mẫu |

---

## 📌 Sơ đồ chân thiết bị trên ESP32 (C++)

| Thiết bị | Chân ESP32 | Loại tín hiệu |
| :--- | :--- | :--- |
| **Relay Bơm nạp** | **GPIO 18** | Output (Kích mức LOW) |
| **Relay Van xả** | **GPIO 19** | Output (Kích mức LOW) |
| **Phao 1 (Thấp / pH)** | **GPIO 17** | Input (Pull-up) |
| **Phao 2 (Cao / TDS)** | **GPIO 4** | Input (Pull-up) |
| **Nút BOOT** | **GPIO 0** | Không dùng cho cấu hình. WiFi/MQTT lấy từ thông số mặc định |

---

## 🔄 Giao thức UART giữa 2 MCU (115200 Baud Rate)

1. **ESP32 gửi sang ESP32-S3:**
   * `CMD:PH\n` : Yêu cầu đo pH
   * `CMD:TDS\n` : Yêu cầu đo TDS
   * `CMD:TEMP\n` : Yêu cầu đo Nhiệt độ
   * `CMD:MEASURE_ALL\n` : Yêu cầu đo toàn bộ
   * `PING\n` : Kiểm tra kết nối

2. **ESP32-S3 phản hồi lại ESP32:**
   * `DATA:{"status":"success","sensors_measured":["ph"],"raw":{"ph":{"adc":2150,"voltage":1.732,"sample_count":40}}}\n`
   * `EVENT:{"stage":"measuring","message":"Đang đo pH..."}\n`
   * `ERROR:{"error":"Không tìm thấy cảm biến"}\n`
   * `PONG\n`

---

## 🚀 Hướng dẫn nạp code

### 1. Nạp code cho ESP32-S3 (TinyGo)
```bash
cd sensor
make docker-build
make flash PORT=/dev/cu.usbmodem2101
```

### 2. Nạp code cho ESP32 (C++)
```bash
pio run -t upload
```
