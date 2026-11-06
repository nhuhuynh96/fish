package mqtt

import (
	"fmt"
	"log"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/nhuhuynh/iot-fish/internal/pkg/config"
)

type Client struct {
	cli  paho.Client
	mu   sync.Mutex
	subs map[string]paho.MessageHandler
}

type MessageHandler func(topic string, payload []byte)

func NewClient(cfg *config.Config) (*Client, error) {
	c := &Client{subs: map[string]paho.MessageHandler{}}
	opts := paho.NewClientOptions().
		AddBroker(cfg.MQTTBroker).
		SetClientID(fmt.Sprintf("%s_%d", cfg.MQTTClient, time.Now().UnixNano()%100000)).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetKeepAlive(30 * time.Second).
		SetOnConnectHandler(func(pc paho.Client) {
			log.Println("[MQTT] Connected to broker:", cfg.MQTTBroker)
			c.resubscribe(pc)
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Printf("[MQTT] Connection lost: %v", err)
		})

	if cfg.MQTTUser != "" {
		opts.SetUsername(cfg.MQTTUser)
		opts.SetPassword(cfg.MQTTPass)
	}

	c.cli = paho.NewClient(opts)
	token := c.cli.Connect()
	if token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return c, nil
}

// Clean session nên broker quên subscription mỗi lần nối lại; phải đăng ký lại.
func (c *Client) resubscribe(pc paho.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for topic, h := range c.subs {
		token := pc.Subscribe(topic, 1, h)
		if token.Wait() && token.Error() != nil {
			log.Printf("[MQTT] Đăng ký lại %s lỗi: %v", topic, token.Error())
			continue
		}
		log.Printf("[MQTT] Đã đăng ký lại %s", topic)
	}
}

func (c *Client) Close() {
	if c.cli.IsConnected() {
		c.cli.Disconnect(250)
	}
}

func (c *Client) Subscribe(topic string, h MessageHandler) error {
	handler := func(_ paho.Client, m paho.Message) {
		h(m.Topic(), m.Payload())
	}
	c.mu.Lock()
	c.subs[topic] = handler
	c.mu.Unlock()

	token := c.cli.Subscribe(topic, 1, handler)
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
