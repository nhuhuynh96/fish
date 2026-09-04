#include "WebPortal.h"

WebPortal webPortal;

const byte DNS_PORT = 53;
const IPAddress AP_IP(192, 168, 4, 1);
const IPAddress AP_NETMASK(255, 255, 255, 0);

WebPortal::WebPortal() : server(80) {}

void WebPortal::startPortal() {
    portalRunning = true;

    // Sinh tên AP duy nhất dựa vào MAC của ESP32
    uint64_t chipid = ESP.getEfuseMac();
    char apName[32];
    snprintf(apName, sizeof(apName), "ESP32-Setup-%04X", (uint16_t)(chipid & 0xFFFF));
    apSSID = String(apName);

    Serial.println("\n[Portal] =========================================");
    Serial.printf("[Portal] Khởi động AP Mode: %s\n", apSSID.c_str());
    Serial.println("[Portal] IP Cấu hình: http://192.168.4.1");
    Serial.println("[Portal] =========================================\n");

    // Bật AP + STA để có thể vừa phát AP vừa quét WiFi xung quanh
    WiFi.mode(WIFI_AP_STA);
    WiFi.softAPConfig(AP_IP, AP_IP, AP_NETMASK);
    WiFi.softAP(apSSID.c_str());

    // Khởi động DNS Server để bắt mọi truy vấn (Captive Portal)
    dnsServer.setErrorReplyCode(DNSReplyCode::NoError);
    dnsServer.start(DNS_PORT, "*", AP_IP);

    setupRoutes();
    server.begin();
    Serial.println("[Portal] Web Server & DNS Server đã sẵn sàng!");
}

void WebPortal::stopPortal() {
    if (portalRunning) {
        dnsServer.stop();
        server.stop();
        WiFi.softAPdisconnect(true);
        portalRunning = false;
        Serial.println("[Portal] Đã tắt AP Mode & Web Server");
    }
}

void WebPortal::handlePortal() {
    if (!portalRunning) return;
    dnsServer.processNextRequest();
    server.handleClient();
}

bool WebPortal::isCaptivePortal() {
    String host = server.hostHeader();
    if (host != "192.168.4.1" && host != apSSID + ".local") {
        return true;
    }
    return false;
}

void WebPortal::setupRoutes() {
    server.on("/", HTTP_GET, [this]() { handleRoot(); });
    server.on("/save", HTTP_POST, [this]() { handleSave(); });
    server.on("/scan", HTTP_GET, [this]() { handleScan(); });
    server.on("/reset", HTTP_POST, [this]() { handleReset(); });

    // Các endpoint xác thực Captive Portal của Android / iOS / Windows
    server.on("/generate_204", HTTP_GET, [this]() { handleRoot(); });
    server.on("/gen_204", HTTP_GET, [this]() { handleRoot(); });
    server.on("/hotspot-detect.html", HTTP_GET, [this]() { handleRoot(); });
    server.on("/canonical.html", HTTP_GET, [this]() { handleRoot(); });
    server.on("/connecttest.txt", HTTP_GET, [this]() { handleRoot(); });
    server.on("/ncsi.txt", HTTP_GET, [this]() { handleRoot(); });

    server.onNotFound([this]() { handleNotFound(); });
}

void WebPortal::handleNotFound() {
    if (isCaptivePortal()) {
        server.sendHeader("Location", String("http://") + AP_IP.toString() + "/", true);
        server.send(302, "text/plain", "");
    } else {
        server.send(404, "text/plain", "404: Not Found");
    }
}

void WebPortal::handleScan() {
    Serial.println("[Portal] Đang quét danh sách mạng WiFi...");
    int n = WiFi.scanNetworks(false, true); // Quét bất đồng bộ hoặc nhanh
    String json = "[";
    for (int i = 0; i < n; ++i) {
        if (i > 0) json += ",";
        json += "{";
        json += "\"ssid\":\"" + WiFi.SSID(i) + "\",";
        json += "\"rssi\":" + String(WiFi.RSSI(i)) + ",";
        json += "\"secure\":" + String(WiFi.encryptionType(i) != WIFI_AUTH_OPEN ? 1 : 0);
        json += "}";
    }
    json += "]";
    server.send(200, "application/json", json);
}

void WebPortal::handleReset() {
    configManager.clearConfig();
    String resp = "{\"status\":\"ok\",\"message\":\"Đã xóa cấu hình. ESP32 sẽ khởi động lại...\"}";
    server.send(200, "application/json", resp);
    delay(1000);
    ESP.restart();
}

void WebPortal::handleSave() {
    if (!server.hasArg("wifi_ssid") || !server.hasArg("device_id")) {
        server.send(400, "text/plain", "Thiếu tham số cấu hình!");
        return;
    }

    DeviceConfig newConfig;
    newConfig.wifi_ssid = server.arg("wifi_ssid");
    newConfig.wifi_pass = server.arg("wifi_pass");
    newConfig.mqtt_host = server.arg("mqtt_host");
    newConfig.mqtt_port = server.hasArg("mqtt_port") && server.arg("mqtt_port").toInt() > 0 
                            ? (uint16_t)server.arg("mqtt_port").toInt() : 1883;
    newConfig.mqtt_user = server.arg("mqtt_user");
    newConfig.mqtt_pass = server.arg("mqtt_pass");
    newConfig.device_id = server.arg("device_id");

    newConfig.wifi_ssid.trim();
    newConfig.wifi_pass.trim();
    newConfig.mqtt_host.trim();
    newConfig.mqtt_user.trim();
    newConfig.mqtt_pass.trim();
    newConfig.device_id.trim();

    if (newConfig.wifi_ssid.isEmpty()) {
        server.send(400, "text/html", "<h3 style='color:red;'>Lỗi: Tên WiFi không được để trống!</h3><a href='/'>Quay lại</a>");
        return;
    }

    if (newConfig.device_id.isEmpty()) {
        uint64_t chipid = ESP.getEfuseMac();
        newConfig.device_id = "esp32_" + String((uint16_t)(chipid & 0xFFFF), HEX);
    }

    configManager.saveConfig(newConfig);

    String successHTML = R"rawhtml(
<!DOCTYPE html>
<html lang="vi">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Đã Lưu Cấu Hình</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
            color: #f8fafc;
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            padding: 20px;
            box-sizing: border-box;
        }
        .card {
            background: rgba(30, 41, 59, 0.85);
            backdrop-filter: blur(12px);
            border: 1px solid rgba(255, 255, 255, 0.1);
            border-radius: 20px;
            padding: 32px;
            max-width: 420px;
            width: 100%;
            text-align: center;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.4);
        }
        .icon {
            width: 64px;
            height: 64px;
            background: linear-gradient(135deg, #10b981, #059669);
            border-radius: 50%;
            display: inline-flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 20px;
            box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);
        }
        .icon svg {
            width: 32px;
            height: 32px;
            fill: white;
        }
        h2 { margin: 0 0 12px; font-size: 22px; color: #fff; }
        p { color: #94a3b8; font-size: 14px; line-height: 1.6; margin: 0 0 24px; }
        .spinner {
            width: 36px;
            height: 36px;
            border: 3px solid rgba(255, 255, 255, 0.1);
            border-top: 3px solid #38bdf8;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto;
        }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">
            <svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
        </div>
        <h2>Lưu Cấu Hình Thành Công!</h2>
        <p>ESP32 đang khởi động lại và kết nối vào mạng WiFi <b>)rawhtml" + newConfig.wifi_ssid + R"rawhtml(</b>.<br>Vui lòng kết nối điện thoại lại vào mạng WiFi chính của bạn.</p>
        <div class="spinner"></div>
    </div>
</body>
</html>
    )rawhtml";

    server.send(200, "text/html", successHTML);

    delay(2000);
    ESP.restart();
}

void WebPortal::handleRoot() {
    server.send(200, "text/html", getPortalHTML());
}

String WebPortal::getPortalHTML() {
    String currentSSID = currentConfig.wifi_ssid;
    String currentHost = currentConfig.mqtt_host.isEmpty() ? "" : currentConfig.mqtt_host;
    String currentPort = String(currentConfig.mqtt_port > 0 ? currentConfig.mqtt_port : 1883);
    String currentUser = currentConfig.mqtt_user;
    String currentDeviceId = currentConfig.device_id;

    if (currentDeviceId.isEmpty()) {
        uint64_t chipid = ESP.getEfuseMac();
        currentDeviceId = "esp32_" + String((uint16_t)(chipid & 0xFFFF), HEX);
    }

    String html = R"rawhtml(
<!DOCTYPE html>
<html lang="vi">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>ESP32 Device Configuration</title>
    <style>
        :root {
            --bg-color: #0b1120;
            --card-bg: rgba(22, 30, 49, 0.85);
            --card-border: rgba(255, 255, 255, 0.08);
            --primary: #38bdf8;
            --primary-hover: #0284c7;
            --accent: #818cf8;
            --text-main: #f1f5f9;
            --text-muted: #94a3b8;
            --input-bg: rgba(15, 23, 42, 0.7);
            --input-border: rgba(148, 163, 184, 0.2);
            --input-focus: #38bdf8;
            --danger: #ef4444;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: radial-gradient(circle at 50% 0%, #1e1b4b 0%, #0b1120 70%);
            color: var(--text-main);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            padding: 24px 16px;
        }

        .container {
            width: 100%;
            max-width: 440px;
        }

        .header {
            text-align: center;
            margin-bottom: 24px;
        }

        .logo-badge {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            width: 54px;
            height: 54px;
            border-radius: 16px;
            background: linear-gradient(135deg, #38bdf8 0%, #6366f1 100%);
            box-shadow: 0 10px 25px rgba(56, 189, 248, 0.35);
            margin-bottom: 12px;
        }

        .logo-badge svg {
            width: 30px;
            height: 30px;
            fill: #ffffff;
        }

        .header h1 {
            font-size: 22px;
            font-weight: 700;
            letter-spacing: -0.5px;
            margin-bottom: 6px;
        }

        .header p {
            color: var(--text-muted);
            font-size: 13px;
        }

        .card {
            background: var(--card-bg);
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
            border: 1px solid var(--card-border);
            border-radius: 20px;
            padding: 24px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.4);
            margin-bottom: 20px;
        }

        .section-title {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 14px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.8px;
            color: var(--primary);
            margin-top: 18px;
            margin-bottom: 14px;
            padding-bottom: 6px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.05);
        }

        .section-title:first-of-type {
            margin-top: 0;
        }

        .form-group {
            margin-bottom: 14px;
        }

        label {
            display: block;
            font-size: 12px;
            font-weight: 500;
            color: var(--text-muted);
            margin-bottom: 6px;
        }

        .input-wrapper {
            position: relative;
            display: flex;
            align-items: center;
        }

        input, select {
            width: 100%;
            background: var(--input-bg);
            border: 1px solid var(--input-border);
            border-radius: 10px;
            padding: 11px 14px;
            font-size: 14px;
            color: var(--text-main);
            outline: none;
            transition: all 0.2s ease;
        }

        select {
            cursor: pointer;
            appearance: none;
            -webkit-appearance: none;
            background-image: url("data:image/svg+xml;utf8,<svg fill='%2394a3b8' height='20' viewBox='0 0 24 24' width='20' xmlns='http://www.w3.org/2000/svg'><path d='M7 10l5 5 5-5z'/></svg>");
            background-repeat: no-repeat;
            background-position: right 10px center;
        }

        input:focus, select:focus {
            border-color: var(--input-focus);
            box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.15);
        }

        .grid-2 {
            display: grid;
            grid-template-columns: 2fr 1fr;
            gap: 10px;
        }

        .scan-bar {
            display: flex;
            gap: 8px;
            margin-bottom: 10px;
        }

        .btn-scan {
            background: rgba(56, 189, 248, 0.15);
            color: var(--primary);
            border: 1px solid rgba(56, 189, 248, 0.3);
            border-radius: 8px;
            padding: 8px 12px;
            font-size: 12px;
            font-weight: 600;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            transition: all 0.2s;
            white-space: nowrap;
        }

        .btn-scan:hover {
            background: rgba(56, 189, 248, 0.25);
        }

        .btn-submit {
            width: 100%;
            background: linear-gradient(135deg, #38bdf8 0%, #4f46e5 100%);
            color: #ffffff;
            border: none;
            border-radius: 12px;
            padding: 13px;
            font-size: 15px;
            font-weight: 600;
            cursor: pointer;
            box-shadow: 0 10px 20px rgba(56, 189, 248, 0.3);
            transition: transform 0.15s, box-shadow 0.15s;
            margin-top: 10px;
        }

        .btn-submit:hover {
            transform: translateY(-1px);
            box-shadow: 0 12px 24px rgba(56, 189, 248, 0.4);
        }

        .btn-submit:active {
            transform: translateY(1px);
        }

        .footer-actions {
            text-align: center;
            margin-top: 12px;
        }

        .btn-reset {
            background: transparent;
            border: none;
            color: #f87171;
            font-size: 12px;
            cursor: pointer;
            text-decoration: underline;
            opacity: 0.8;
            transition: opacity 0.2s;
        }

        .btn-reset:hover {
            opacity: 1;
        }

        .tag-hint {
            font-size: 11px;
            color: #64748b;
            margin-top: 4px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo-badge">
                <svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>
            </div>
            <h1>Cài Đặt Thiết Bị ESP32</h1>
            <p>Cấu hình kết nối Mạng WiFi & MQTT Broker</p>
        </div>

        <div class="card">
            <form action="/save" method="POST">
                <!-- WiFi Section -->
                <div class="section-title">
                    <svg width="16" height="16" fill="currentColor" viewBox="0 0 24 24"><path d="M12 4C7.31 4 3.07 5.9 0 8.98L12 21 24 8.98A16.88 16.88 0 0 0 12 4zm0 2.9c3.78 0 7.22 1.48 9.77 3.89L12 18.66 2.23 10.79A14.9 14.9 0 0 1 12 6.9z"/></svg>
                    1. Cấu hình WiFi
                </div>

                <div class="form-group">
                    <div class="scan-bar">
                        <select id="wifi_select" onchange="selectWiFi(this.value)">
                            <option value="">-- Chọn mạng WiFi hoặc quét --</option>
                        </select>
                        <button type="button" class="btn-scan" onclick="scanNetworks()">
                            <span id="scan-icon">&#x21bb;</span> Quét WiFi
                        </button>
                    </div>
                </div>

                <div class="form-group">
                    <label for="wifi_ssid">Tên WiFi (SSID) *</label>
                    <input type="text" id="wifi_ssid" name="wifi_ssid" placeholder="Nhập tên mạng WiFi" value=")rawhtml" + currentSSID + R"rawhtml(" required>
                </div>

                <div class="form-group">
                    <label for="wifi_pass">Mật khẩu WiFi</label>
                    <input type="password" id="wifi_pass" name="wifi_pass" placeholder="Nhập mật khẩu WiFi (nếu có)">
                </div>

                <!-- MQTT Section -->
                <div class="section-title">
                    <svg width="16" height="16" fill="currentColor" viewBox="0 0 24 24"><path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM19 18H6c-2.21 0-4-1.79-4-4 0-2.05 1.53-3.76 3.56-3.97l1.07-.11.5-.95C8.08 7.14 9.94 6 12 6c2.62 0 4.88 1.86 5.39 4.43l.3 1.5 1.53.11c1.56.1 2.78 1.41 2.78 2.96 0 1.65-1.35 3-3 3z"/></svg>
                    2. Cấu hình MQTT
                </div>

                <div class="grid-2">
                    <div class="form-group">
                        <label for="mqtt_host">MQTT Broker (Host) *</label>
                        <input type="text" id="mqtt_host" name="mqtt_host" placeholder="broker.emqx.io / IP" value=")rawhtml" + currentHost + R"rawhtml(" required>
                    </div>
                    <div class="form-group">
                        <label for="mqtt_port">Port *</label>
                        <input type="number" id="mqtt_port" name="mqtt_port" value=")rawhtml" + currentPort + R"rawhtml(" required>
                    </div>
                </div>

                <div class="grid-2">
                    <div class="form-group">
                        <label for="mqtt_user">Username</label>
                        <input type="text" id="mqtt_user" name="mqtt_user" placeholder="Tùy chọn" value=")rawhtml" + currentUser + R"rawhtml(">
                    </div>
                    <div class="form-group">
                        <label for="mqtt_pass">Password</label>
                        <input type="password" id="mqtt_pass" name="mqtt_pass" placeholder="Tùy chọn">
                    </div>
                </div>

                <!-- Device ID Section -->
                <div class="section-title">
                    <svg width="16" height="16" fill="currentColor" viewBox="0 0 24 24"><path d="M4 6h16v12H4z M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2z"/></svg>
                    3. Device ID
                </div>

                <div class="form-group">
                    <label for="device_id">Device ID (Định danh thiết bị) *</label>
                    <input type="text" id="device_id" name="device_id" placeholder="Ví dụ: fish_feeder_01" value=")rawhtml" + currentDeviceId + R"rawhtml(" required>
                    <div class="tag-hint">Topic MQTT sẽ tự động gán theo ID: fish/<b>&lt;DeviceID&gt;</b>/status</div>
                </div>

                <button type="submit" class="btn-submit">Lưu Cấu Hình & Khởi Động Lại</button>
            </form>

            <div class="footer-actions">
                <button type="button" class="btn-reset" onclick="resetConfig()">Khôi phục cài đặt gốc (Xóa cấu hình)</button>
            </div>
        </div>
    </div>

    <script>
        function selectWiFi(ssid) {
            if (ssid) {
                document.getElementById('wifi_ssid').value = ssid;
                document.getElementById('wifi_pass').focus();
            }
        }

        function scanNetworks() {
            const btn = document.querySelector('.btn-scan');
            const select = document.getElementById('wifi_select');
            btn.innerHTML = '<span>Đang quét...</span>';
            btn.disabled = true;

            fetch('/scan')
                .then(r => r.json())
                .then(data => {
                    select.innerHTML = '<option value="">-- Chọn mạng WiFi đã quét --</option>';
                    if (data && data.length > 0) {
                        data.forEach(net => {
                            const opt = document.createElement('option');
                            opt.value = net.ssid;
                            const sec = net.secure ? '🔒' : '🔓';
                            opt.textContent = `${sec} ${net.ssid} (${net.rssi} dBm)`;
                            select.appendChild(opt);
                        });
                    } else {
                        const opt = document.createElement('option');
                        opt.value = "";
                        opt.textContent = "Không tìm thấy mạng nào";
                        select.appendChild(opt);
                    }
                })
                .catch(err => {
                    console.error(err);
                    alert('Lỗi khi quét WiFi!');
                })
                .finally(() => {
                    btn.innerHTML = '<span id="scan-icon">&#x21bb;</span> Quét WiFi';
                    btn.disabled = false;
                });
        }

        function resetConfig() {
            if (confirm('Bạn có chắc chắn muốn xóa toàn bộ cấu hình đã lưu và khởi động lại ESP32?')) {
                fetch('/reset', { method: 'POST' })
                    .then(() => {
                        alert('Đã xóa cấu hình! Thiết bị đang khởi động lại.');
                        location.reload();
                    });
            }
        }

        // Tự động quét WiFi khi vừa mở trang
        window.addEventListener('load', () => {
            scanNetworks();
        });
    </script>
</body>
</html>
    )rawhtml";

    return html;
}
