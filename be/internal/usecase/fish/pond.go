package fishuc

import (
	"context"
	"sort"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

// Số đo của hồ nằm ở hai bo: sensor (nhiệt độ DS18B20, TDS) và ph (pH và nhiệt độ RS485).
var pondDevices = []string{sensorDeviceID, phDeviceID}

// Nhiệt độ cảm biến pH cũ hơn mức này thì lấy DS18B20 làm nhiệt độ nước.
const phTempMaxAge = 2 * time.Hour

const (
	tempSourcePH      = "ph"
	tempSourceDS18B20 = "ds18b20"
)

// Gán nhiệt độ nước cho từng bản ghi: bản ghi bo ph lấy ph_temp; DS18B20 chỉ dùng khi
// cả danh sách không có ph_temp, để biểu đồ không trộn hai nguồn.
func withWaterTemp(list []fish.Measurement) []fish.Measurement {
	hasPHTemp := false
	for i := range list {
		if list[i].PHTemp != nil {
			hasPHTemp = true
			break
		}
	}
	for i := range list {
		m := &list[i]
		m.WaterTemp, m.TempSource = nil, ""
		switch {
		case m.PHTemp != nil:
			m.WaterTemp, m.TempSource = m.PHTemp, tempSourcePH
		case !hasPHTemp && m.Temperature != nil:
			m.WaterTemp, m.TempSource = m.Temperature, tempSourceDS18B20
		}
	}
	return list
}

func isPondDevice(id string) bool {
	for _, d := range pondDevices {
		if d == id {
			return true
		}
	}
	return false
}

func sortNewestFirst(list []fish.Measurement, limit int) []fish.Measurement {
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

// Dùng cho phân tích: Temperature của bản sao được thay bằng nhiệt độ nước, DB không đổi.
func (s *Service) listPondSince(ctx context.Context, since time.Time, limit int) ([]fish.Measurement, error) {
	var all []fish.Measurement
	for _, id := range pondDevices {
		list, err := s.repo.ListMeasurementsSince(ctx, id, since, limit)
		if err != nil {
			return nil, err
		}
		all = append(all, list...)
	}
	all = withWaterTemp(sortNewestFirst(all, limit))
	for i := range all {
		all[i].Temperature = all[i].WaterTemp
	}
	return all, nil
}

// ListPondHistory gộp lịch sử của mọi bo đo, mới nhất trước.
func (s *Service) ListPondHistory(ctx context.Context, limit int) ([]fish.Measurement, error) {
	if limit <= 0 {
		limit = 20
	}
	var all []fish.Measurement
	for _, id := range pondDevices {
		list, err := s.repo.ListMeasurements(ctx, id, limit)
		if err != nil {
			return nil, err
		}
		all = append(all, list...)
	}
	return withWaterTemp(sortNewestFirst(all, limit)), nil
}

// GetPondLatest lấy TDS và nhiệt độ DS18B20 mới nhất từ bo sensor, pH và nhiệt độ cảm biến pH từ bo ph.
// Nhiệt độ nước ưu tiên cảm biến pH; quá phTempMaxAge hoặc không có thì dùng DS18B20.
func (s *Service) GetPondLatest(ctx context.Context) (*fish.Measurement, error) {
	base, err := s.repo.GetLatestMeasurement(ctx, sensorDeviceID)
	if err != nil {
		return nil, err
	}
	ph, err := s.repo.GetLatestMeasurement(ctx, phDeviceID)
	if err != nil {
		return nil, err
	}
	if base == nil && ph == nil {
		return nil, nil
	}

	// Bo ph có pH, nhiệt độ và TDS; bo sensor cũ chỉ còn làm dự phòng cho trường bo ph chưa có.
	var merged fish.Measurement
	switch {
	case base == nil:
		merged = *ph
	case ph == nil:
		merged = *base
	default:
		merged = *base
		if ph.PH != nil {
			merged.PH, merged.PHRaw, merged.PHAdc, merged.PHVoltage = ph.PH, ph.PHRaw, nil, nil
		}
		if ph.PHTemp != nil {
			merged.PHTemp = ph.PHTemp
		}
		if ph.TDS != nil {
			merged.TDS, merged.TDSAdc, merged.TDSVoltage = ph.TDS, ph.TDSAdc, ph.TDSVoltage
		}
		if ph.CreatedAt.After(merged.CreatedAt) {
			merged.CreatedAt = ph.CreatedAt
		}
		for _, name := range ph.Sensors {
			if !contains(merged.Sensors, name) {
				merged.Sensors = append(append([]string{}, merged.Sensors...), name)
			}
		}
	}

	phFresh := ph != nil && ph.PHTemp != nil && time.Since(ph.CreatedAt) <= phTempMaxAge
	switch {
	case phFresh || (merged.PHTemp != nil && merged.Temperature == nil):
		merged.WaterTemp, merged.TempSource = merged.PHTemp, tempSourcePH
	case merged.Temperature != nil:
		merged.WaterTemp, merged.TempSource = merged.Temperature, tempSourceDS18B20
	}
	return &merged, nil
}
