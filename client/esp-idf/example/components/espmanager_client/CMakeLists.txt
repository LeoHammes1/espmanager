idf_component_register(
    SRCS "espmanager_client.c"
    INCLUDE_DIRS "include"
    REQUIRES esp_wifi esp_event esp_netif esp_http_client mqtt app_update nvs_flash esp-tls json mbedtls libsodium
)
