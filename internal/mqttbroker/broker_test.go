package mqttbroker

import (
	"testing"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

type recordingPresence struct {
	connected    []string
	disconnected []string
}

func (r *recordingPresence) Connected(id string)    { r.connected = append(r.connected, id) }
func (r *recordingPresence) Disconnected(id string) { r.disconnected = append(r.disconnected, id) }

func TestPresenceIgnoresSessionTakeover(t *testing.T) {
	rec := &recordingPresence{}
	hook := &presenceHook{presence: rec}

	cl := &mqtt.Client{ID: "device-1"}
	cl.Stop(packets.ErrSessionTakenOver)

	hook.OnDisconnect(cl, packets.ErrSessionTakenOver, false)

	if len(rec.disconnected) != 0 {
		t.Fatalf("takeover must not mark the device offline, got %v", rec.disconnected)
	}
}

func TestPresenceReportsRealDisconnect(t *testing.T) {
	rec := &recordingPresence{}
	hook := &presenceHook{presence: rec}

	cl := &mqtt.Client{ID: "device-1"}
	cl.Stop(packets.ErrKeepAliveTimeout)

	hook.OnDisconnect(cl, packets.ErrKeepAliveTimeout, true)

	if len(rec.disconnected) != 1 || rec.disconnected[0] != "device-1" {
		t.Fatalf("genuine disconnect must mark the device offline, got %v", rec.disconnected)
	}
}
