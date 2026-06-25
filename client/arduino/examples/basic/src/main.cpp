#include <Arduino.h>

#include "EspManagerClient.h"
#include "secrets.h"

static EspManagerConfig makeConfig() {
	EspManagerConfig c;
	c.wifiSSID = WIFI_SSID;
	c.wifiPassword = WIFI_PASS;
	c.host = ESPM_HOST;
	c.httpPort = 8080;
	c.mqttPort = 1883;
	c.claimToken = ESPM_CLAIM_TOKEN;
	c.firmwareVersion = "test-0.4.0";
	c.signingPublicKeyHex = ESPM_SIGNING_PUBKEY;
	c.heartbeatIntervalMs = 15000;
	return c;
}

EspManagerClient espm(makeConfig());

void setup() {
	Serial.begin(115200);
	delay(500);
	espm.begin();
	Serial.printf("espmanager: device %s starting\n", espm.deviceID().c_str());
}

void loop() {
	espm.loop();
	delay(10);
}
