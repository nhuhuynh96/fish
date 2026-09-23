package mqtt

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/nhuhuynh/iot-fish/esp32_go/internal/pkg/config"
)

type Client struct {
	cli      mqtt.Client
	deviceID string
}

func NewClient(cfg *config.Config, onCommand func(topic string, payload []byte)) (*Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)
	if cfg.MQTTUser != "" {
		opts.SetUsername(cfg.MQTTUser)
		opts.SetPassword(cfg.MQTTPass)
	}
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	will := fmt.Sprintf(`{"status":"offline","device_id":"%s"}`, cfg.DeviceID)
	opts.SetWill(fmt.Sprintf("fish/%s/status", cfg.DeviceID), will, 1, true)

	c := mqtt.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return nil, fmt.Errorf("mqtt connect timeout")
	}
	if err := token.Error(); err != nil {
		return nil, err
	}

	cmdTopic := fmt.Sprintf("fish/%s/command", cfg.DeviceID)
	sub := c.Subscribe(cmdTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		onCommand(msg.Topic(), msg.Payload())
	})
	sub.Wait()
	if err := sub.Error(); err != nil {
		return nil, err
	}
	log.Printf("[MQTT] Subscribe %s", cmdTopic)

	online := fmt.Sprintf(`{"status":"online","device_id":"%s","ip":"sim"}`, cfg.DeviceID)
	c.Publish(fmt.Sprintf("fish/%s/status", cfg.DeviceID), 1, true, online)

	return &Client{cli: c, deviceID: cfg.DeviceID}, nil
}

func (c *Client) Publish(subTopic string, payload []byte, retained bool) error {
	if c == nil || c.cli == nil || !c.cli.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}
	topic := fmt.Sprintf("fish/%s/%s", c.deviceID, subTopic)
	token := c.cli.Publish(topic, 1, retained, payload)
	token.Wait()
	return token.Error()
}

func (c *Client) Connected() bool {
	return c != nil && c.cli != nil && c.cli.IsConnected()
}

func (c *Client) Close() {
	if c == nil || c.cli == nil {
		return
	}
	offline := fmt.Sprintf(`{"status":"offline","device_id":"%s"}`, c.deviceID)
	c.cli.Publish(fmt.Sprintf("fish/%s/status", c.deviceID), 1, true, offline)
	c.cli.Disconnect(250)
}
