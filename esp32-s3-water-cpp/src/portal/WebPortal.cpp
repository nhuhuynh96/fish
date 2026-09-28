#include "WebPortal.h"
#include <ArduinoJson.h>

WebPortal webPortal;

static const byte DNS_PORT = 53;
static const IPAddress AP_IP(192, 168, 4, 1);
static const IPAddress AP_NETMASK(255, 255, 255, 0);

static String htmlEscape(const String &s) {
    String out;
    out.reserve(s.length() + 8);
    for (size_t i = 0; i < s.length(); i++) {
        char c = s[i];
        switch (c) {
            case '&': out += "&amp;"; break;
            case '<': out += "&lt;"; break;
            case '>': out += "&gt;"; break;
            case '"': out += "&quot;"; break;
            case '\'': out += "&#39;"; break;
            default: out += c;
        }
    }
    return out;
}

WebPortal::WebPortal() : server(80) {}

void WebPortal::startPortal() {
    portalRunning = true;

    uint64_t chipid = ESP.getEfuseMac();
    char apName[32];
    snprintf(apName, sizeof(apName), "FishWater-Setup-%04X", (uint16_t)(chipid >> 32));
    apSSID = String(apName);

    Serial.println("\n[Portal] =========================================");
    Serial.printf("[Portal] Phát WiFi cấu hình: %s (không mật khẩu)\n", apSSID.c_str());
    Serial.println("[Portal] Mở trình duyệt: http://192.168.4.1");
    Serial.println("[Portal] =========================================\n");

    WiFi.disconnect(true);
    delay(100);
    // AP + STA để vừa phát AP vừa quét được WiFi xung quanh
    WiFi.mode(WIFI_AP_STA);
    WiFi.softAPConfig(AP_IP, AP_IP, AP_NETMASK);
    WiFi.softAP(apSSID.c_str());

    dnsServer.setErrorReplyCode(DNSReplyCode::NoError);
    dnsServer.start(DNS_PORT, "*", AP_IP);

    setupRoutes();
    server.begin();
    Serial.println("[Portal] Web Server & DNS Server đã sẵn sàng");
}

void WebPortal::handlePortal() {
    if (!portalRunning) return;
    dnsServer.processNextRequest();
    server.handleClient();
}

void WebPortal::setupRoutes() {
    server.on("/", HTTP_GET, [this]() { handleRoot(); });
    server.on("/save", HTTP_POST, [this]() { handleSave(); });
    server.on("/scan", HTTP_GET, [this]() { handleScan(); });
    server.on("/reset", HTTP_POST, [this]() { handleReset(); });

    // Endpoint dò Captive Portal của Android / iOS / Windows
    server.on("/generate_204", HTTP_GET, [this]() { handleRoot(); });
    server.on("/gen_204", HTTP_GET, [this]() { handleRoot(); });
    server.on("/hotspot-detect.html", HTTP_GET, [this]() { handleRoot(); });
    server.on("/canonical.html", HTTP_GET, [this]() { handleRoot(); });
    server.on("/connecttest.txt", HTTP_GET, [this]() { handleRoot(); });
    server.on("/ncsi.txt", HTTP_GET, [this]() { handleRoot(); });

    server.onNotFound([this]() { handleNotFound(); });
}

void WebPortal::handleNotFound() {
    server.sendHeader("Location", String("http://") + AP_IP.toString() + "/", true);
    server.send(302, "text/plain", "");
}

void WebPortal::handleScan() {
    Serial.println("[Portal] Đang quét WiFi xung quanh...");
    int n = WiFi.scanNetworks(false, false);

    JsonDocument doc;
    JsonArray arr = doc.to<JsonArray>();
    for (int i = 0; i < n; ++i) {
        String ssid = WiFi.SSID(i);
        if (ssid.isEmpty()) continue;
        bool dup = false;
        for (JsonObject o : arr) {
            if (ssid == o["ssid"].as<const char *>()) {
                dup = true;
                break;
            }
        }
        if (dup) continue;
        JsonObject o = arr.add<JsonObject>();
        o["ssid"] = ssid;
        o["rssi"] = WiFi.RSSI(i);
        o["secure"] = WiFi.encryptionType(i) != WIFI_AUTH_OPEN;
    }
    WiFi.scanDelete();
    Serial.printf("[Portal] Tìm thấy %d mạng\n", (int)arr.size());

    String json;
    serializeJson(doc, json);
    server.send(200, "application/json", json);
}

void WebPortal::handleReset() {
    configManager.clearConfig();
    server.send(200, "application/json", "{\"status\":\"ok\"}");
    delay(1000);
    ESP.restart();
}

void WebPortal::handleSave() {
    DeviceConfig cfg;
    cfg.wifi_ssid = server.arg("wifi_ssid");
    cfg.wifi_pass = server.arg("wifi_pass");
    cfg.mqtt_host = server.arg("mqtt_host");
    long port = server.arg("mqtt_port").toInt();
    cfg.mqtt_port = port > 0 && port < 65536 ? (uint16_t)port : 1883;
    cfg.mqtt_user = server.arg("mqtt_user");
    cfg.mqtt_pass = server.arg("mqtt_pass");
    cfg.device_id = server.arg("device_id");

    cfg.wifi_ssid.trim();
    cfg.mqtt_host.trim();
    cfg.mqtt_user.trim();
    cfg.device_id.trim();

    if (cfg.wifi_ssid.isEmpty() || cfg.mqtt_host.isEmpty() || cfg.device_id.isEmpty()) {
        server.send(400, "text/html; charset=utf-8",
                    "<h3 style='color:red'>Thiếu WiFi, MQTT Host hoặc Device ID</h3><a href='/'>Quay lại</a>");
        return;
    }
    cfg.topic = ConfigManager::normalizeTopic(server.arg("topic"), cfg.device_id);

    configManager.saveConfig(cfg);

    String html = F("<!DOCTYPE html><html lang='vi'><head><meta charset='UTF-8'>"
                    "<meta name='viewport' content='width=device-width,initial-scale=1'>"
                    "<title>Đã lưu</title><style>body{font-family:sans-serif;background:#0f172a;color:#f1f5f9;"
                    "display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}"
                    ".c{background:#1e293b;padding:28px;border-radius:16px;max-width:420px;text-align:center}"
                    "code{color:#38bdf8}</style></head><body><div class='c'><h2>Đã lưu cấu hình</h2>");
    html += "<p>Thiết bị khởi động lại và kết nối WiFi <b>" + htmlEscape(cfg.wifi_ssid) + "</b>.</p>";
    html += "<p>Subscribe: <code>" + htmlEscape(cfg.topic) + "/command</code><br>";
    html += "Publish: <code>" + htmlEscape(cfg.topic) + "/status|event|telemetry|log</code></p>";
    html += F("</div></body></html>");
    server.send(200, "text/html; charset=utf-8", html);

    delay(2000);
    ESP.restart();
}

void WebPortal::handleRoot() {
    server.send(200, "text/html; charset=utf-8", getPortalHTML());
}

static const char PORTAL_HTML[] PROGMEM = R"rawhtml(<!DOCTYPE html>
<html lang="vi"><head><meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1">
<title>Fish Water Setup</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:radial-gradient(circle at 50% 0%,#1e1b4b 0%,#0b1120 70%);color:#f1f5f9;min-height:100vh;display:flex;justify-content:center;padding:24px 16px}
.w{width:100%;max-width:440px}
h1{font-size:21px;text-align:center;margin-bottom:4px}
.sub{color:#94a3b8;font-size:13px;text-align:center;margin-bottom:18px}
.card{background:rgba(22,30,49,.85);border:1px solid rgba(255,255,255,.08);border-radius:18px;padding:22px}
.t{font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:.8px;color:#38bdf8;margin:18px 0 12px;padding-bottom:6px;border-bottom:1px solid rgba(255,255,255,.06)}
.t:first-child{margin-top:0}
.g{margin-bottom:12px}
label{display:block;font-size:12px;color:#94a3b8;margin-bottom:5px}
input,select{width:100%;background:rgba(15,23,42,.7);border:1px solid rgba(148,163,184,.2);border-radius:10px;padding:10px 12px;font-size:14px;color:#f1f5f9;outline:none}
input:focus,select:focus{border-color:#38bdf8}
.row{display:flex;gap:8px}
.g2{display:grid;grid-template-columns:2fr 1fr;gap:10px}
.bs{background:rgba(56,189,248,.15);color:#38bdf8;border:1px solid rgba(56,189,248,.3);border-radius:8px;padding:8px 12px;font-size:12px;font-weight:600;white-space:nowrap;cursor:pointer}
.bt{width:100%;background:linear-gradient(135deg,#38bdf8,#4f46e5);color:#fff;border:0;border-radius:12px;padding:13px;font-size:15px;font-weight:600;margin-top:8px;cursor:pointer}
.h{font-size:11px;color:#64748b;margin-top:4px}
.h code{color:#38bdf8}
.rs{display:block;margin:12px auto 0;background:none;border:0;color:#f87171;font-size:12px;text-decoration:underline;cursor:pointer}
</style></head><body><div class="w">
<h1>Board Bơm / Xả Nước</h1>
<div class="sub">Cấu hình WiFi, MQTT, Device ID và Topic</div>
<div class="card"><form action="/save" method="POST">
<div class="t">1. WiFi</div>
<div class="g"><div class="row">
<select id="wsel" onchange="if(this.value){ssid.value=this.value;wifi_pass.focus()}"><option value="">-- Đang quét WiFi... --</option></select>
<button type="button" class="bs" id="bscan" onclick="scan()">&#x21bb; Quét</button>
</div></div>
<div class="g"><label>Tên WiFi (SSID) *</label><input id="ssid" name="wifi_ssid" value="{{SSID}}" required></div>
<div class="g"><label>Mật khẩu WiFi</label><input type="password" id="wifi_pass" name="wifi_pass" placeholder="Bỏ trống nếu mạng mở"></div>
<div class="t">2. MQTT</div>
<div class="g2">
<div class="g"><label>MQTT Host *</label><input name="mqtt_host" value="{{HOST}}" placeholder="192.168.1.10" required></div>
<div class="g"><label>Port *</label><input type="number" name="mqtt_port" value="{{PORT}}" required></div>
</div>
<div class="g2">
<div class="g"><label>Username</label><input name="mqtt_user" value="{{USER}}" placeholder="Tùy chọn"></div>
<div class="g"><label>Password</label><input type="password" name="mqtt_pass" placeholder="Tùy chọn"></div>
</div>
<div class="t">3. Thiết bị</div>
<div class="g"><label>Device ID *</label><input id="did" name="device_id" value="{{DEVICE_ID}}" required oninput="hint()">
<div class="h">Gửi kèm trong mọi bản tin publish (<code>"device_id"</code>)</div></div>
<div class="g"><label>Topic *</label><input id="topic" name="topic" value="{{TOPIC}}" placeholder="fish/ten_thiet_bi" required oninput="hint()">
<div class="h" id="th"></div></div>
<button type="submit" class="bt">Lưu &amp; Khởi động lại</button>
</form>
<button type="button" class="rs" onclick="if(confirm('Xóa cấu hình và khởi động lại?'))fetch('/reset',{method:'POST'}).then(()=>alert('Đã xóa, thiết bị đang khởi động lại.'))">Xóa cấu hình</button>
</div></div>
<script>
function hint(){var t=(topic.value||('fish/'+did.value)).replace(/\/+$/,'').replace(/\/command$/,'');
th.innerHTML='Subscribe: <code>'+t+'/command</code><br>Publish: <code>'+t+'/status</code>, <code>/event</code>, <code>/telemetry</code>, <code>/log</code>';}
function scan(){var b=bscan;b.disabled=true;b.textContent='Đang quét...';
fetch('/scan').then(r=>r.json()).then(d=>{wsel.innerHTML='';var o=document.createElement('option');o.value='';
o.textContent=d.length?'-- Chọn WiFi ('+d.length+' mạng) --':'Không tìm thấy mạng nào';wsel.appendChild(o);
d.sort((a,b)=>b.rssi-a.rssi).forEach(n=>{var e=document.createElement('option');e.value=n.ssid;
e.textContent=(n.secure?'\u{1F512} ':'\u{1F513} ')+n.ssid+' ('+n.rssi+' dBm)';if(n.ssid===ssid.value)e.selected=true;wsel.appendChild(e);});})
.catch(()=>alert('Lỗi khi quét WiFi')).finally(()=>{b.disabled=false;b.innerHTML='&#x21bb; Quét';});}
window.addEventListener('load',()=>{hint();scan();});
</script></body></html>)rawhtml";

String WebPortal::getPortalHTML() {
    String html = FPSTR(PORTAL_HTML);
    html.replace("{{SSID}}", htmlEscape(currentConfig.wifi_ssid));
    html.replace("{{HOST}}", htmlEscape(currentConfig.mqtt_host));
    html.replace("{{PORT}}", String(currentConfig.mqtt_port > 0 ? currentConfig.mqtt_port : 1883));
    html.replace("{{USER}}", htmlEscape(currentConfig.mqtt_user));
    html.replace("{{DEVICE_ID}}", htmlEscape(currentConfig.device_id));
    html.replace("{{TOPIC}}", htmlEscape(currentConfig.topic));
    return html;
}
