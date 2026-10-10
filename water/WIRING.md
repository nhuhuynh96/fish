# Sơ đồ đấu nối bơm nạp, bơm xả, phao (ESP32)

Khớp firmware `water`. GPIO 18 và GPIO 19 điều khiển relay kích mức thấp: chân ở mức LOW thì relay đóng, bơm chạy. Firmware không bao giờ bật bơm nạp và bơm xả cùng lúc.

## Nguồn

Hai nguồn riêng. Hai bơm không lấy điện từ nguồn của ESP32.

```
Nguồn 5 V (+) ──┬── ESP32 chân VIN (5V)
                ├── 1000 µF (+)  ┐ sát chân VIN / GND
                └── 100 nF       ┘
Nguồn 5 V (−) ──┴── ESP32 GND

Nguồn bơm (+), ví dụ 12 V ── COM relay 1, COM relay 2
Nguồn bơm (−) ────────────── (−) bơm nạp, (−) bơm xả
```

Tụ 1000 µF có cực: chân (−), phía có vạch, nối GND. Tụ 100 nF (ghi 104) không có cực.

Dùng module relay có optocoupler thì GND nguồn bơm không cần nối với GND ESP32. Dùng module relay không có optocoupler thì nối hai GND tại một điểm duy nhất, ngay chỗ module relay.

## Module relay 2 kênh (kích mức thấp, có optocoupler)

```
ESP32 3V3 ──── VCC   (phía tín hiệu, cấp cho LED optocoupler)
ESP32 GPIO 18 ─ IN1  (bơm nạp)
ESP32 GPIO 19 ─ IN2  (bơm xả)
ESP32 GND ──── GND

Nguồn 5 V (+) ── JD-VCC   (cuộn relay, bỏ jumper JD-VCC ↔ VCC)
Nguồn 5 V (−) ── GND phía JD-VCC
```

VCC phía tín hiệu phải là 3,3 V, không phải 5 V. Cấp 5 V vào VCC thì khi GPIO lên mức cao 3,3 V, LED optocoupler vẫn còn chênh khoảng 1,7 V, relay có thể không nhả và bơm chạy mãi.

Module không có jumper JD-VCC: cấp VCC 5 V, rồi đo thử. GPIO lên cao mà relay không nhả thì đổi module hoặc thêm transistor đệm.

## Bơm nạp và bơm xả

Hai bơm đấu giống nhau, bơm nạp qua relay 1, bơm xả qua relay 2.

```
Nguồn bơm (+) ── COM relay
                 NO relay ────┬──────────┬── (+) bơm
                              │          │
                     vạch → K │          │
                      1N4007  ▲         ═══ 100 nF
                            A │          │
                              │          │
Nguồn bơm (−) ────────────────┴──────────┴── (−) bơm
```

Diode và tụ 100 nF hàn ngay hai chân bơm, hoặc đầu dây sát bơm nếu bơm chìm kín. Đầu có vạch của diode nối (+) bơm. Gắn ngược thì diode chập nguồn khi bật bơm.

Dùng tiếp điểm NO (thường mở). Mất điện ESP32 hoặc relay thì bơm tắt.

Bơm 220 V thì không gắn diode. Thay bằng mạch RC (100 Ω nối tiếp 100 nF loại 275 V AC) song song với bơm, và dùng relay chịu được 220 V.

Bơm xả không nên chạy khô lâu. Lệnh `drain_on` không kèm `level` chạy đủ `duration` kể cả khi bình đã cạn, nên dùng `duration` ngắn (10–20 giây) khi xả nốt nước đọng.

## Phao mực nước

Mỗi phao là một công tắc hai dây. Nước chạm phao thì công tắc đóng, kéo GPIO xuống GND.

```
3V3 ── 10 kΩ ──┬── GPIO 17 ── dây phao 1 ── phao 1 (mực thấp) ── GND
               └── 100 nF ── GND

3V3 ── 10 kΩ ──┬── GPIO 4 ─── dây phao 2 ── phao 2 (mực cao) ─── GND
               └── 100 nF ── GND
```

Firmware đã bật điện trở kéo lên bên trong, nhưng điện trở đó khoảng 45 kΩ, yếu khi dây phao dài và chạy gần dây bơm. Điện trở 10 kΩ lên 3V3 và tụ 100 nF xuống GND, đặt sát chân ESP32, giúp phao không nhảy sai khi bơm chạy. Firmware lọc thêm 800 ms.

Phao 1 lắp thấp hơn phao 2. Firmware tính mực nước: không phao nào đóng là 0, phao 1 đóng là 1, phao 2 đóng là 2.

Phao loại hai trạng thái thường mở hoặc thường đóng tùy chiều lắp. Lật phao để khi nước dâng lên thì công tắc đóng. Kiểm tra bằng log khởi động: `GPIO17=0` nghĩa là phao 1 đang đóng.

## Đi dây

- Dây hai bơm mang dòng lớn. Đi cách board ESP32 và anten ít nhất 10 cm.
- Xoắn hai dây (+) và (−) của mỗi bơm lại với nhau.
- Dây phao không chạy song song sát dây bơm.

## Chân ESP32

| Chân | Việc |
|---|---|
| GPIO 18 | IN1 relay bơm nạp, LOW = bật |
| GPIO 19 | IN2 relay bơm xả, LOW = bật |
| GPIO 17 | Phao 1 (mực thấp), LOW = có nước |
| GPIO 4 | Phao 2 (mực cao), LOW = có nước |
| GPIO 0 | Nút BOOT, giữ 3 giây để mở AP cấu hình |

GPIO 16 và 17 bị PSRAM chiếm trên module ESP32-WROVER. Board đang dùng là ESP32 dev thường (WROOM), GPIO 17 dùng được. Đổi sang board WROVER thì phải dời phao 1 sang chân khác và sửa `FLOAT_LOW_PIN` trong firmware.

## Lệnh MQTT

Gửi lệnh vào `fish/water/command`. Kết quả trả về `fish/water/event`.

| Lệnh | Tham số | Dừng khi |
|---|---|---|
| `inlet_on` | `level` 1/2 (tùy chọn), `duration` giây (mặc định 60, 1–300) | Có `level`: phao đó đóng hoặc hết `duration`, cái nào trước. Không có `level`: hết `duration` |
| `drain_on` | `level` 1/2 (tùy chọn), `duration` giây (mặc định 60, 1–300) | Có `level`: phao đó tắt hoặc hết `duration`, cái nào trước. Không có `level`: hết `duration` |
| `inlet_off` | | Tắt bơm nạp ngay, hủy lệnh đang chạy |
| `drain_off` | | Tắt bơm xả ngay, hủy lệnh đang chạy |
| `level` | | Trả mực hiện tại ngay, sự kiện `water_level` |
| `clear_queue` | | Xóa hàng đợi, tắt cả hai bơm |

`drain_on` với `level: 2` xả tới khi phao 2 tắt, còn khoảng mực 1. Với `level: 1` xả tới khi phao 1 tắt, mực 0. Phao 2 không tự chặn bơm nạp: `inlet_on` không kèm `level` bơm đủ `duration` kể cả khi nước đã đầy.

`inlet_on` và `drain_on` vào hàng đợi. Lệnh sau chờ lệnh trước dừng hẳn mới chạy.

Ví dụ:

```json
{"action":"inlet_on","level":2,"duration":45}
{"action":"drain_on","level":1}
{"action":"drain_on","duration":15}
```

Sự kiện trên `fish/water/event`, mỗi sự kiện kèm `level`, `target` (0 nếu không có level), `duration_s`, `inlet_on`, `drain_on`, `float_1`, `float_2`:

| `stage` | Khi nào |
|---|---|
| `queued` | Đã xếp lệnh |
| `filling` / `draining` | Bơm bắt đầu chạy |
| `filled` / `drained` | Dừng vì tới điều kiện phao |
| `time_up` | Dừng vì hết `duration` |
| `already_at_level` | Phao đích đã đạt sẵn, không chạy |
| `pump_off` / `drain_off` | Tắt do lệnh off |
| `queue_cleared` | Đã xóa hàng đợi |
| `water_level` | Trả lời lệnh `level` |
| `command_error` | Lệnh không hỗ trợ hoặc JSON sai |
