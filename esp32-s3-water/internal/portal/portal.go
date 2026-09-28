package portal

import (
	"fmt"
	"strings"

	"esp32-s3-water/internal/config"
)

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

func ssidOptions(ssids []string, current string) string {
	if len(ssids) == 0 {
		return `<option value="">Không tìm thấy mạng nào</option>`
	}
	var b strings.Builder
	b.WriteString(`<option value="">-- Chọn WiFi xung quanh --</option>`)
	for _, s := range ssids {
		sel := ""
		if s == current {
			sel = " selected"
		}
		e := htmlEscape(s)
		b.WriteString(`<option value="` + e + `"` + sel + `>` + e + `</option>`)
	}
	return b.String()
}

// GetPortalHTML trả về trang cấu hình: chọn WiFi quét được, MQTT, device_id và topic
func GetPortalHTML(cfg config.DeviceConfig, apName string, ssids []string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="vi">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Cấu hình ESP32-S3 Water</title>
    <style>
        :root { --primary: #2563eb; --primary-hover: #1d4ed8; --bg: #0f172a; --card: #1e293b; --text: #f8fafc; --text-muted: #94a3b8; --border: #334155; --input-bg: #0f172a; }
        * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
        body { background: var(--bg); color: var(--text); display: flex; justify-content: center; align-items: center; min-height: 100vh; padding: 20px; }
        .card { background: var(--card); border: 1px solid var(--border); border-radius: 16px; padding: 28px; width: 100%%; max-width: 440px; box-shadow: 0 20px 25px -5px rgba(0,0,0,0.5); }
        .header { text-align: center; margin-bottom: 24px; }
        .header h1 { font-size: 20px; font-weight: 700; color: #38bdf8; margin-bottom: 6px; }
        .header p { font-size: 13px; color: var(--text-muted); }
        .badge { display: inline-block; background: rgba(56,189,248,0.15); color: #38bdf8; font-size: 12px; font-weight: 600; padding: 4px 10px; border-radius: 9999px; margin-top: 8px; border: 1px solid rgba(56,189,248,0.3); }
        .section-title { font-size: 13px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #38bdf8; margin: 18px 0 10px; border-bottom: 1px solid var(--border); padding-bottom: 4px; }
        .form-group { margin-bottom: 14px; }
        label { display: block; font-size: 13px; font-weight: 500; color: var(--text-muted); margin-bottom: 6px; }
        input, select { width: 100%%; padding: 10px 14px; background: var(--input-bg); border: 1px solid var(--border); border-radius: 8px; color: var(--text); font-size: 14px; outline: none; }
        input:focus, select:focus { border-color: var(--primary); }
        .hint { font-size: 11px; color: var(--text-muted); margin-top: 4px; }
        .hint code { color: #38bdf8; }
        .btn { width: 100%%; padding: 12px; background: var(--primary); color: white; border: none; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; margin-top: 18px; }
        .btn:hover { background: var(--primary-hover); }
        .footer { text-align: center; margin-top: 20px; font-size: 12px; color: var(--text-muted); }
    </style>
</head>
<body>
    <div class="card">
        <div class="header">
            <h1>ESP32-S3 WATER</h1>
            <p>Điều khiển bơm nạp, van xả, phao (TinyGo)</p>
            <div class="badge">AP: %s</div>
        </div>

        <form action="/save" method="POST">
            <div class="section-title">1. Kết nối WiFi</div>
            <div class="form-group">
                <label>WiFi xung quanh</label>
                <select onchange="if(this.value){document.getElementById('ssid').value=this.value}">%s</select>
            </div>
            <div class="form-group">
                <label>Tên WiFi (SSID)</label>
                <input type="text" id="ssid" name="wifi_ssid" value="%s" required placeholder="Chọn ở trên hoặc nhập tay">
            </div>
            <div class="form-group">
                <label>Mật khẩu WiFi</label>
                <input type="password" name="wifi_pass" value="%s" placeholder="Mật khẩu WiFi">
            </div>

            <div class="section-title">2. Cấu hình MQTT Broker</div>
            <div class="form-group">
                <label>Địa chỉ Broker (Host / IP)</label>
                <input type="text" name="mqtt_host" value="%s" required placeholder="192.168.1.10">
            </div>
            <div class="form-group">
                <label>Cổng kết nối (Port)</label>
                <input type="number" name="mqtt_port" value="%d" required>
            </div>
            <div class="form-group">
                <label>MQTT Username (Tùy chọn)</label>
                <input type="text" name="mqtt_user" value="%s" placeholder="Để trống nếu không có">
            </div>
            <div class="form-group">
                <label>MQTT Password (Tùy chọn)</label>
                <input type="password" name="mqtt_pass" value="%s" placeholder="Để trống nếu không có">
            </div>

            <div class="section-title">3. Thiết bị</div>
            <div class="form-group">
                <label>Mã thiết bị (Device ID)</label>
                <input type="text" name="device_id" value="%s" required placeholder="fish_water">
                <div class="hint">Gửi kèm trong mọi bản tin publish</div>
            </div>
            <div class="form-group">
                <label>Topic</label>
                <input type="text" name="topic" value="%s" required placeholder="fish/ten_thiet_bi">
                <div class="hint">Subscribe <code>&lt;topic&gt;/command</code>, publish <code>&lt;topic&gt;/status</code>, <code>/event</code>, <code>/telemetry</code>, <code>/log</code></div>
            </div>

            <button type="submit" class="btn">Lưu cấu hình & Kết nối</button>
        </form>

        <div class="footer">
            Giữ nút BOOT 3 giây để mở lại trang này.
        </div>
    </div>
</body>
</html>`,
		htmlEscape(apName),
		ssidOptions(ssids, cfg.WifiSSID),
		htmlEscape(cfg.WifiSSID),
		htmlEscape(cfg.WifiPass),
		htmlEscape(cfg.MqttHost),
		cfg.MqttPort,
		htmlEscape(cfg.MqttUser),
		htmlEscape(cfg.MqttPass),
		htmlEscape(cfg.DeviceID),
		htmlEscape(cfg.TopicPrefix()),
	)
}
