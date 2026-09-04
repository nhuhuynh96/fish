package mqtt

import (
	"fmt"
	"log"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/nhuhuynh/iot-fish/internal/pkg/config"
)

type Client struct {
	cli paho.Client
}

type MessageHandler func(topic string, payload []byte)

func NewClient(cfg *config.Config) (*Client, error) {
	opts := paho.NewClientOptions().
		AddBroker(cfg.MQTTBroker).
		SetClientID(fmt.Sprintf("%s_%d", cfg.MQTTClient, time.Now().UnixNano()%100000)).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetKeepAlive(30 * time.Second).
		SetOnConnectHandler(func(c paho.Client) {
			log.Println("[MQTT] Connected to broker:", cfg.MQTTBroker)
		}).
		SetConnectionLostHandler(func(c paho.Client, err error) {
			log.Printf("[MQTT] Connection lost: %v", err)
		})

	if cfg.MQTTUser != "" {
		opts.SetUsername(cfg.MQTTUser)
		opts.SetPassword(cfg.MQTTPass)
	}

	cli := paho.NewClient(opts)
	token := cli.Connect()
	if token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &Client{cli: cli}, nil
}

func (c *Client) Close() {
	if c.cli.IsConnected() {
		c.cli.Disconnect(250)
	}
}

func (c *Client) Subscribe(topic string, h MessageHandler) error {
	token := c.cli.Subscribe(topic, 1, func(_ paho.Client, m paho.Message) {
		h(m.Topic(), m.Payload())
	})
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *Client) Publish(topic string, payload []byte) error {
	token := c.cli.Publish(topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}
