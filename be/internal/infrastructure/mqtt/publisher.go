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

func (p *CommandPublisher) publish(deviceID string, body map[string]any) error {
	topic := fmt.Sprintf("fish/%s/command", deviceID)
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return p.client.Publish(topic, payload)
}

func (p *CommandPublisher) publishCommand(deviceID, action string) error {
	return p.publish(deviceID, map[string]any{"action": action})
}

func (p *CommandPublisher) PublishLevel(ctx context.Context, deviceID string) error {
	_ = ctx
	return p.publishCommand(deviceID, "float")
}

// sensor "ph" | "temp" | "tds" | "all" (chỉ tắt). maxOnS 0 = mặc định của firmware.
func (p *CommandPublisher) PublishSensorPower(ctx context.Context, deviceID string, sensor string, on bool, maxOnS int) error {
	_ = ctx
	body := map[string]any{"action": "power", "sensor": sensor, "state": on}
	if on && maxOnS > 0 {
		body["max_on_s"] = maxOnS
	}
	return p.publish(deviceID, body)
}

// Chỉ đọc, firmware không bật tắt nguồn. samples và intervalMs 0 = mặc định của firmware.
func (p *CommandPublisher) PublishRead(ctx context.Context, deviceID string, sensor string, samples, intervalMs int) error {
	_ = ctx
	body := map[string]any{"action": sensor}
	if samples > 0 {
		body["samples"] = samples
	}
	if intervalMs > 0 {
		body["interval_ms"] = intervalMs
	}
	return p.publish(deviceID, body)
}

// Luôn gửi until_full / until_empty vì firmware mặc định until_full = true. duration 0 = mặc định 60 giây của firmware.
func (p *CommandPublisher) PublishPump(ctx context.Context, deviceID string, target string, state bool, untilFloat bool, duration int) error {
	_ = ctx
	prefix := "inlet"
	untilKey := "until_full"
	if target == "drain" {
		prefix = "drain"
		untilKey = "until_empty"
	}
	if !state {
		return p.publishCommand(deviceID, prefix+"_off")
	}
	body := map[string]any{"action": prefix + "_on", untilKey: untilFloat}
	if duration > 0 {
		body["duration"] = duration
	}
	return p.publish(deviceID, body)
}

func (p *CommandPublisher) PublishClearQueue(ctx context.Context, deviceID string) error {
	_ = ctx
	return p.publishCommand(deviceID, "stop")
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
