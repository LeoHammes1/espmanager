#include "espmanager_client.h"
#include "secrets.h"

void app_main(void) {
	espmanager_config_t cfg = {
		.wifi_ssid = WIFI_SSID,
		.wifi_password = WIFI_PASS,
		.host = ESPM_HOST,
		.http_port = 8080,
		.mqtt_port = 1883,
		.claim_token = ESPM_CLAIM_TOKEN,
		.firmware_version = "idf-1.0.0",
		.signing_public_key_hex = ESPM_SIGNING_PUBKEY,
		.heartbeat_interval_ms = 15000,
	};
	espmanager_start(&cfg);
}
