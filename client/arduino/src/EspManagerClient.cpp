#include "EspManagerClient.h"

#include <ArduinoJson.h>
#include <Ed25519.h>
#include <HTTPClient.h>
#include <SHA256.h>
#include <Update.h>
#include <WiFi.h>

namespace {
constexpr char prefsNamespace[] = "espm";
constexpr char prefsPasswordKey[] = "mqttpass";
constexpr char prefsOtaTargetKey[] = "otatarget";
constexpr char prefsOtaSeqKey[] = "otaseq";
constexpr char prefsOtaPendSeqKey[] = "otapendseq";
constexpr uint32_t reconnectIntervalMs = 5000;
constexpr uint32_t claimRetryIntervalMs = 15000;
constexpr uint32_t wifiRetryIntervalMs = 30000;
constexpr uint32_t otaIdleTimeoutMs = 15000;
}

EspManagerClient::EspManagerClient(const EspManagerConfig &cfg)
	: cfg_(cfg), otaPending_(false), otaSeq_(0), otaFloor_(0), otaPendSeq_(0), mqtt_(net_),
	  lastHeartbeat_(0), lastReconnectAttempt_(0), lastClaimAttempt_(0), lastWiFiAttempt_(0) {}

void EspManagerClient::begin() {
	WiFi.mode(WIFI_STA);
	deviceID_ = computeDeviceID();

	prefs_.begin(prefsNamespace, true);
	password_ = prefs_.getString(prefsPasswordKey, "");
	otaTarget_ = prefs_.getString(prefsOtaTargetKey, "");
	otaFloor_ = prefs_.getULong64(prefsOtaSeqKey, 0);
	otaPendSeq_ = prefs_.getULong64(prefsOtaPendSeqKey, 0);
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
	flushPendingStatus();

	if (otaPending_) {
		otaPending_ = false;
		applyOTA();
		return;
	}

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
	confirmOTA();
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

void EspManagerClient::confirmOTA() {
	if (otaTarget_.length() == 0) {
		return;
	}
	pendingStatus_ = otaTarget_ == cfg_.firmwareVersion ? "ok" : "failed";
}

void EspManagerClient::flushPendingStatus() {
	if (pendingStatus_.length() == 0) {
		return;
	}
	bool confirmed = pendingStatus_ == "ok";
	if (!reportOTA(pendingStatus_.c_str())) {
		return;
	}
	pendingStatus_ = "";
	if (otaTarget_.length() > 0) {
		prefs_.begin(prefsNamespace, false);
		if (confirmed) {
			prefs_.putULong64(prefsOtaSeqKey, otaPendSeq_);
		}
		prefs_.remove(prefsOtaTargetKey);
		prefs_.remove(prefsOtaPendSeqKey);
		prefs_.end();
		if (confirmed) {
			otaFloor_ = otaPendSeq_;
		}
		otaTarget_ = "";
		otaPendSeq_ = 0;
	}
}

void EspManagerClient::onMessage(const char *t, const uint8_t *payload, unsigned int length) {
	if (topic("cmd/ota") != t) {
		return;
	}

	JsonDocument cmd;
	if (deserializeJson(cmd, payload, length)) {
		return;
	}

	String version = cmd["version"].as<String>();
	String url = cmd["url"].as<String>();
	uint64_t sequence = cmd["sequence"].as<uint64_t>();
	if (version.length() == 0 || url.length() == 0 || version == cfg_.firmwareVersion) {
		return;
	}
	if (sequence <= otaFloor_) {
		pendingStatus_ = "failed";
		return;
	}

	otaVersion_ = version;
	otaURL_ = url;
	otaSha_ = cmd["sha256"].as<String>();
	otaSig_ = cmd["signature"].as<String>();
	otaSeq_ = sequence;
	otaPending_ = true;
}

void EspManagerClient::applyOTA() {
	uint8_t expectedSha[32];
	uint8_t signature[64];
	uint8_t publicKey[32];
	if (!hexToBytes(otaSha_, expectedSha, sizeof(expectedSha)) ||
		!hexToBytes(otaSig_, signature, sizeof(signature)) ||
		!hexToBytes(String(cfg_.signingPublicKeyHex), publicKey, sizeof(publicKey)) ||
		isAllZero(publicKey, sizeof(publicKey))) {
		pendingStatus_ = "failed";
		return;
	}

	reportOTA("updating");

	HTTPClient http;
	if (!http.begin(net_, otaURL_)) {
		pendingStatus_ = "failed";
		return;
	}
	if (http.GET() != HTTP_CODE_OK) {
		http.end();
		pendingStatus_ = "failed";
		return;
	}

	int contentLen = http.getSize();
	if (!Update.begin(contentLen > 0 ? (size_t)contentLen : UPDATE_SIZE_UNKNOWN)) {
		http.end();
		pendingStatus_ = "failed";
		return;
	}

	SHA256 sha;
	sha.reset();
	WiFiClient *stream = http.getStreamPtr();
	uint8_t buf[1024];
	size_t total = 0;
	bool ok = true;
	uint32_t lastProgress = millis();

	while (http.connected() && (contentLen < 0 || total < (size_t)contentLen)) {
		size_t avail = stream->available();
		if (avail == 0) {
			if (millis() - lastProgress > otaIdleTimeoutMs) {
				ok = false;
				break;
			}
			delay(1);
			continue;
		}
		lastProgress = millis();
		int n = stream->readBytes(buf, avail < sizeof(buf) ? avail : sizeof(buf));
		if (n <= 0) {
			continue;
		}
		if (Update.write(buf, n) != (size_t)n) {
			ok = false;
			break;
		}
		sha.update(buf, n);
		total += n;
	}
	http.end();

	if (!ok || (contentLen > 0 && total != (size_t)contentLen)) {
		Update.abort();
		pendingStatus_ = "failed";
		return;
	}

	uint8_t digest[32];
	sha.finalize(digest, sizeof(digest));

	uint8_t signed_[40];
	for (int i = 0; i < 8; i++) {
		signed_[i] = (uint8_t)(otaSeq_ >> (8 * (7 - i)));
	}
	memcpy(signed_ + 8, digest, sizeof(digest));

	if (memcmp(digest, expectedSha, sizeof(digest)) != 0 ||
		!Ed25519::verify(signature, publicKey, signed_, sizeof(signed_))) {
		Update.abort();
		pendingStatus_ = "failed";
		return;
	}

	if (!Update.end(true)) {
		pendingStatus_ = "failed";
		return;
	}

	prefs_.begin(prefsNamespace, false);
	prefs_.putString(prefsOtaTargetKey, otaVersion_);
	prefs_.putULong64(prefsOtaPendSeqKey, otaSeq_);
	prefs_.end();

	delay(200);
	ESP.restart();
}

bool EspManagerClient::reportOTA(const char *status) {
	JsonDocument doc;
	doc["status"] = status;
	String out;
	serializeJson(doc, out);
	return mqtt_.publish(topic("ota/status").c_str(), out.c_str());
}

String EspManagerClient::topic(const char *suffix) const {
	return String("espmanager/") + deviceID_ + "/" + suffix;
}

bool EspManagerClient::hexToBytes(const String &hex, uint8_t *out, size_t outLen) {
	if (hex.length() != outLen * 2) {
		return false;
	}
	auto nibble = [](char c) -> int {
		if (c >= '0' && c <= '9') return c - '0';
		if (c >= 'a' && c <= 'f') return c - 'a' + 10;
		if (c >= 'A' && c <= 'F') return c - 'A' + 10;
		return -1;
	};
	for (size_t i = 0; i < outLen; i++) {
		int hi = nibble(hex[2 * i]);
		int lo = nibble(hex[2 * i + 1]);
		if (hi < 0 || lo < 0) {
			return false;
		}
		out[i] = (uint8_t)((hi << 4) | lo);
	}
	return true;
}

bool EspManagerClient::isAllZero(const uint8_t *data, size_t len) {
	for (size_t i = 0; i < len; i++) {
		if (data[i] != 0) {
			return false;
		}
	}
	return true;
}
