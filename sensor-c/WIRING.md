# Sơ đồ đấu nối cảm biến (ESP32-S3)

Khớp firmware `sensor-c`. GPIO 5, GPIO 7 và GPIO 9 là output, mức thấp lúc khởi động. Chỉ lên cao khi backend gửi lệnh `power` qua MQTT (xem cuối file).

## Nguồn

```
Cục 5 V (+) ── ESP32-S3 chân 5V
            └─ E 2N3906  C ──── VCC mạch pH
```

Chân 5V của ESP32-S3 chỉ để nhận điện vào. Cắm USB thì chân này không phát 5 V ra ngoài.

GND của cục nguồn, ESP32-S3, mạch pH, mạch TDS và DS18B20 nối chung.

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

## pH

```
5 V ── E 2N3906  C ──── VCC mạch pH
          B
          |
         1 kΩ
          |
     10 kΩ├──── C 2N3904  E ── GND
     |    |
    5 V   B
          |
         1 kΩ
          |
        GPIO 7

GND mạch pH ── GND

Po ── 10 kΩ ── GPIO 6 ── 20 kΩ ── GND
```

Điện trở 1 kΩ chỉ trên đường điều khiển transistor. Đường 5 V sang VCC mạch pH đi qua 2N3906, không qua điện trở 1 kΩ. Que pH cắm vào mạch pH.

GPIO 7 lên cao khi có lệnh bật nguồn pH, về thấp khi có lệnh tắt. Lúc chân này thấp, mạch pH mất điện và GPIO 6 về 0 V.

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
| GPIO 6 | Po pH, sau cầu chia 10 kΩ / 20 kΩ |
| GPIO 7 | Bật nguồn pH |
| GPIO 8 | A của TDS |
| GPIO 9 | VCC của TDS |

## Lệnh MQTT

Topic `fish/<device_id>/command`. Backend bật nguồn, chờ, đọc rồi tắt; firmware không tự bật tắt khi đọc.

| Lệnh | Việc |
|---|---|
| `{"action":"power","sensor":"ph","state":true,"max_on_s":300}` | Bật nguồn pH. `sensor` là `ph`, `temp` hoặc `tds` |
| `{"action":"power","sensor":"ph","state":false}` | Tắt nguồn pH |
| `{"action":"power","sensor":"all","state":false}` | Tắt cả ba |
| `{"action":"power_status"}` | Trả trạng thái nguồn |
| `{"action":"ph","samples":40,"interval_ms":100}` | Đọc pH. Chưa bật nguồn thì báo `sensor_error` |
| `{"action":"temp"}`, `{"action":"tds","samples":20}` | Đọc nhiệt độ, TDS |

Mỗi lệnh `power` trả sự kiện `{"stage":"power","sensor":"ph","power":true}` trên topic `event`. Telemetry 30 giây có thêm `"power":{"temp":false,"ph":true,"tds":false}`.

Firmware vẫn tự tắt trong ba trường hợp:

- pH và TDS không được có điện cùng lúc. Bật cái này khi cái kia đang bật thì bị từ chối bằng `command_error`.
- Bật quá `max_on_s` (mặc định 300, tối đa 1800) thì tự tắt và gửi `power_timeout`.
- Mất MQTT thì tắt cả ba.

Giá trị đọc là trung bình các mẫu sau khi bỏ 20% thấp nhất và 20% cao nhất.
