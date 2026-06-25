package mqttbroker

import (
	"context"
	"errors"
	"strings"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/LeoHammes1/espmanager/internal/topics"
)

type Presence interface {
	Connected(id string)
	Disconnected(id string)
}

type Authenticator interface {
	Authenticate(ctx context.Context, deviceID, password string) bool
}

type Broker struct {
	server  *mqtt.Server
	nextSub int
}

func New(addr string, presence Presence, auth Authenticator) (*Broker, error) {
	server := mqtt.New(&mqtt.Options{InlineClient: true})

	if err := server.AddHook(&aclHook{auth: auth}, nil); err != nil {
		return nil, err
	}
	if err := server.AddHook(&presenceHook{presence: presence}, nil); err != nil {
		return nil, err
	}
	if err := server.AddListener(listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr})); err != nil {
		return nil, err
	}

	return &Broker{server: server}, nil
}

func (b *Broker) Start() error { return b.server.Serve() }

func (b *Broker) Close() error { return b.server.Close() }

func (b *Broker) Publish(topic string, payload []byte) error {
	return b.server.Publish(topic, payload, false, 1)
}

func (b *Broker) Subscribe(filter string, handler func(topic string, payload []byte)) error {
	b.nextSub++
	return b.server.Subscribe(filter, b.nextSub, func(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
		handler(pk.TopicName, pk.Payload)
	})
}

type aclHook struct {
	mqtt.HookBase
	auth Authenticator
}

func (h *aclHook) ID() string { return "acl" }

func (h *aclHook) Provides(b byte) bool {
	switch b {
	case mqtt.OnConnectAuthenticate, mqtt.OnACLCheck:
		return true
	default:
		return false
	}
}

func (h *aclHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	if cl.ID == mqtt.InlineClientId {
		return true
	}
	username := string(pk.Connect.Username)
	if username == "" || username != cl.ID {
		return false
	}
	return h.auth.Authenticate(context.Background(), username, string(pk.Connect.Password))
}

func (h *aclHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	if cl.ID == mqtt.InlineClientId {
		return true
	}
	return strings.HasPrefix(topic, topics.Root+"/"+cl.ID+"/")
}

type presenceHook struct {
	mqtt.HookBase
	presence Presence
}

func (h *presenceHook) ID() string { return "presence" }

func (h *presenceHook) Provides(b byte) bool {
	switch b {
	case mqtt.OnConnect, mqtt.OnDisconnect:
		return true
	default:
		return false
	}
}

func (h *presenceHook) OnConnect(cl *mqtt.Client, pk packets.Packet) error {
	h.presence.Connected(cl.ID)
	return nil
}

func (h *presenceHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	if errors.Is(cl.StopCause(), packets.ErrSessionTakenOver) {
		return
	}
	h.presence.Disconnected(cl.ID)
}
