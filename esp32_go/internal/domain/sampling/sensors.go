package sampling

import "strings"

func ParseSensorName(name string) []SensorType {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "all", "tat_ca":
		return []SensorType{SensorPH, SensorTurbidity, SensorTDS}
	case "temp", "temperature", "nhiet_do":
		return []SensorType{SensorTemp}
	case "ph":
		return []SensorType{SensorPH}
	case "turbidity", "turb", "do_duc", "do_can":
		return []SensorType{SensorTurbidity}
	case "tds", "chat_ran", "chat_luong":
		return []SensorType{SensorTDS}
	default:
		return nil
	}
}

func ExpandSensorTokens(sensors []string) []SensorType {
	var out []SensorType
	for _, raw := range sensors {
		for _, part := range strings.Split(raw, ",") {
			out = append(out, ParseSensorName(part)...)
		}
	}
	seen := map[SensorType]bool{}
	unique := make([]SensorType, 0, len(out))
	for _, t := range out {
		if seen[t] {
			continue
		}
		seen[t] = true
		unique = append(unique, t)
	}
	return unique
}
