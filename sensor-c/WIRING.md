# Sơ đồ đấu nối cảm biến (ESP32-S3)

Khớp firmware `sensor-c`. GPIO 5, GPIO 7 và GPIO 9 là output, mức thấp khi không đo, chỉ lên cao đúng lúc đo cảm biến đó.

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

GPIO 7 lên cao khoảng 2 giây khi có lệnh `ph`, rồi về thấp. Lúc chân này thấp, mạch pH mất điện và GPIO 6 về 0 V.

## TDS Meter V1.0

```
GPIO 9 ── VCC
GPIO 8 ── A
GND ──── GND
```

Không có cầu chia trên chân A. Hai chân que TDS cắm vào jack trên mạch TDS. GPIO 9 chỉ lên cao khi có lệnh `tds`.

## Chân ESP32-S3

| Chân | Việc |
|---|---|
| GPIO 4 | DQ nhiệt độ |
| GPIO 5 | VDD nhiệt độ |
| GPIO 6 | Po pH, sau cầu chia 10 kΩ / 20 kΩ |
| GPIO 7 | Bật nguồn pH |
| GPIO 8 | A của TDS |
| GPIO 9 | VCC của TDS |
