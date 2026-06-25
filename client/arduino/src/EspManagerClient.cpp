#include "EspManagerClient.h"

#include <ArduinoJson.h>
#include <HTTPClient.h>
#include <WiFi.h>

namespace {
constexpr char prefsNamespace[] = "espm";
constexpr char prefsPasswordKey[] = "mqttpass";
constexpr uint32_t reconnectIntervalMs = 5000;
constexpr uint32_t claimRetryIntervalMs = 15000;
constexpr uint32_t wifiRetryIntervalMs = 30000;
}

EspManagerClient::EspManagerClient(const EspManagerConfig &cfg)
	: cfg_(cfg), mqtt_(net_), lastHeartbeat_(0), lastReconnectAttempt_(0),
	  lastClaimAttempt_(0), lastWiFiAttempt_(0) {}

void EspManagerClient::begin() {
	WiFi.mode(WIFI_STA);
	deviceID_ = computeDeviceID();

	prefs_.begin(prefsNamespace, true);
	password_ = prefs_.getString(prefsPasswordKey, "");
	prefs_.end();

	mqtt_.setServer(cfg_.host, cfg_.mqttPort);
	mqtt_.setBufferSize(1024);
	mqtt_.setCallback([this](char *topic, uint8_t *payload, unsigned int length) {
		onMessage(topic, payload, length);
	});

	WiFi.setAutoReconnect(true);
	WiFi.begin(cfg_.wifiSSID, cfg_.wifiPassword);
	lastWiFiAttempt_ = millis();
}

void EspManagerClient::loop() {
	uint32_t now = millis();

	if (WiFi.status() != WL_CONNECTED) {
		if (now - lastWiFiAttempt_ >= wifiRetryIntervalMs) {
			lastWiFiAttempt_ = now;
			WiFi.begin(cfg_.wifiSSID, cfg_.wifiPassword);
		}
		return;
	}

	if (password_.length() == 0) {
		if (now - lastClaimAttempt_ >= claimRetryIntervalMs) {
			lastClaimAttempt_ = now;
			claimCredentials();
		}
		return;
	}

	if (!mqtt_.connected()) {
		if (now - lastReconnectAttempt_ >= reconnectIntervalMs) {
			lastReconnectAttempt_ = now;
			connectMQTT();
		}
		return;
	}

	mqtt_.loop();
	if (now - lastHeartbeat_ >= cfg_.heartbeatIntervalMs) {
		publishHeartbeat();
	}
}

String EspManagerClient::computeDeviceID() const {
	uint8_t mac[6];
	WiFi.macAddress(mac);
	char buf[13];
	snprintf(buf, sizeof(buf), "%02x%02x%02x%02x%02x%02x", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
	return String(buf);
}

bool EspManagerClient::claimCredentials() {
	if (cfg_.claimToken == nullptr || strlen(cfg_.claimToken) == 0) {
		return false;
	}

	HTTPClient http;
	String url = String("http://") + cfg_.host + ":" + cfg_.httpPort + "/v1/claim";
	if (!http.begin(net_, url)) {
		return false;
	}
	http.addHeader("Content-Type", "application/json");

	JsonDocument req;
	req["device_id"] = deviceID_;
	req["token"] = cfg_.claimToken;
	String body;
	serializeJson(req, body);

	int status = http.POST(body);
	if (status != HTTP_CODE_OK) {
		http.end();
		return false;
	}

	JsonDocument res;
	DeserializationError err = deserializeJson(res, http.getString());
	http.end();
	if (err) {
		return false;
	}

	String password = res["password"].as<String>();
	if (password.length() == 0 || !persistPassword(password)) {
		return false;
	}
	password_ = password;
	return true;
}

bool EspManagerClient::persistPassword(const String &password) {
	if (!prefs_.begin(prefsNamespace, false)) {
		return false;
	}
	size_t written = prefs_.putString(prefsPasswordKey, password);
	prefs_.end();
	return written == password.length();
}

bool EspManagerClient::connectMQTT() {
	String availability = topic("availability");
	bool ok = mqtt_.connect(
		deviceID_.c_str(),
		deviceID_.c_str(),
		password_.c_str(),
		availability.c_str(),
		1,
		true,
		"offline");
	if (!ok) {
		return false;
	}

	mqtt_.publish(availability.c_str(), "online", true);
	mqtt_.subscribe(topic("cmd/ota").c_str(), 1);
	publishHeartbeat();
	return true;
}

void EspManagerClient::publishHeartbeat() {
	JsonDocument doc;
	doc["version"] = cfg_.firmwareVersion;
	doc["rssi"] = WiFi.RSSI();
	doc["uptime"] = millis() / 1000;
	String payload;
	serializeJson(doc, payload);
	mqtt_.publish(topic("state").c_str(), payload.c_str());
	lastHeartbeat_ = millis();
}

void EspManagerClient::onMessage(const char *t, const uint8_t *payload, unsigned int length) {
	if (topic("cmd/ota") != t) {
		return;
	}

	JsonDocument cmd;
	if (deserializeJson(cmd, payload, length)) {
		return;
	}

	JsonDocument status;
	status["status"] = "updating";
	status["version"] = cmd["version"];
	String out;
	serializeJson(status, out);
	mqtt_.publish(topic("ota/status").c_str(), out.c_str());
}

String EspManagerClient::topic(const char *suffix) const {
	return String("espmanager/") + deviceID_ + "/" + suffix;
}
