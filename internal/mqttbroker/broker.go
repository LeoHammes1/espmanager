package mqttbroker

import (
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

type Tracker interface {
	Connected(id string)
	Disconnected(id string)
	Seen(id, topic string, payload []byte)
}

type Broker struct {
	server *mqtt.Server
}

func New(addr string, tracker Tracker) (*Broker, error) {
	server := mqtt.New(&mqtt.Options{InlineClient: true})

	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		return nil, err
	}
	if err := server.AddHook(&presenceHook{tracker: tracker}, nil); err != nil {
		return nil, err
	}
	if err := server.AddListener(listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr})); err != nil {
		return nil, err
	}

	return &Broker{server: server}, nil
}

func (b *Broker) Start() error { return b.server.Serve() }

func (b *Broker) Close() error { return b.server.Close() }

type presenceHook struct {
	mqtt.HookBase
	tracker Tracker
}

func (h *presenceHook) ID() string { return "presence" }

func (h *presenceHook) Provides(b byte) bool {
	switch b {
	case mqtt.OnConnect, mqtt.OnDisconnect, mqtt.OnPublish:
		return true
	default:
		return false
	}
}

func (h *presenceHook) OnConnect(cl *mqtt.Client, pk packets.Packet) error {
	h.tracker.Connected(cl.ID)
	return nil
}

func (h *presenceHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	h.tracker.Disconnected(cl.ID)
}

func (h *presenceHook) OnPublish(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	h.tracker.Seen(cl.ID, pk.TopicName, pk.Payload)
	return pk, nil
}
