#include "espmanager_client.h"

#include <errno.h>
#include <stdlib.h>
#include <string.h>

#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/event_groups.h"
#include "freertos/queue.h"
#include "freertos/semphr.h"

#include "cJSON.h"
#include "esp_event.h"
#include "esp_http_client.h"
#include "esp_log.h"
#include "esp_mac.h"
#include "esp_netif.h"
#include "esp_ota_ops.h"
#include "esp_timer.h"
#include "esp_wifi.h"
#include "mbedtls/sha256.h"
#include "mqtt_client.h"
#include "nvs.h"
#include "nvs_flash.h"
#include "sodium.h"
#include "driver/uart.h"
#include "esp_system.h"

static const char *TAG = "espmanager";

#define NS "espm"
#define KEY_PASS "mqttpass"
#define KEY_OLDPASS "mqttpassold"
#define KEY_TARGET "otatarget"
#define KEY_PENDSEQ "otapendseq"
#define KEY_FLOOR "otaseq"
#define KEY_FAILEDSEQ "otafailseq"
// Provisioning config written over serial by the browser wizard.
#define KEY_WSSID "wssid"
#define KEY_WPASS "wpass"
#define KEY_HOST "host"
#define KEY_HPORT "hport"
#define KEY_MPORT "mport"
#define KEY_CTOK "ctok"
#define WIFI_CONNECTED_BIT BIT0

typedef struct {
	char version[64];
	char url[256];
	char sha256[65];
	char signature[129];
	uint64_t sequence;
} ota_cmd_t;

#define CONFIRM_TIMEOUT_MS 120000

static espmanager_config_t s_cfg;
static char s_device_id[13];
static char s_password[96];
static char s_lwt_topic[80];
static uint64_t s_floor;
static uint64_t s_pendingSeq;
static uint64_t s_failedSeq;
static portMUX_TYPE s_floorLock = portMUX_INITIALIZER_UNLOCKED;
static esp_mqtt_client_handle_t s_mqtt;
static EventGroupHandle_t s_wifi_eg;
static QueueHandle_t s_ota_queue;
static SemaphoreHandle_t s_connected_sem;
static volatile bool s_awaiting_confirm;
static volatile bool s_report_rollback_failed;

// Owned copies of the connection config: seeded from the compile-time
// espmanager_config_t, then overridden by anything the wizard wrote to NVS.
static char s_ssid[33];
static char s_pass[65];
static char s_host[80];
static char s_token[64];

static void nvs_init_safe(void) {
	esp_err_t err = nvs_flash_init();
	if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
		ESP_ERROR_CHECK(nvs_flash_erase());
		ESP_ERROR_CHECK(nvs_flash_init());
	}
}

static void nvs_load_str(const char *key, char *out, size_t out_len) {
	out[0] = '\0';
	nvs_handle_t h;
	if (nvs_open(NS, NVS_READONLY, &h) != ESP_OK) {
		return;
	}
	size_t len = out_len;
	nvs_get_str(h, key, out, &len);
	nvs_close(h);
}

static esp_err_t nvs_store_str(const char *key, const char *val) {
	nvs_handle_t h;
	esp_err_t err = nvs_open(NS, NVS_READWRITE, &h);
	if (err != ESP_OK) {
		return err;
	}
	err = nvs_set_str(h, key, val);
	if (err == ESP_OK) {
		err = nvs_commit(h);
	}
	nvs_close(h);
	return err;
}

static uint64_t nvs_load_u64(const char *key) {
	nvs_handle_t h;
	uint64_t v = 0;
	if (nvs_open(NS, NVS_READONLY, &h) != ESP_OK) {
		return 0;
	}
	nvs_get_u64(h, key, &v);
	nvs_close(h);
	return v;
}

static void nvs_store_u64(const char *key, uint64_t v) {
	nvs_handle_t h;
	if (nvs_open(NS, NVS_READWRITE, &h) != ESP_OK) {
		return;
	}
	nvs_set_u64(h, key, v);
	nvs_commit(h);
	nvs_close(h);
}

static void nvs_clear(const char *key) {
	nvs_handle_t h;
	if (nvs_open(NS, NVS_READWRITE, &h) != ESP_OK) {
		return;
	}
	nvs_erase_key(h, key);
	nvs_commit(h);
	nvs_close(h);
}

static bool hex_to_bytes(const char *hex, uint8_t *out, size_t out_len) {
	if (strlen(hex) != out_len * 2) {
		return false;
	}
	for (size_t i = 0; i < out_len; i++) {
		char pair[3] = {hex[2 * i], hex[2 * i + 1], 0};
		char *end = NULL;
		long v = strtol(pair, &end, 16);
		if (*end != '\0') {
			return false;
		}
		out[i] = (uint8_t)v;
	}
	return true;
}

static void make_topic(char *buf, size_t n, const char *suffix) {
	snprintf(buf, n, "espmanager/%s/%s", s_device_id, suffix);
}

static void report_ota(const char *status, uint64_t sequence) {
	if (!s_mqtt) {
		return;
	}
	char seqstr[21];
	snprintf(seqstr, sizeof(seqstr), "%llu", (unsigned long long)sequence);
	cJSON *o = cJSON_CreateObject();
	cJSON_AddStringToObject(o, "status", status);
	cJSON_AddStringToObject(o, "sequence", seqstr);
	char *s = cJSON_PrintUnformatted(o);
	char t[80];
	make_topic(t, sizeof(t), "ota/status");
	esp_mqtt_client_publish(s_mqtt, t, s, 0, 1, 0);
	cJSON_free(s);
	cJSON_Delete(o);
}

static void publish_heartbeat(void) {
	if (!s_mqtt) {
		return;
	}
	cJSON *o = cJSON_CreateObject();
	cJSON_AddStringToObject(o, "version", s_cfg.firmware_version);
	wifi_ap_record_t ap;
	cJSON_AddNumberToObject(o, "rssi", esp_wifi_sta_get_ap_info(&ap) == ESP_OK ? ap.rssi : 0);
	cJSON_AddNumberToObject(o, "uptime", esp_timer_get_time() / 1000000);
	char *s = cJSON_PrintUnformatted(o);
	char t[80];
	make_topic(t, sizeof(t), "state");
	esp_mqtt_client_publish(s_mqtt, t, s, 0, 0, 0);
	cJSON_free(s);
	cJSON_Delete(o);
}

static void compute_device_id(void) {
	uint8_t mac[6];
	esp_read_mac(mac, ESP_MAC_WIFI_STA);
	snprintf(s_device_id, sizeof(s_device_id), "%02x%02x%02x%02x%02x%02x",
	         mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
}

static void wifi_event_handler(void *arg, esp_event_base_t base, int32_t id, void *data) {
	if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) {
		esp_wifi_connect();
	} else if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
		xEventGroupClearBits(s_wifi_eg, WIFI_CONNECTED_BIT);
		esp_wifi_connect();
	} else if (base == IP_EVENT && id == IP_EVENT_STA_GOT_IP) {
		xEventGroupSetBits(s_wifi_eg, WIFI_CONNECTED_BIT);
	}
}

static void wifi_start(void) {
	s_wifi_eg = xEventGroupCreate();
	esp_netif_create_default_wifi_sta();

	wifi_init_config_t ic = WIFI_INIT_CONFIG_DEFAULT();
	ESP_ERROR_CHECK(esp_wifi_init(&ic));
	ESP_ERROR_CHECK(esp_event_handler_instance_register(WIFI_EVENT, ESP_EVENT_ANY_ID, wifi_event_handler, NULL, NULL));
	ESP_ERROR_CHECK(esp_event_handler_instance_register(IP_EVENT, IP_EVENT_STA_GOT_IP, wifi_event_handler, NULL, NULL));

	wifi_config_t wc = {0};
	strlcpy((char *)wc.sta.ssid, s_cfg.wifi_ssid, sizeof(wc.sta.ssid));
	strlcpy((char *)wc.sta.password, s_cfg.wifi_password, sizeof(wc.sta.password));
	ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
	ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wc));
	ESP_ERROR_CHECK(esp_wifi_start());

	xEventGroupWaitBits(s_wifi_eg, WIFI_CONNECTED_BIT, pdFALSE, pdTRUE, portMAX_DELAY);
}

static bool claim_credentials(void) {
	if (!s_cfg.claim_token || s_cfg.claim_token[0] == '\0') {
		return false;
	}

	char url[160];
	snprintf(url, sizeof(url), "http://%s:%u/v1/claim", s_cfg.host, s_cfg.http_port);

	cJSON *req = cJSON_CreateObject();
	cJSON_AddStringToObject(req, "device_id", s_device_id);
	cJSON_AddStringToObject(req, "token", s_cfg.claim_token);
	char *body = cJSON_PrintUnformatted(req);
	cJSON_Delete(req);

	esp_http_client_config_t cfg = {.url = url, .method = HTTP_METHOD_POST, .timeout_ms = 10000};
	esp_http_client_handle_t c = esp_http_client_init(&cfg);
	esp_http_client_set_header(c, "Content-Type", "application/json");

	bool ok = false;
	if (esp_http_client_open(c, strlen(body)) == ESP_OK) {
		esp_http_client_write(c, body, strlen(body));
		esp_http_client_fetch_headers(c);
		if (esp_http_client_get_status_code(c) == 200) {
			char resp[256];
			int total = 0, n;
			while (total < (int)sizeof(resp) - 1 &&
			       (n = esp_http_client_read(c, resp + total, sizeof(resp) - 1 - total)) > 0) {
				total += n;
			}
			resp[total] = '\0';
			cJSON *r = cJSON_Parse(resp);
			cJSON *pw = cJSON_GetObjectItem(r, "password");
			if (cJSON_IsString(pw) && strlen(pw->valuestring) < sizeof(s_password)) {
				strlcpy(s_password, pw->valuestring, sizeof(s_password));
				if (nvs_store_str(KEY_PASS, s_password) == ESP_OK) {
					ok = true;
				}
			}
			cJSON_Delete(r);
		}
	}
	esp_http_client_close(c);
	esp_http_client_cleanup(c);
	free(body);
	return ok;
}

static bool download_verify_apply(const ota_cmd_t *cmd) {
	uint8_t expected[32], signature[64], pubkey[32];
	if (!hex_to_bytes(cmd->sha256, expected, sizeof(expected)) ||
	    !hex_to_bytes(cmd->signature, signature, sizeof(signature)) ||
	    !hex_to_bytes(s_cfg.signing_public_key_hex, pubkey, sizeof(pubkey)) ||
	    sodium_is_zero(pubkey, sizeof(pubkey))) {
		return false;
	}

	report_ota("updating", cmd->sequence);

	esp_http_client_config_t hc = {.url = cmd->url, .timeout_ms = 20000};
	esp_http_client_handle_t c = esp_http_client_init(&hc);
	if (esp_http_client_open(c, 0) != ESP_OK) {
		esp_http_client_cleanup(c);
		return false;
	}
	esp_http_client_fetch_headers(c);
	if (esp_http_client_get_status_code(c) != 200) {
		esp_http_client_close(c);
		esp_http_client_cleanup(c);
		return false;
	}

	const esp_partition_t *part = esp_ota_get_next_update_partition(NULL);
	esp_ota_handle_t oh;
	if (part == NULL || esp_ota_begin(part, OTA_SIZE_UNKNOWN, &oh) != ESP_OK) {
		esp_http_client_close(c);
		esp_http_client_cleanup(c);
		return false;
	}

	mbedtls_sha256_context sc;
	mbedtls_sha256_init(&sc);
	mbedtls_sha256_starts(&sc, 0);

	uint8_t buf[1024];
	int n;
	bool ok = true;
	while ((n = esp_http_client_read(c, (char *)buf, sizeof(buf))) > 0) {
		if (esp_ota_write(oh, buf, n) != ESP_OK) {
			ok = false;
			break;
		}
		mbedtls_sha256_update(&sc, buf, n);
	}
	if (n < 0 || !esp_http_client_is_complete_data_received(c)) {
		ok = false;
	}
	esp_http_client_close(c);
	esp_http_client_cleanup(c);

	uint8_t digest[32];
	mbedtls_sha256_finish(&sc, digest);
	mbedtls_sha256_free(&sc);

	if (!ok) {
		esp_ota_abort(oh);
		return false;
	}
	if (esp_ota_end(oh) != ESP_OK) {
		return false;
	}

	uint8_t msg[40];
	for (int i = 0; i < 8; i++) {
		msg[i] = (uint8_t)(cmd->sequence >> (8 * (7 - i)));
	}
	memcpy(msg + 8, digest, sizeof(digest));

	if (memcmp(digest, expected, sizeof(digest)) != 0 ||
	    crypto_sign_verify_detached(signature, msg, sizeof(msg), pubkey) != 0) {
		ESP_LOGW(TAG, "ota verification failed");
		return false;
	}
	nvs_store_str(KEY_TARGET, cmd->version);
	nvs_store_u64(KEY_PENDSEQ, cmd->sequence);
	if (esp_ota_set_boot_partition(part) != ESP_OK) {
		nvs_clear(KEY_TARGET);
		nvs_clear(KEY_PENDSEQ);
		return false;
	}
	ESP_LOGI(TAG, "ota verified, rebooting into %s", cmd->version);
	vTaskDelay(pdMS_TO_TICKS(200));
	esp_restart();
	return true;
}

static void clear_inflight(uint64_t sequence) {
	portENTER_CRITICAL(&s_floorLock);
	if (s_pendingSeq == sequence) {
		s_pendingSeq = 0;
	}
	portEXIT_CRITICAL(&s_floorLock);
}

static void ota_task(void *arg) {
	ota_cmd_t cmd;
	for (;;) {
		if (xQueueReceive(s_ota_queue, &cmd, portMAX_DELAY) == pdTRUE) {
			if (!download_verify_apply(&cmd)) {
				clear_inflight(cmd.sequence);
				report_ota("failed", cmd.sequence);
			}
		}
	}
}

static void heartbeat_task(void *arg) {
	for (;;) {
		vTaskDelay(pdMS_TO_TICKS(s_cfg.heartbeat_interval_ms));
		publish_heartbeat();
	}
}

// confirm_task is the single owner of the confirm-or-rollback decision for a
// freshly applied image. It either confirms (the image reached the broker within
// the deadline) or forces the bootloader to roll back, so the two outcomes can
// never race.
static void confirm_task(void *arg) {
	if (xSemaphoreTake(s_connected_sem, pdMS_TO_TICKS(CONFIRM_TIMEOUT_MS)) != pdTRUE) {
		ESP_LOGW(TAG, "new image did not confirm in time; rolling back");
		esp_ota_mark_app_invalid_rollback_and_reboot();
		vTaskDelete(NULL);
		return;
	}
	if (esp_ota_mark_app_valid_cancel_rollback() != ESP_OK) {
		ESP_LOGW(TAG, "mark_app_valid failed; rolling back");
		esp_ota_mark_app_invalid_rollback_and_reboot();
		vTaskDelete(NULL);
		return;
	}

	uint64_t pending = nvs_load_u64(KEY_PENDSEQ);
	portENTER_CRITICAL(&s_floorLock);
	bool advance = pending > s_floor;
	if (advance) {
		s_floor = pending;
	}
	s_pendingSeq = 0;
	portEXIT_CRITICAL(&s_floorLock);

	if (advance) {
		nvs_store_u64(KEY_FLOOR, pending);
	}
	// A newer image confirmed; any earlier failed-sequence quarantine is now below
	// the floor and no longer needed.
	s_failedSeq = 0;
	nvs_clear(KEY_FAILEDSEQ);
	nvs_clear(KEY_TARGET);
	nvs_clear(KEY_PENDSEQ);
	report_ota("ok", pending);
	vTaskDelete(NULL);
}

static bool running_image_pending_verify(void) {
	esp_ota_img_states_t state;
	if (esp_ota_get_state_partition(esp_ota_get_running_partition(), &state) != ESP_OK) {
		return false;
	}
	return state == ESP_OTA_IMG_PENDING_VERIFY;
}

static void arm_confirmation(void) {
	if (running_image_pending_verify()) {
		s_pendingSeq = nvs_load_u64(KEY_PENDSEQ);
		s_connected_sem = xSemaphoreCreateBinary();
		s_awaiting_confirm = true;
		xTaskCreate(confirm_task, "espm_cfm", 4096, NULL, 6, NULL);
		return;
	}
	// Not pending-verify but a pending sequence is still recorded. If it never
	// advanced the floor, the bootloader rolled the image back: report the
	// failure. If the floor already covers it, the image confirmed and only the
	// final bookkeeping was interrupted (e.g. a power cut) — clear it silently.
	uint64_t pending = nvs_load_u64(KEY_PENDSEQ);
	if (pending != 0) {
		nvs_clear(KEY_TARGET);
		nvs_clear(KEY_PENDSEQ);
		if (pending > s_floor) {
			s_report_rollback_failed = true;
			// Quarantine the rolled-back sequence so a re-published bad image is not
			// applied again in an endless reboot loop.
			s_failedSeq = pending;
			nvs_store_u64(KEY_FAILEDSEQ, pending);
		}
	}
}

static void handle_cmd_ota(const char *data, int len) {
	cJSON *j = cJSON_ParseWithLength(data, len);
	if (j == NULL) {
		return;
	}
	cJSON *ver = cJSON_GetObjectItem(j, "version");
	cJSON *url = cJSON_GetObjectItem(j, "url");
	cJSON *sha = cJSON_GetObjectItem(j, "sha256");
	cJSON *sig = cJSON_GetObjectItem(j, "signature");
	cJSON *seq = cJSON_GetObjectItem(j, "sequence");

	// The sequence is carried as a string so the full 64-bit value survives JSON
	// (a JSON number is a double and loses precision above 2^53).
	if (cJSON_IsString(ver) && cJSON_IsString(url) && cJSON_IsString(sha) && cJSON_IsString(sig) && cJSON_IsString(seq)) {
		char *end = NULL;
		errno = 0;
		uint64_t sequence = strtoull(seq->valuestring, &end, 10);
		if (seq->valuestring[0] == '\0' || end == seq->valuestring || *end != '\0' || errno == ERANGE) {
			cJSON_Delete(j);
			return;
		}

		// A sequence that already failed and rolled back is quarantined: re-applying
		// it would just loop through the same bad image.
		if (s_failedSeq != 0 && sequence == s_failedSeq) {
			report_ota("failed", sequence);
			cJSON_Delete(j);
			return;
		}

		// Claim the single in-flight slot atomically. Only one OTA may be in
		// flight at a time: applying a second image while the first is still
		// unconfirmed would overwrite the partition we need to roll back to.
		bool accept = false;
		portENTER_CRITICAL(&s_floorLock);
		uint64_t floor = s_floor;
		uint64_t inflight = s_pendingSeq;
		if (sequence > floor && inflight == 0) {
			s_pendingSeq = sequence;
			accept = true;
		}
		portEXIT_CRITICAL(&s_floorLock);

		if (accept) {
			ota_cmd_t cmd = {0};
			strlcpy(cmd.version, ver->valuestring, sizeof(cmd.version));
			strlcpy(cmd.url, url->valuestring, sizeof(cmd.url));
			strlcpy(cmd.sha256, sha->valuestring, sizeof(cmd.sha256));
			strlcpy(cmd.signature, sig->valuestring, sizeof(cmd.signature));
			cmd.sequence = sequence;
			if (xQueueSend(s_ota_queue, &cmd, 0) != pdTRUE) {
				clear_inflight(sequence);
				report_ota("failed", sequence);
			}
		} else if (sequence < floor) {
			report_ota("failed", sequence);
		} else if (sequence == floor) {
			report_ota("ok", sequence);
		} else if (sequence == inflight) {
			report_ota("updating", sequence);
		}
		// else: a different OTA is in flight; drop and let the server redeliver this
		// one once the current image finishes.
	}
	cJSON_Delete(j);
}

static void handle_cmd_cred(const char *data, int len) {
	cJSON *j = cJSON_ParseWithLength(data, len);
	if (j == NULL) {
		return;
	}
	cJSON *pw = cJSON_GetObjectItem(j, "password");
	bool ok = cJSON_IsString(pw) && pw->valuestring[0] != '\0' && strlen(pw->valuestring) < sizeof(s_password);
	if (ok) {
		// Keep the working credential so a rejected rotation can roll back instead
		// of bricking the device.
		nvs_store_str(KEY_OLDPASS, s_password);
		nvs_store_str(KEY_PASS, pw->valuestring);
	}
	cJSON_Delete(j);
	if (ok) {
		ESP_LOGI(TAG, "credential rotated; restarting to reconnect");
		vTaskDelay(pdMS_TO_TICKS(200));
		esp_restart();
	}
}

static bool topic_is(esp_mqtt_event_handle_t e, const char *suffix) {
	char t[80];
	make_topic(t, sizeof(t), suffix);
	return e->topic_len == (int)strlen(t) && strncmp(e->topic, t, e->topic_len) == 0;
}

static void mqtt_event_handler(void *args, esp_event_base_t base, int32_t id, void *event_data) {
	esp_mqtt_event_handle_t e = event_data;
	switch ((esp_mqtt_event_id_t)id) {
	case MQTT_EVENT_CONNECTED: {
		// The active credential authenticated, so any rotation rollback copy is no
		// longer needed.
		nvs_clear(KEY_OLDPASS);
		char t[80];
		make_topic(t, sizeof(t), "availability");
		esp_mqtt_client_publish(s_mqtt, t, "online", 0, 1, 1);
		make_topic(t, sizeof(t), "cmd/ota");
		esp_mqtt_client_subscribe(s_mqtt, t, 1);
		make_topic(t, sizeof(t), "cmd/cred");
		esp_mqtt_client_subscribe(s_mqtt, t, 1);
		publish_heartbeat();
		if (s_report_rollback_failed) {
			s_report_rollback_failed = false;
			report_ota("failed", s_failedSeq);
		}
		if (s_awaiting_confirm) {
			s_awaiting_confirm = false;
			xSemaphoreGive(s_connected_sem);
		}
		break;
	}
	case MQTT_EVENT_DATA: {
		if (topic_is(e, "cmd/ota")) {
			handle_cmd_ota(e->data, e->data_len);
		} else if (topic_is(e, "cmd/cred")) {
			handle_cmd_cred(e->data, e->data_len);
		}
		break;
	}
	case MQTT_EVENT_ERROR: {
		// A rejected credential (e.g. a rotation that did not stick) would
		// otherwise loop forever; roll back to the last working password once.
		if (e->error_handle != NULL && e->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED &&
		    (e->error_handle->connect_return_code == MQTT_CONNECTION_REFUSE_BAD_USERNAME ||
		     e->error_handle->connect_return_code == MQTT_CONNECTION_REFUSE_NOT_AUTHORIZED)) {
			char previous[sizeof(s_password)];
			nvs_load_str(KEY_OLDPASS, previous, sizeof(previous));
			if (previous[0] != '\0') {
				nvs_store_str(KEY_PASS, previous);
				nvs_clear(KEY_OLDPASS);
				ESP_LOGW(TAG, "credential rejected; rolling back to the previous one");
				vTaskDelay(pdMS_TO_TICKS(200));
				esp_restart();
			}
		}
		break;
	}
	default:
		break;
	}
}

static void mqtt_start(void) {
	char uri[96];
	snprintf(uri, sizeof(uri), "mqtt://%s:%u", s_cfg.host, s_cfg.mqtt_port);
	make_topic(s_lwt_topic, sizeof(s_lwt_topic), "availability");

	esp_mqtt_client_config_t cfg = {
		.broker.address.uri = uri,
		.credentials.client_id = s_device_id,
		.credentials.username = s_device_id,
		.credentials.authentication.password = s_password,
		.session.last_will = {
			.topic = s_lwt_topic,
			.msg = "offline",
			.msg_len = 0,
			.qos = 1,
			.retain = 1,
		},
	};
	s_mqtt = esp_mqtt_client_init(&cfg);
	esp_mqtt_client_register_event(s_mqtt, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);
	esp_mqtt_client_start(s_mqtt);
}

// resolve_config seeds the live connection config from the compile-time defaults
// and lets any value the wizard wrote to NVS override it. The signing pubkey is
// never NVS-overridable — it stays the compile-time trust anchor.
static void resolve_config(const espmanager_config_t *cfg) {
	strlcpy(s_ssid, cfg->wifi_ssid ? cfg->wifi_ssid : "", sizeof(s_ssid));
	strlcpy(s_pass, cfg->wifi_password ? cfg->wifi_password : "", sizeof(s_pass));
	strlcpy(s_host, cfg->host ? cfg->host : "", sizeof(s_host));
	strlcpy(s_token, cfg->claim_token ? cfg->claim_token : "", sizeof(s_token));

	char tmp[96];
	nvs_load_str(KEY_WSSID, tmp, sizeof(tmp));
	if (tmp[0]) strlcpy(s_ssid, tmp, sizeof(s_ssid));
	nvs_load_str(KEY_WPASS, tmp, sizeof(tmp));
	if (tmp[0]) strlcpy(s_pass, tmp, sizeof(s_pass));
	nvs_load_str(KEY_HOST, tmp, sizeof(tmp));
	if (tmp[0]) strlcpy(s_host, tmp, sizeof(s_host));
	nvs_load_str(KEY_CTOK, tmp, sizeof(tmp));
	if (tmp[0]) strlcpy(s_token, tmp, sizeof(s_token));

	uint64_t hp = nvs_load_u64(KEY_HPORT);
	uint64_t mp = nvs_load_u64(KEY_MPORT);
	if (hp > 0 && hp <= 65535) s_cfg.http_port = (uint16_t)hp;
	if (mp > 0 && mp <= 65535) s_cfg.mqtt_port = (uint16_t)mp;

	s_cfg.wifi_ssid = s_ssid;
	s_cfg.wifi_password = s_pass;
	s_cfg.host = s_host;
	s_cfg.claim_token = s_token;
}

static void agent_send(const char *line) {
	uart_write_bytes(UART_NUM_0, line, strlen(line));
}

// provision_agent is the serial onboarding channel the browser wizard drives over
// USB when the device is unprovisioned. Line protocol on UART0 (logging muted so
// the stream is clean):
//   <- ESPM:READY <mac>          (on entry; also answer to ESPM:GETMAC)
//   -> ESPM:SET <key> <value>    (ssid|pass|host|token|hport|mport) -> ESPM:OK
//   -> ESPM:PROVISION            (commit to NVS + reboot) -> ESPM:OK PROVISIONED
static void provision_agent(void) {
	esp_log_level_set("*", ESP_LOG_NONE);
	uart_driver_install(UART_NUM_0, 1024, 0, 0, NULL, 0);

	char ssid[33] = "", pass[65] = "", host[80] = "", token[64] = "";
	uint16_t hport = s_cfg.http_port, mport = s_cfg.mqtt_port;

	char ready[40];
	snprintf(ready, sizeof(ready), "ESPM:READY %s\n", s_device_id);
	agent_send(ready);

	char line[200];
	int len = 0;
	for (;;) {
		uint8_t ch;
		if (uart_read_bytes(UART_NUM_0, &ch, 1, portMAX_DELAY) != 1) continue;
		if (ch == '\r') continue;
		if (ch != '\n') {
			if (len < (int)sizeof(line) - 1) line[len++] = (char)ch;
			continue;
		}
		line[len] = '\0';
		len = 0;
		if (strncmp(line, "ESPM:", 5) != 0) continue;
		char *cmd = line + 5;

		if (strcmp(cmd, "GETMAC") == 0) {
			agent_send(ready);
		} else if (strncmp(cmd, "SET ", 4) == 0) {
			char *kv = cmd + 4;
			char *sp = strchr(kv, ' ');
			char *val = "";
			if (sp) {
				*sp = '\0';
				val = sp + 1;
			}
			if (strcmp(kv, "ssid") == 0) strlcpy(ssid, val, sizeof(ssid));
			else if (strcmp(kv, "pass") == 0) strlcpy(pass, val, sizeof(pass));
			else if (strcmp(kv, "host") == 0) strlcpy(host, val, sizeof(host));
			else if (strcmp(kv, "token") == 0) strlcpy(token, val, sizeof(token));
			else if (strcmp(kv, "hport") == 0 || strcmp(kv, "mport") == 0) {
				char *end;
				long p = strtol(val, &end, 10);
				if (*val == '\0' || *end != '\0' || p < 1 || p > 65535) {
					agent_send("ESPM:ERR port\n");
					continue;
				}
				if (kv[0] == 'h') hport = (uint16_t)p;
				else mport = (uint16_t)p;
			} else {
				agent_send("ESPM:ERR key\n");
				continue;
			}
			agent_send("ESPM:OK\n");
		} else if (strcmp(cmd, "PROVISION") == 0) {
			if (ssid[0] == '\0' || host[0] == '\0' || token[0] == '\0') {
				agent_send("ESPM:ERR incomplete\n");
				continue;
			}
			nvs_store_str(KEY_WSSID, ssid);
			nvs_store_str(KEY_WPASS, pass);
			nvs_store_str(KEY_HOST, host);
			nvs_store_str(KEY_CTOK, token);
			nvs_store_u64(KEY_HPORT, hport);
			nvs_store_u64(KEY_MPORT, mport);
			// Re-onboarding must start clean: drop any prior manager's credential
			// and OTA anti-downgrade state so the device re-claims and accepts the
			// new manager's sequence floor.
			nvs_clear(KEY_PASS);
			nvs_clear(KEY_OLDPASS);
			nvs_clear(KEY_FLOOR);
			nvs_clear(KEY_FAILEDSEQ);
			nvs_clear(KEY_TARGET);
			nvs_clear(KEY_PENDSEQ);
			agent_send("ESPM:OK PROVISIONED\n");
			vTaskDelay(pdMS_TO_TICKS(250));
			esp_restart();
		} else {
			agent_send("ESPM:ERR cmd\n");
		}
	}
}

void espmanager_start(const espmanager_config_t *cfg) {
	s_cfg = *cfg;
	if (s_cfg.heartbeat_interval_ms == 0) {
		s_cfg.heartbeat_interval_ms = 15000;
	}

	nvs_init_safe();
	if (sodium_init() < 0) {
		ESP_LOGE(TAG, "libsodium init failed");
		return;
	}

	ESP_ERROR_CHECK(esp_netif_init());
	ESP_ERROR_CHECK(esp_event_loop_create_default());

	compute_device_id();
	resolve_config(cfg);

	// Unprovisioned (no WiFi from NVS or compile-time): hand the serial port to
	// the browser wizard. provision_agent reboots into the normal path once the
	// operator commits, so it never returns.
	if (s_cfg.wifi_ssid[0] == '\0') {
		ESP_LOGI(TAG, "device %s unprovisioned; starting serial provisioning agent", s_device_id);
		provision_agent();
		return;
	}

	ESP_LOGI(TAG, "device %s firmware %s", s_device_id, s_cfg.firmware_version);

	nvs_load_str(KEY_PASS, s_password, sizeof(s_password));
	s_floor = nvs_load_u64(KEY_FLOOR);
	s_failedSeq = nvs_load_u64(KEY_FAILEDSEQ);

	arm_confirmation();

	wifi_start();

	while (s_password[0] == '\0') {
		if (!claim_credentials()) {
			ESP_LOGW(TAG, "claim failed; retrying");
			vTaskDelay(pdMS_TO_TICKS(15000));
		}
	}

	s_ota_queue = xQueueCreate(2, sizeof(ota_cmd_t));
	xTaskCreate(ota_task, "espm_ota", 8192, NULL, 5, NULL);
	xTaskCreate(heartbeat_task, "espm_hb", 4096, NULL, 4, NULL);

	mqtt_start();
}
