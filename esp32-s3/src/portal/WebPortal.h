#ifndef S3_WEB_PORTAL_H
#define S3_WEB_PORTAL_H

#include <Arduino.h>
#include <WiFi.h>
#include <WebServer.h>
#include <DNSServer.h>
#include "../config/ConfigManager.h"

class WebPortal {
public:
    WebPortal();
    void startPortal();
    void stopPortal();
    void handlePortal();
    bool isRunning() const { return portalRunning; }

private:
    WebServer server;
    DNSServer dnsServer;
    bool portalRunning = false;
    String apSSID;

    void setupRoutes();
    void handleRoot();
    void handleSave();
    void handleScan();
    void handleReset();
    void handleNotFound();

    bool isCaptivePortal();
    String getPortalHTML();
};

extern WebPortal webPortal;

#endif // S3_WEB_PORTAL_H
