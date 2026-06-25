package topics

import "strings"

const Root = "espmanager"

func Availability(deviceID string) string { return Root + "/" + deviceID + "/availability" }

func State(deviceID string) string { return Root + "/" + deviceID + "/state" }

func CmdOTA(deviceID string) string { return Root + "/" + deviceID + "/cmd/ota" }

func OTAStatus(deviceID string) string { return Root + "/" + deviceID + "/ota/status" }

func StateFilter() string { return Root + "/+/state" }

func OTAStatusFilter() string { return Root + "/+/ota/status" }

func DeviceFromTopic(topic string) (string, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) >= 2 && parts[0] == Root && parts[1] != "" {
		return parts[1], true
	}
	return "", false
}
