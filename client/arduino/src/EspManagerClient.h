#pragma once

#include <Arduino.h>
#include <WiFiClient.h>
#include <PubSubClient.h>
#include <Preferences.h>

struct EspManagerConfig {
	const char *wifiSSID;
	const char *wifiPassword;
	const char *host;
	uint16_t httpPort;
	uint16_t mqttPort;
	const char *claimToken;
	const char *firmwareVersion;
	uint32_t heartbeatIntervalMs;
};

class EspManagerClient {
public:
	explicit EspManagerClient(const EspManagerConfig &cfg);

	bool begin();
	void loop();

	const String &deviceID() const { return deviceID_; }

private:
	bool connectWiFi();
	bool ensureCredentials();
	bool claimCredentials();
	bool connectMQTT();
	void publishHeartbeat();
	void onMessage(const char *topic, const uint8_t *payload, unsigned int length);

	String topic(const char *suffix) const;

	EspManagerConfig cfg_;
	String deviceID_;
	String password_;
	WiFiClient net_;
	PubSubClient mqtt_;
	Preferences prefs_;
	uint32_t lastHeartbeat_;
	uint32_t lastReconnectAttempt_;
};
