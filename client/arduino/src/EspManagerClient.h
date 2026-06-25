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
	const char *signingPublicKeyHex;
	uint32_t heartbeatIntervalMs;
};

class EspManagerClient {
public:
	explicit EspManagerClient(const EspManagerConfig &cfg);

	void begin();
	void loop();

	const String &deviceID() const { return deviceID_; }

private:
	String computeDeviceID() const;
	bool claimCredentials();
	bool persistPassword(const String &password);
	bool connectMQTT();
	void publishHeartbeat();
	void onMessage(const char *topic, const uint8_t *payload, unsigned int length);
	void applyOTA(const String &targetVersion, const String &url, const String &sha256Hex, const String &signatureHex);
	void confirmOTA();
	void reportOTA(const char *status);

	String topic(const char *suffix) const;
	static bool hexToBytes(const String &hex, uint8_t *out, size_t outLen);

	EspManagerConfig cfg_;
	String deviceID_;
	String password_;
	String otaTarget_;
	WiFiClient net_;
	PubSubClient mqtt_;
	Preferences prefs_;
	uint32_t lastHeartbeat_;
	uint32_t lastReconnectAttempt_;
	uint32_t lastClaimAttempt_;
	uint32_t lastWiFiAttempt_;
};
