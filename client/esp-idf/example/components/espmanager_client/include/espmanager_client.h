#pragma once

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
	const char *wifi_ssid;
	const char *wifi_password;
	const char *host;
	uint16_t http_port;
	uint16_t mqtt_port;
	const char *claim_token;
	const char *firmware_version;
	const char *signing_public_key_hex;
	uint32_t heartbeat_interval_ms;
} espmanager_config_t;

// espmanager_start brings up WiFi, ensures per-device MQTT credentials (claiming
// once with claim_token if none are stored), connects to the broker, and serves
// presence, heartbeats and signed A/B OTA updates with anti-downgrade and
// rollback-on-failed-boot. It blocks until WiFi is up, then returns; the MQTT
// client, heartbeat and OTA workers run on their own tasks.
void espmanager_start(const espmanager_config_t *cfg);

#ifdef __cplusplus
}
#endif
