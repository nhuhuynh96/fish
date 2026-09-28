#include "WebPortal.h"

WebPortal webPortal;

const byte DNS_PORT = 53;
const IPAddress AP_IP(192, 168, 4, 1);
const IPAddress AP_NETMASK(255, 255, 255, 0);

WebPortal::WebPortal() : server(80) {}

void WebPortal::startPortal() {
    portalRunning = true;

    // Sinh tên AP duy nhất dựa vào MAC của ESP32-S3
    uint64_t chipid = ESP.getEfuseMac();
    char apName[32];
    snprintf(apName, sizeof(apName), "ESP32S3-Sensors-%04X", (uint16_t)(chipid & 0xFFFF));
    apSSID = String(apName);

    Serial.println("\n==========================================");
    Serial.printf("[Portal S3] Khởi động AP Mode: %s\n", apSSID.c_str());
    Serial.println("[Portal S3] IP Cấu hình: http://192.168.4.1");
    Serial.println("==========================================\n");

    WiFi.mode(WIFI_AP_STA);
    WiFi.softAPConfig(AP_IP, AP_IP, AP_NETMASK);
    WiFi.softAP(apSSID.c_str());

    // Khởi động DNS Server cho Captive Portal
    dnsServer.setErrorReplyCode(DNSReplyCode::NoError);
    dnsServer.start(DNS_PORT, "*", AP_IP);

    setupRoutes();
    server.begin();
    Serial.println("[Portal S3] Web Server & DNS Server đã sẵn sàng!");
}

void WebPortal::stopPortal() {
    if (portalRunning) {
        dnsServer.stop();
        server.stop();
        WiFi.softAPdisconnect(true);
        portalRunning = false;
        Serial.println("[Portal S3] Đã tắt AP Mode & Web Server");
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
    server.on("/", HTTP_GET, [this]() {
        if (isCaptivePortal()) {
            server.sendHeader("Location", String("http://") + "192.168.4.1/", true);
            server.send(302, "text/plain", "");
            return;
        }
        handleRoot();
    });

    server.on("/save", HTTP_POST, [this]() { handleSave(); });
    server.on("/scan", HTTP_GET, [this]() { handleScan(); });
    server.on("/reset", HTTP_POST, [this]() { handleReset(); });

    // Các endpoint captive portal chuẩn của Apple, Android, Windows
    server.on("/hotspot-detect.html", [this]() {
        server.sendHeader("Location", "http://192.168.4.1/", true);
        server.send(302, "text/plain", "");
    });
    server.on("/canonical.html", [this]() { handleRoot(); });
    server.on("/generate_204", [this]() {
        server.sendHeader("Location", "http://192.168.4.1/", true);
        server.send(302, "text/plain", "");
    });

    server.onNotFound([this]() { handleNotFound(); });
}

void WebPortal::handleRoot() {
    server.send(200, "text/html", getPortalHTML());
}

void WebPortal::handleScan() {
    int n = WiFi.scanNetworks();
    String json = "[";
    for (int i = 0; i < n; ++i) {
        if (i > 0) json += ",";
        json += "{\"ssid\":\"" + WiFi.SSID(i) + "\",\"rssi\":" + String(WiFi.RSSI(i)) + "}";
    }
    json += "]";
    server.send(200, "application/json", json);
}

void WebPortal::handleSave() {
    if (server.hasArg("wifi_ssid")) currentConfig.wifi_ssid = server.arg("wifi_ssid");
    if (server.hasArg("wifi_pass")) currentConfig.wifi_pass = server.arg("wifi_pass");
    if (server.hasArg("mqtt_host")) currentConfig.mqtt_host = server.arg("mqtt_host");
    if (server.hasArg("mqtt_port")) currentConfig.mqtt_port = server.arg("mqtt_port").toInt();
    if (server.hasArg("mqtt_user")) currentConfig.mqtt_user = server.arg("mqtt_user");
    if (server.hasArg("mqtt_pass")) currentConfig.mqtt_pass = server.arg("mqtt_pass");
    if (server.hasArg("device_id")) currentConfig.device_id = server.arg("device_id");

    configManager.saveConfig(currentConfig);

    String successHtml = "<!DOCTYPE html><html><head><meta charset='UTF-8'><meta name='viewport' content='width=device-width,initial-scale=1'>"
                         "<style>body{background:#0f172a;color:#fff;display:flex;justify-content:center;align-items:center;min-height:100vh;font-family:sans-serif;text-align:center;}"
                         ".card{background:#1e293b;padding:32px;border-radius:16px;border:1px solid #334155;max-width:400px;}</style></head>"
                         "<body><div class='card'><h2 style='color:#4ade80'>✓ Đã lưu cấu hình!</h2><p style='color:#94a3b8'>ESP32-S3 đang khởi động lại để kết nối WiFi...</p></div></body></html>";
    server.send(200, "text/html", successHtml);

    delay(2000);
    ESP.restart();
}

void WebPortal::handleReset() {
    configManager.clearConfig();
    server.send(200, "text/plain", "Đã xóa toàn bộ cấu hình. ESP32-S3 đang khởi động lại...");
    delay(1000);
    ESP.restart();
}

void WebPortal::handleNotFound() {
    if (isCaptivePortal()) {
        server.sendHeader("Location", "http://192.168.4.1/", true);
        server.send(302, "text/plain", "");
        return;
    }
    server.send(404, "text/plain", "404 Not Found");
}

String WebPortal::getPortalHTML() {
    String html = "<!DOCTYPE html><html lang='vi'><head><meta charset='UTF-8'><meta name='viewport' content='width=device-width,initial-scale=1.0'>"
                  "<title>Cấu hình ESP32-S3 Sensors</title>"
                  "<style>"
                  ":root{--bg:#0f172a;--card:#1e293b;--text:#f8fafc;--muted:#94a3b8;--border:#334155;--primary:#0284c7;--primary-hover:#0369a1;}"
                  "*{box-sizing:border-box;margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;}"
                  "body{background:var(--bg);color:var(--text);display:flex;justify-content:center;align-items:center;min-height:100vh;padding:20px;}"
                  ".card{background:var(--card);border:1px solid var(--border);border-radius:16px;padding:28px;width:100%;max-width:440px;box-shadow:0 20px 25px -5px rgba(0,0,0,0.5);}"
                  ".header{text-align:center;margin-bottom:20px;}"
                  ".header h1{font-size:20px;color:#38bdf8;margin-bottom:4px;}"
                  ".header p{font-size:13px;color:var(--muted);}"
                  ".badge{display:inline-block;background:rgba(56,189,248,0.15);color:#38bdf8;font-size:12px;font-weight:600;padding:4px 10px;border-radius:9999px;margin-top:8px;border:1px solid rgba(56,189,248,0.3);}"
                  ".section-title{font-size:13px;font-weight:600;text-transform:uppercase;color:#38bdf8;margin:18px 0 10px;border-bottom:1px solid var(--border);padding-bottom:4px;}"
                  ".form-group{margin-bottom:14px;}"
                  "label{display:block;font-size:13px;color:var(--muted);margin-bottom:6px;}"
                  "input{width:100%;padding:10px 14px;background:#0f172a;border:1px solid var(--border);border-radius:8px;color:#fff;font-size:14px;outline:none;}"
                  "input:focus{border-color:var(--primary);}"
                  ".btn{width:100%;padding:12px;background:var(--primary);color:#fff;border:none;border-radius:8px;font-size:14px;font-weight:600;cursor:pointer;margin-top:16px;}"
                  ".btn:hover{background:var(--primary-hover);}"
                  "</style></head><body>"
                  "<div class='card'>"
                  "<div class='header'><h1>ESP32-S3 SENSORS</h1><p>Module Quan Trắc Nước Thông Minh</p><div class='badge'>AP: " + apSSID + "</div></div>"
                  "<form action='/save' method='POST'>"
                  "<div class='section-title'>1. Kết nối WiFi</div>"
                  "<div class='form-group'><label>Tên WiFi (SSID)</label><input type='text' name='wifi_ssid' value='" + currentConfig.wifi_ssid + "' required placeholder='WiFi 2.4GHz'></div>"
                  "<div class='form-group'><label>Mật khẩu WiFi</label><input type='password' name='wifi_pass' value='" + currentConfig.wifi_pass + "' placeholder='Mật khẩu'></div>"
                  "<div class='section-title'>2. Cấu hình MQTT Broker</div>"
                  "<div class='form-group'><label>Địa chỉ Broker (IP / Host)</label><input type='text' name='mqtt_host' value='" + currentConfig.mqtt_host + "' required placeholder='192.168.1.xxx'></div>"
                  "<div class='form-group'><label>Port</label><input type='number' name='mqtt_port' value='" + String(currentConfig.mqtt_port) + "' required></div>"
                  "<div class='form-group'><label>Username (Tùy chọn)</label><input type='text' name='mqtt_user' value='" + currentConfig.mqtt_user + "'></div>"
                  "<div class='form-group'><label>Password (Tùy chọn)</label><input type='password' name='mqtt_pass' value='" + currentConfig.mqtt_pass + "'></div>"
                  "<div class='form-group'><label>Device ID</label><input type='text' name='device_id' value='" + currentConfig.device_id + "' required></div>"
                  "<button type='submit' class='btn'>Lưu cấu hình & Kết nối</button>"
                  "</form></div></body></html>";
    return html;
}
