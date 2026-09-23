package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

type CommandPublisher struct {
	client *Client
}

func NewCommandPublisher(client *Client) *CommandPublisher {
	return &CommandPublisher{client: client}
}

var _ fish.CommandPublisher = (*CommandPublisher)(nil)

func (p *CommandPublisher) publishCommand(deviceID, action string, level int) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	body := map[string]any{
		"action": action,
	}
	if action == "inlet_on" {
		if level != 1 {
			level = 2
		}
		body["level"] = level
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}

func (p *CommandPublisher) PublishMeasure(ctx context.Context, deviceID string, sensors []string) error {
	_ = ctx
	actions := expandMeasureActions(sensors)
	if len(actions) == 0 {
		return fmt.Errorf("no supported sensors in %v", sensors)
	}
	for _, action := range actions {
		if err := p.publishCommand(deviceID, action, 0); err != nil {
			return err
		}
	}
	return nil
}

func expandMeasureActions(sensors []string) []string {
	if len(sensors) == 0 {
		sensors = []string{"all"}
	}
	seen := map[string]bool{}
	var out []string
	add := func(action string) {
		if seen[action] {
			return
		}
		seen[action] = true
		out = append(out, action)
	}
	for _, raw := range sensors {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "all", "tat_ca":
			add("ph")
			add("turb")
			add("tds")
		case "ph":
			add("ph")
		case "turbidity", "turb", "do_duc", "do_can":
			add("turb")
		case "tds", "chat_ran", "chat_luong":
			add("tds")
		}
	}
	return out
}

func (p *CommandPublisher) PublishPump(ctx context.Context, deviceID string, target string, state bool, level int) error {
	_ = ctx
	if target == "drain" {
		if state {
			return p.publishCommand(deviceID, "drain_on", 0)
		}
		return p.publishCommand(deviceID, "drain_off", 0)
	}
	if !state {
		return p.publishCommand(deviceID, "inlet_off", 0)
	}
	if level != 1 {
		level = 2
	}
	return p.publishCommand(deviceID, "inlet_on", level)
}

func (p *CommandPublisher) PublishClearQueue(ctx context.Context, deviceID string) error {
	_ = ctx
	return p.publishCommand(deviceID, "clear_queue", 0)
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
