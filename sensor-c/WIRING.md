# Sơ đồ đấu nối cảm biến (ESP32-S3)

> **Ngừng dùng.** TDS đã chuyển sang bo `sensor-ph` (ESP32-C3), nhiệt độ lấy từ cảm biến pH RS485. Backend không còn gửi lệnh tới device `sensor`. Xem `sensor-ph/WIRING.md`.

Khớp firmware `sensor-c`: nhiệt độ DS18B20 và TDS. pH đã chuyển sang bo riêng `sensor-ph` (ESP32-C3 + cảm biến RS485, nguồn riêng), xem `sensor-ph/WIRING.md`. Tháo mạch PH-4502C và dây GPIO 6 / GPIO 7 khỏi bo này.

GPIO 5 và GPIO 9 là output, mức thấp lúc khởi động. Chỉ lên cao khi backend gửi lệnh `power` qua MQTT (xem cuối file).

## Nguồn

```
Cục 5 V (+) ── ESP32-S3 chân 5V
```

Chân 5V của ESP32-S3 chỉ để nhận điện vào. Cắm USB thì chân này không phát 5 V ra ngoài.

GND của cục nguồn, ESP32-S3, mạch TDS và DS18B20 nối chung. Không nối GND bo này với GND nguồn của bo pH.

## Nhiệt độ DS18B20

```
GPIO 5 -------------- đỏ / VDD
        |
       4,7 kΩ
        |
GPIO 4 -------------- vàng / DQ
GND ----------------- đen / GND
```

Điện trở 4,7 kΩ nằm giữa DQ và GPIO 5, không kéo lên đường 3,3 V luôn có điện.

## TDS Meter V1.0

```
GPIO 9 ── VCC
GPIO 8 ── A
GND ──── GND
```

Không có cầu chia trên chân A. Hai chân que TDS cắm vào jack trên mạch TDS. GPIO 9 chỉ lên cao khi có lệnh bật nguồn TDS.

## Chân ESP32-S3

| Chân | Việc |
|---|---|
| GPIO 4 | DQ nhiệt độ |
| GPIO 5 | VDD nhiệt độ |
| GPIO 8 | A của TDS |
| GPIO 9 | VCC của TDS |

## Lệnh MQTT

Topic `fish/<device_id>/command`. Backend bật nguồn, chờ, đọc rồi tắt; firmware không tự bật tắt khi đọc.

| Lệnh | Việc |
|---|---|
| `{"action":"power","sensor":"tds","state":true,"max_on_s":300}` | Bật nguồn TDS. `sensor` là `temp` hoặc `tds` |
| `{"action":"power","sensor":"tds","state":false}` | Tắt nguồn TDS |
| `{"action":"power","sensor":"all","state":false}` | Tắt cả hai |
| `{"action":"power_status"}` | Trả trạng thái nguồn |
| `{"action":"temp"}`, `{"action":"tds","samples":20}` | Đọc nhiệt độ, TDS. Chưa bật nguồn thì báo `sensor_error` |

Lệnh `ph` hoặc `power` với `sensor` là `ph` trả `command_error`: pH đã chuyển sang bo `sensor-ph`.

Mỗi lệnh `power` trả sự kiện `{"stage":"power","sensor":"tds","power":true}` trên topic `event`. Telemetry 30 giây có thêm `"power":{"temp":false,"tds":true}`.

Firmware vẫn tự tắt trong hai trường hợp:

- Bật quá `max_on_s` (mặc định 300, tối đa 1800) thì tự tắt và gửi `power_timeout`.
- Mất MQTT thì tắt cả hai.

Giá trị TDS là trung bình các mẫu sau khi bỏ 20% thấp nhất và 20% cao nhất.
