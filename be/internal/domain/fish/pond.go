package fish

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const DefaultPondConfigID = "default"

type WaterThresholds struct {
	TempMin       float64 `json:"temp_min"`
	TempMax       float64 `json:"temp_max"`
	PHMin         float64 `json:"ph_min"`
	PHMax         float64 `json:"ph_max"`
	TurbidityWarn float64 `json:"turbidity_warn"`
	TurbidityMax  float64 `json:"turbidity_max"`
	TDSMin        float64 `json:"tds_min"`
	TDSMax        float64 `json:"tds_max"`
}

type MetricBadges struct {
	OK   string `json:"ok"`
	Low  string `json:"low,omitempty"`
	Warn string `json:"warn,omitempty"`
	High string `json:"high"`
}

type PondConfig struct {
	ID         string                  `json:"id"`
	Species    string                  `json:"species"`
	VolumeL    float64                 `json:"volume_l"`
	VolumeM3   float64                 `json:"volume_m3"`
	HasFilter  bool                    `json:"has_filter"`
	Thresholds WaterThresholds         `json:"thresholds"`
	Badges     map[string]MetricBadges `json:"badges"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

func DefaultPondConfig() *PondConfig {
	c := &PondConfig{
		ID:        DefaultPondConfigID,
		Species:   "ca_chinh",
		VolumeL:   9000,
		HasFilter: false,
		Thresholds: WaterThresholds{
			TempMin: 26, TempMax: 32,
			PHMin: 7.0, PHMax: 8.5,
			TurbidityWarn: 25, TurbidityMax: 50,
			TDSMin: 100, TDSMax: 800,
		},
		UpdatedAt: time.Now(),
	}
	return c.Normalize()
}

func (c *PondConfig) Normalize() *PondConfig {
	if c == nil {
		return DefaultPondConfig()
	}
	if c.ID == "" {
		c.ID = DefaultPondConfigID
	}
	c.Species = strings.ToLower(strings.TrimSpace(c.Species))
	if c.Species == "" {
		c.Species = "ca_chinh"
	}
	if c.VolumeL <= 0 {
		c.VolumeL = 9000
	}
	c.VolumeM3 = c.VolumeL / 1000
	if c.Thresholds == (WaterThresholds{}) {
		c.Thresholds = DefaultPondConfig().Thresholds
	}
	c.Badges = BuildBadges(c.Species, c.Thresholds)
	return c
}

func speciesTitle(species string) string {
	switch strings.ToLower(species) {
	case "ca_chinh", "chinh", "eel", "anguilla":
		return "chình"
	case "koi":
		return "koi"
	case "ca_vang", "goldfish":
		return "cá vàng"
	case "discus", "ca_dia":
		return "cá đĩa"
	case "neon", "ca_neon":
		return "neon"
	default:
		return "cá"
	}
}

func BuildBadges(species string, t WaterThresholds) map[string]MetricBadges {
	title := speciesTitle(species)
	return map[string]MetricBadges{
		"temperature": {
			OK:   fmt.Sprintf("Lý tưởng %s (%.0f–%.0f°C)", title, t.TempMin, t.TempMax),
			Low:  fmt.Sprintf("Hơi lạnh (<%.0f°C)", t.TempMin),
			High: fmt.Sprintf("Nóng (>%.0f°C)", t.TempMax),
		},
		"ph": {
			OK:   fmt.Sprintf("Chuẩn hồ %s (%.1f–%.1f)", title, t.PHMin, t.PHMax),
			Low:  fmt.Sprintf("Nhiễm axit (<%.1f)", t.PHMin),
			High: fmt.Sprintf("Nhiễm kiềm (>%.1f)", t.PHMax),
		},
		"turbidity": {
			OK:   fmt.Sprintf("Nước trong (<%.0f NTU)", t.TurbidityWarn),
			Warn: fmt.Sprintf("Hơi đục (%.0f–%.0f NTU)", t.TurbidityWarn, t.TurbidityMax),
			High: fmt.Sprintf("Nước đục cao (>%.0f NTU)", t.TurbidityMax),
		},
		"tds": {
			OK:   fmt.Sprintf("Khoáng hồ %s (%.0f–%.0f ppm)", title, t.TDSMin, t.TDSMax),
			Low:  fmt.Sprintf("Khoáng thấp (<%.0f ppm)", t.TDSMin),
			High: fmt.Sprintf("Khoáng/TDS cao (>%.0f ppm)", t.TDSMax),
		},
	}
}

type PondConfigRepository interface {
	GetPondConfig(ctx context.Context) (*PondConfig, error)
	SavePondConfig(ctx context.Context, cfg *PondConfig) error
}
