package mqttiface

import (
	"context"
	"log"
	"strings"

	inframqtt "github.com/nhuhuynh/iot-fish/internal/infrastructure/mqtt"
	fishuc "github.com/nhuhuynh/iot-fish/internal/usecase/fish"
)

type Subscriber struct {
	client *inframqtt.Client
	svc    *fishuc.Service
}

func NewSubscriber(client *inframqtt.Client, svc *fishuc.Service) *Subscriber {
	return &Subscriber{client: client, svc: svc}
}

func (s *Subscriber) Start() error {
	if err := s.client.Subscribe("fish/+/sensor_data", s.onSensorData); err != nil {
		return err
	}
	if err := s.client.Subscribe("fish/+/event", s.onEvent); err != nil {
		return err
	}
	if err := s.client.Subscribe("fish/+/status", s.onStatus); err != nil {
		return err
	}
	if err := s.client.Subscribe("fish/+/telemetry", s.onTelemetry); err != nil {
		return err
	}

	log.Println("[MQTT Subscriber] Subscribed to fish/+/sensor_data, fish/+/event, fish/+/status, fish/+/telemetry")
	return nil
}

func (s *Subscriber) onSensorData(topic string, payload []byte) {
	deviceID := extractDeviceID(topic)
	if deviceID == "" {
		return
	}
	if err := s.svc.HandleSensorData(context.Background(), deviceID, payload); err != nil {
		log.Printf("[MQTT] handle sensor data error: %v", err)
	}
}

func (s *Subscriber) onEvent(topic string, payload []byte) {
	deviceID := extractDeviceID(topic)
	if deviceID == "" {
		return
	}
	if err := s.svc.HandleEvent(context.Background(), deviceID, payload); err != nil {
		log.Printf("[MQTT] handle event error: %v", err)
	}
}

func (s *Subscriber) onStatus(topic string, payload []byte) {
	deviceID := extractDeviceID(topic)
	if deviceID == "" {
		return
	}
	if err := s.svc.HandleStatus(context.Background(), deviceID, payload); err != nil {
		log.Printf("[MQTT] handle status error: %v", err)
	}
}

func (s *Subscriber) onTelemetry(topic string, payload []byte) {
	deviceID := extractDeviceID(topic)
	if deviceID == "" {
		return
	}
	if err := s.svc.HandleTelemetry(context.Background(), deviceID, payload); err != nil {
		log.Printf("[MQTT] handle telemetry error: %v", err)
	}
}

func extractDeviceID(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 || parts[0] != "fish" {
		return ""
	}
	return parts[1]
}
