# Bo pH RS485 (ESP32-C3 Super Mini)

Khớp firmware `sensor-ph`. Cảm biến pH RS485 (Modbus RTU) cấp điện liên tục; ESP32-C3 chỉ đọc khi backend gửi lệnh. Device ID cố định `ph`, topic `fish/ph/...`.

## Nguồn

Bo này dùng nguồn 12 V riêng (nguồn 2), không chung GND với nguồn của bơm, bo water và sensor-c. Hai hệ chỉ chung nước trong bình.

```
Nguồn 2: 12V (+) ─┬──────────────── V+ cảm biến pH (nâu / đỏ)
                  └── IN+ hạ áp
         12V (−) ─┬──────────────── GND cảm biến pH (đen)
                  └── IN− hạ áp

Hạ áp OUT+ 5.0V ──┬── 470 µF (+) ┐ sát chân 5V ESP32-C3
                  ├── 100 nF     ┘
                  └── chân 5V ESP32-C3
Hạ áp OUT− ───────┴── GND ESP32-C3
```

- Chỉnh mạch hạ áp đúng 5.0 V bằng đồng hồ trước khi nối ESP32-C3.
- Cảm biến cần 5 V thay vì 12 V (xem datasheet) thì nối V+ vào 5 V của hạ áp.
- Adapter 12 V dùng loại 2 chấu (cách ly). Loại 3 chấu nối cực âm với dây đất sẽ nối GND hai nguồn qua ổ điện.
- Cắm USB vào ESP32-C3 để nạp firmware: rút dây 5 V từ hạ áp trước. Nhiều bo Super Mini nối thẳng 5 V của USB với chân 5V.
- Không cắm USB máy tính vào bo pH và bo water / sensor-c cùng lúc: máy tính sẽ nối GND hai hệ.

## Module UART TTL ↔ RS485

```
ESP32-C3 3V3 ──── VCC module
ESP32-C3 GND ──── GND module
ESP32-C3 GPIO5 (TX) ── TXD module
ESP32-C3 GPIO4 (RX) ── RXD module
ESP32-C3 GPIO6 ─────── DE + RE nối chung   (chỉ khi module có chân DE/RE)

Module A ──── A cảm biến (vàng)
Module B ──── B cảm biến (xanh)
```

- Module bốn chân (VCC, TXD, RXD, GND) tự đổi chiều truyền, không cần DE. Mặc định `de_pin = -1`.
- Module có chân DE/RE (MAX485): nối DE và RE với GPIO 6 rồi gửi `{"action":"modbus_config","de_pin":6}`.
- Module "UART TTL to RS485 V2" đang dùng ghi nhãn theo vi điều khiển: TXD nối TX của ESP32, RXD nối RX. Module MAX485 (RO/DI) thì ngược lại: RO → GPIO 4, DI ← GPIO 5. Không đọc được thì đảo TX/RX trước, rồi đảo A/B.
- Đèn TX/RX trên module chỉ nháy khoảng 20 ms mỗi lần đọc ở 4800 baud, gần như không thấy được.
- Dây A/B dài hơn khoảng 10 m: gắn điện trở 120 Ω giữa A và B ở đầu cảm biến (nhiều module có sẵn).

## Chân ESP32-C3

| Chân | Việc |
|---|---|
| GPIO 4 | RX UART1, nối RXD module V2 (hoặc RO của MAX485) |
| GPIO 5 | TX UART1, nối TXD module V2 (hoặc DI của MAX485) |
| GPIO 6 | DE/RE (tùy chọn) |
| GPIO 8 | LED xanh trên bo: sáng khi đã nối MQTT |
| GPIO 9 | Nút BOOT: giữ 3 giây để mở AP cấu hình `ESP32C3-PH-xxxx` |

Không dùng GPIO 2, 8, 9 cho việc khác: đây là các chân quyết định chế độ khởi động.

## Cảm biến đang dùng: SN-30030PH-N01-201T

- Nguồn 7–30 VDC, cấp 12 V. Dây đỏ V+, đen GND, vàng A, xanh dương B.
- Modbus: địa chỉ 1, 4800 8N1, function 3 (function 4 cũng trả cùng giá trị).
- Thanh ghi 0 là pH × 100, thanh ghi 1 là nhiệt độ × 10 (°C). Thanh ghi 2–4 chưa rõ ý nghĩa.

Đây cũng là cấu hình mặc định của firmware.

## Lần đầu: tìm cấu hình Modbus (cảm biến khác)

Mặc định: địa chỉ 1, 4800 8N1, function 3, pH ở thanh ghi 0 nhân 0.01, nhiệt độ ở thanh ghi 1 nhân 0.1.

1. Nạp firmware, kết nối WiFi `ESP32C3-PH-xxxx`, mở http://192.168.4.1, nhập WiFi và MQTT broker `192.168.1.10`.
2. Dò thiết bị: gửi `{"action":"scan"}` vào `fish/ph/command`. Sự kiện `scan_result` liệt kê địa chỉ và baud trả lời.
3. Đặt địa chỉ và baud tìm được: `{"action":"modbus_config","addr":1,"baud":9600}`.
4. Đọc thô một dải thanh ghi: `{"action":"read_regs","start":0,"count":10}`. Nhúng cảm biến vào dung dịch 6.86: thanh ghi nào ra khoảng 686 (hoặc 6860) là pH. Thử `"fc":4` nếu fc 3 báo lỗi.
5. Lưu thanh ghi và hệ số: `{"action":"modbus_config","ph_reg":0,"ph_scale":0.01}`. Có nhiệt độ thì thêm `"temp_reg":1,"temp_scale":0.1`.
6. Thử đọc: `{"action":"ph"}`. Kết quả về `fish/ph/sensor_data`.

Cấu hình Modbus lưu trong NVS, nạp lại firmware không mất.

## Lệnh MQTT

Gửi vào `fish/ph/command`.

| Lệnh | Việc |
|---|---|
| `{"action":"ph","samples":10,"interval_ms":500}` | Đọc pH nhiều lần (1–30, cách 100–5000 ms), lấy trung vị, gửi `sensor_data` |
| `{"action":"modbus_config", ...}` | Đổi và lưu `addr`, `baud` (2400–38400), `fc` (3/4), `ph_reg`, `ph_scale`, `temp_reg` (-1 = không đọc), `temp_scale`, `de_pin` |
| `{"action":"get_config"}` | Trả cấu hình hiện tại, sự kiện `modbus_config` |
| `{"action":"read_regs","start":0,"count":10,"fc":3}` | Đọc thô tối đa 20 thanh ghi, sự kiện `regs` |
| `{"action":"scan"}` | Thử địa chỉ 1–10 ở 4800/9600/19200 baud, sự kiện `scan_result` |

`sensor_data` gửi pH thô của cảm biến; backend áp hiệu chỉnh `ph = ph_gain × ph_raw + ph_offset` của device `ph`:

```json
{"device_id":"ph","status":"success","sensors_measured":["ph"],"data":{"ph":7.12},"raw":{"ph_reg":712,"temp_c":26.4,"ok":10,"fail":0}}
```

Sự kiện trên `fish/ph/event`:

| `stage` | Khi nào |
|---|---|
| `sensor_error` | Modbus không trả lời, sai CRC, hoặc pH đọc ra ngoài 0–14 |
| `command_error` | Lệnh sai hoặc đang bận |
| `modbus_config` | Trả lời `modbus_config` / `get_config` |
| `regs` | Trả lời `read_regs` |
| `scan_started` / `scan_result` | Lệnh `scan` |

Telemetry 30 giây có `rssi`, `uptime`, `state`, `modbus_ok` (lần đọc gần nhất có thành công không) và `last_ph`.

## Nạp firmware

```bash
cd sensor-ph
make ports
make flash PORT=/dev/cu.usbmodemXXXX
make monitor PORT=/dev/cu.usbmodemXXXX
```
