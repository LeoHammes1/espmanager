#pragma once

// ESPManager device configuration. Two ways to onboard a device:
//
// 1) Browser wizard (recommended): leave WiFi / host / claim-token EMPTY. On
//    first boot the device runs the serial provisioning agent, and the "Add
//    device" wizard flashes this firmware and writes WiFi + manager host + a
//    MAC-bound claim token over USB. Only the signing pubkey is baked in — it is
//    the OTA trust anchor and is never provisioned over the wire.
//
// 2) Manual: fill everything in, flash, and the device claims on first boot.

#define WIFI_SSID ""
#define WIFI_PASS ""
#define ESPM_HOST ""        // optional default; the wizard can override it
#define ESPM_CLAIM_TOKEN "" // leave empty for the wizard flow
#define ESPM_SIGNING_PUBKEY "REPLACE_WITH_SIGNER_PUBLIC_KEY_HEX"
