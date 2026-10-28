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

Bơm xả không nên chạy khô lâu. Lệnh `drain_on` không kèm `until_empty` chạy đủ `duration` kể cả khi bình đã cạn, nên dùng `duration` ngắn (10–20 giây) khi xả nốt nước đọng.

## Phao mực nước

Chỉ một phao, lắp ở mực "đầy" (đủ ngập cả ba que đo). Phao là công tắc hai dây: nước chạm phao thì công tắc đóng, kéo GPIO 17 xuống GND.

```
3V3 ── 10 kΩ ──┬── GPIO 17 ── dây phao ── phao (mực đầy) ── GND
               └── 100 nF ── GND
```

Firmware đã bật điện trở kéo lên bên trong, nhưng điện trở đó khoảng 45 kΩ, yếu khi dây phao dài và chạy gần dây bơm. Điện trở 10 kΩ lên 3V3 và tụ 100 nF xuống GND, đặt sát chân ESP32, giúp phao không nhảy sai khi bơm chạy. Firmware chỉ đổi trạng thái phao khi chân giữ nguyên 800 ms.

GPIO 4 (phao 2 cũ) không còn dùng, tháo dây ra.

Phao loại hai trạng thái thường mở hoặc thường đóng tùy chiều lắp. Lật phao để khi nước dâng lên thì công tắc đóng. Kiểm tra bằng log khởi động: `GPIO17=0 (đầy)` nghĩa là phao đang đóng.

## Đi dây

- Dây hai bơm mang dòng lớn. Đi cách board ESP32 và anten ít nhất 10 cm.
- Xoắn hai dây (+) và (−) của mỗi bơm lại với nhau.
- Dây phao không chạy song song sát dây bơm.

## Chân ESP32

| Chân | Việc |
|---|---|
| GPIO 18 | IN1 relay bơm nạp, LOW = bật |
| GPIO 19 | IN2 relay bơm xả, LOW = bật |
| GPIO 17 | Phao (mực đầy), LOW = có nước |
| GPIO 0 | Nút BOOT, giữ 3 giây để mở AP cấu hình |

GPIO 16 và 17 bị PSRAM chiếm trên module ESP32-WROVER. Board đang dùng là ESP32 dev thường (WROOM), GPIO 17 dùng được. Đổi sang board WROVER thì phải dời phao sang chân khác và sửa `FLOAT_PIN` trong firmware.

## Lệnh MQTT

Gửi lệnh vào `fish/water/command`. Kết quả trả về `fish/water/event`. Mạch chỉ làm theo lệnh; backend quyết định khi nào bơm, khi nào xả.

| Lệnh | Tham số | Dừng khi |
|---|---|---|
| `inlet_on` | `until_full` (mặc định `true`), `duration` giây (mặc định 60, 1–300) | `until_full`: phao đầy hoặc hết `duration`, cái nào trước. `until_full: false`: hết `duration` |
| `drain_on` | `until_empty` (mặc định `false`), `duration` giây (mặc định 60, 1–300) | `until_empty`: phao tắt hoặc hết `duration`, cái nào trước. Không có: hết `duration` |
| `inlet_off` | | Tắt bơm nạp ngay |
| `drain_off` | | Tắt bơm xả ngay |
| `stop` | | Tắt cả hai bơm (`clear_queue` cũng làm việc này) |
| `float` | | Trả trạng thái phao ngay, sự kiện `water_level` (`level` là tên cũ, vẫn nhận) |

Không có hàng đợi. `inlet_on` hoặc `drain_on` đến lúc đang có bơm chạy thì bị từ chối bằng `command_error`; gửi `stop` hoặc lệnh off trước.

Firmware vẫn tự tắt bơm khi:

- tới phao (nếu lệnh có `until_full` / `until_empty`),
- hết `duration`,
- mất MQTT.

Hai bơm không bao giờ chạy cùng lúc.

Ví dụ một lượt đo:

```json
{"action":"inlet_on","until_full":true,"duration":180}
{"action":"drain_on","until_empty":true,"duration":120}
{"action":"drain_on","duration":20}
```

Sự kiện trên `fish/water/event`, mỗi sự kiện kèm `float` (phao đầy hay chưa), `level` (1 nếu phao đầy, 0 nếu chưa), `inlet_on`, `drain_on`. Khi đang chạy bơm có thêm `until_float` và `duration_s`.

| `stage` | Khi nào |
|---|---|
| `filling` / `draining` | Bơm bắt đầu chạy |
| `filled` | Bơm nạp dừng vì phao đầy |
| `drained` | Bơm xả dừng vì phao tắt |
| `time_up` | Dừng vì hết `duration` |
| `already_full` | `inlet_on` với `until_full` khi phao đã đầy, không bơm |
| `already_empty` | `drain_on` với `until_empty` khi phao đã tắt, không xả |
| `pump_off` / `drain_off` / `stopped` | Tắt do lệnh |
| `float_changed` | Phao đổi trạng thái |
| `water_level` | Trả lời lệnh `float` |
| `command_error` | Lệnh không hỗ trợ, JSON sai, hoặc đang có bơm chạy |

Telemetry 30 giây có `state` (`IDLE`, `FILLING_WATER`, `DRAINING_WATER`) và `float`.
