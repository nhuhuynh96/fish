package mqtt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

type CommandPublisher struct {
	client *Client
}

func NewCommandPublisher(client *Client) *CommandPublisher {
	return &CommandPublisher{client: client}
}

var _ fish.CommandPublisher = (*CommandPublisher)(nil)

func (p *CommandPublisher) PublishMeasure(ctx context.Context, deviceID string, sensors []string) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	payload, err := json.Marshal(map[string]any{
		"action":  "measure",
		"sensors": sensors,
	})
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}

func (p *CommandPublisher) PublishPump(ctx context.Context, deviceID string, target string, state bool) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	stateStr := "OFF"
	if state {
		stateStr = "ON"
	}
	payload, err := json.Marshal(map[string]any{
		"action": "pump",
		"target": target,
		"state":  stateStr,
	})
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}

func (p *CommandPublisher) PublishClearQueue(ctx context.Context, deviceID string) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	payload, err := json.Marshal(map[string]any{
		"action": "clear_queue",
	})
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}

func (p *CommandPublisher) PublishSchedule(ctx context.Context, deviceID string, enabled bool, temp, ph, turb, tds int) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	payload, err := json.Marshal(map[string]any{
		"action":        "schedule",
		"auto_enabled":  enabled,
		"temp_interval": temp,
		"ph_interval":   ph,
		"turb_interval": turb,
		"tds_interval":  tds,
	})
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}

func (p *CommandPublisher) PublishAutoToggle(ctx context.Context, deviceID string, enabled bool) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	payload, err := json.Marshal(map[string]any{
		"action": "auto_toggle",
		"state":  enabled,
	})
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}
