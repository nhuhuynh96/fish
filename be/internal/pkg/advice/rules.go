package advice

import (
	"fmt"
	"strings"
)

func ThresholdsFor(species string) Thresholds {
	switch strings.ToLower(strings.TrimSpace(species)) {
	case "ca_chinh", "chinh", "eel", "anguilla":
		// Hồ nuôi cá chình (nước ngọt / hơi lợ): rộng hơn hồ cá cảnh
		return Thresholds{TempMin: 26, TempMax: 32, PHMin: 7.0, PHMax: 8.5, TurbidityWarn: 25, TurbidityMax: 50, TDSMin: 100, TDSMax: 800}
	case "discus", "ca_dia":
		return Thresholds{TempMin: 28, TempMax: 31, PHMin: 6.0, PHMax: 7.0, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 50, TDSMax: 150}
	case "neon", "ca_neon":
		return Thresholds{TempMin: 24, TempMax: 27, PHMin: 6.0, PHMax: 7.0, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 80, TDSMax: 180}
	case "koi":
		return Thresholds{TempMin: 18, TempMax: 28, PHMin: 7.0, PHMax: 8.2, TurbidityWarn: 20, TurbidityMax: 35, TDSMin: 150, TDSMax: 400}
	case "ca_vang", "goldfish":
		return Thresholds{TempMin: 18, TempMax: 24, PHMin: 7.0, PHMax: 8.0, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 120, TDSMax: 300}
	default:
		return Thresholds{TempMin: 25, TempMax: 29, PHMin: 6.8, PHMax: 7.8, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 120, TDSMax: 260}
	}
}

func pct(v int) *int { return &v }

func applyRules(res *Result) {
	th := res.Thresholds
	trend := res.Trend
	hasFilter := false
	if res.Profile.HasFilter != nil {
		hasFilter = *res.Profile.HasFilter
	}

	ph := trend["ph"]
	tds := trend["tds"]
	turb := trend["turbidity"]
	temp := trend["temperature"]

	res.Findings = []Finding{
		findingFor("temperature", "Nhiệt độ", temp, th.TempMin, th.TempMax, "°C"),
		findingFor("ph", "pH", ph, th.PHMin, th.PHMax, ""),
		findingForTurbidity(turb, th.TurbidityWarn, th.TurbidityMax),
		findingFor("tds", "TDS", tds, th.TDSMin, th.TDSMax, "ppm"),
	}

	overall := "ok"
	for _, f := range res.Findings {
		if f.Status == "missing" {
			continue
		}
		if isDanger(f, th) {
			overall = "danger"
			break
		}
		if f.Status != "ok" && overall == "ok" {
			overall = "warning"
		}
		if f.Status == "ok" && (f.Slope == "rising" || f.Slope == "falling") && driftingAway(f, th) && overall == "ok" {
			overall = "warning"
		}
	}
	res.Overall = overall

	actions := make([]Action, 0, 6)
	doNot := []string{
		"Không xử lý cả 4 chỉ số cùng lúc — ưu tiên 1 việc rồi đo lại.",
		"Chưa đo NH3/NO2/NO3: nếu cá nổi đầu, thở gấp thì test kit riêng.",
	}

	phLow := ph.Current != nil && *ph.Current < th.PHMin
	phHigh := ph.Current != nil && *ph.Current > th.PHMax
	tdsHigh := tds.Current != nil && *tds.Current > th.TDSMax
	tdsLow := tds.Current != nil && *tds.Current < th.TDSMin
	turbHigh := turb.Current != nil && *turb.Current > th.TurbidityWarn
	turbDanger := turb.Current != nil && *turb.Current > th.TurbidityMax
	tempHigh := temp.Current != nil && *temp.Current > th.TempMax
	tempLow := temp.Current != nil && *temp.Current < th.TempMin

	worseningWater := (phLow && ph.Slope == "falling") ||
		(tdsHigh && tds.Slope == "rising") ||
		(turbHigh && (turb.Slope == "rising" || turb.Slope == "stable"))

	improvingPH := phLow && ph.Slope == "rising"
	improvingTDS := tdsHigh && tds.Slope == "falling"
	improvingTurb := turbHigh && turb.Slope == "falling"

	if tempHigh {
		actions = append(actions, Action{
			Type:   "increase_aeration",
			Title:  "Tăng sục khí / hạ nhiệt",
			Detail: "Nước nóng làm giảm oxy. Hạ nhiệt từ từ (quạt mặt nước, giảm đèn), tăng sục khí. Không đổ đá trực tiếp vào hồ.",
		})
	}
	if tempLow {
		actions = append(actions, Action{
			Type:   "warm_water",
			Title:  "Tăng nhiệt từ từ",
			Detail: "Bật sưởi, che hồ. Tăng khoảng 1°C/giờ, không hâm nóng đột ngột.",
		})
	}

	needChange := turbDanger || (turbHigh && turb.Slope == "rising") ||
		(tdsHigh && tds.Slope == "rising") ||
		(phLow && (tdsHigh || tds.Slope == "rising")) ||
		(phHigh && ph.Slope != "falling")

	if needChange {
		amount := 20
		if turbDanger || (ph.Current != nil && (*ph.Current < th.PHMin-0.3 || *ph.Current > th.PHMax+0.4)) ||
			(tds.Current != nil && *tds.Current > th.TDSMax+140) {
			amount = 30
		}
		if improvingPH && improvingTDS && improvingTurb && !turbDanger {
			amount = 15
		}
		detail := "Thay nước đã khử chlorine, cùng nhiệt độ. Ưu tiên thay nước hơn đổ hóa chất khi pH/TDS/độ đục đang xấu đi cùng lúc."
		if phLow && tdsHigh {
			detail = "pH thấp đi kèm TDS cao: thường do hữu cơ tích tụ. Thay nước xử lý cả hai; không tăng pH bằng baking soda (TDS sẽ còn tăng)."
		}
		if turbHigh && tdsHigh {
			detail = "Độ đục và TDS cùng tăng: giảm cho ăn, hút cặn, thay nước. Kiểm tra lọc cơ học."
		}
		actions = append(actions, Action{
			Type:      "water_change_percent",
			Title:     fmt.Sprintf("Thay %d%% nước", amount),
			Detail:    detail,
			AmountPct: pct(amount),
			Hardware:  map[string]any{"can_auto": true, "drain": true, "inlet": true},
		})
	}

	if turbHigh {
		filterMsg := "Vệ sinh bể lắng/lọc, hút cặn đáy. Sục khí không thay được lọc."
		if !hasFilter {
			filterMsg = "Hồ không có hệ thống lọc — hút cặn, thay nước, tăng sục khí. Lọc ở đây là bể lọc/lắng, không phải máy sục."
		}
		actions = append(actions, Action{
			Type:   "check_filter",
			Title:  "Kiểm tra lọc và hút cặn",
			Detail: filterMsg,
		})
	}

	if (tdsHigh && tds.Slope != "falling") || (turbHigh && tds.Slope == "rising") {
		actions = append(actions, Action{
			Type:   "reduce_feeding",
			Title:  "Giảm cho ăn 1–2 ngày",
			Detail: "TDS/độ đục tăng thường do thức ăn thừa. Cho ăn ít hơn, vớt thức ăn không ăn hết.",
		})
	}

	allowKH := phLow && !tdsHigh && (tdsLow || tds.Current == nil || tds.Slope != "rising")
	if allowKH && !improvingPH {
		detail := "pH thấp và khoáng không cao: có thể tăng KH rất chậm (buffer / baking soda rất ít). Đo lại sau 2–4 giờ. Không tăng quá 0.3 pH/ngày."
		if res.Profile.VolumeL <= 0 {
			detail += " Chưa có thể tích hồ nên không nêu số gram — chỉ thay nước hoặc dùng liều ghi trên bao bì theo số lít."
		}
		actions = append(actions, Action{
			Type:   "add_kh_buffer",
			Title:  "Tăng KH / pH rất chậm",
			Detail: detail,
		})
	}

	if tdsLow && tds.Slope != "rising" {
		actions = append(actions, Action{
			Type:   "add_gh_mineral",
			Title:  "Bổ sung khoáng (GH)",
			Detail: "TDS thấp: nước quá mềm, pH dễ sập. Thêm khoáng cá cảnh / trộn nước máy, không đổ muối ăn.",
		})
	}

	if phHigh && !needChange {
		actions = append(actions, Action{
			Type:      "water_change_percent",
			Title:     "Thay 15% nước",
			Detail:    "pH cao: thay nước nguồn thấp pH hơn. Có thể dùng gỗ lũa/lá bàng về sau. Không dùng axit mạnh.",
			AmountPct: pct(15),
			Hardware:  map[string]any{"can_auto": true, "drain": true, "inlet": true},
		})
	}

	if phLow && tdsHigh {
		doNot = append(doNot, "Không đổ baking soda / pH Up khi TDS đang cao.")
	}
	if improvingPH || improvingTDS || improvingTurb {
		doNot = append(doNot, "Chỉ số đang về vùng tốt — đừng chồng thêm hóa chất hôm nay.")
	}
	doNot = append(doNot, "Không tắt lọc và không trộn nhiều chế phẩm cùng lúc.")

	if len(actions) == 0 {
		if worseningWater {
			actions = append(actions, Action{
				Type:   "monitor",
				Title:  "Theo dõi sát 6 giờ tới",
				Detail: "Chưa lệch ngưỡng nhiều nhưng đang trôi theo hướng xấu. Giữ lịch đo auto.",
			})
		} else {
			actions = append(actions, Action{
				Type:   "nothing",
				Title:  "Giữ nguyên, đo định kỳ",
				Detail: "Các chỉ số trong ngưỡng (hoặc đang ổn định). Tiếp tục lịch đo tự động.",
			})
		}
	}

	for i := range actions {
		actions[i].Priority = i + 1
	}
	res.Actions = actions
	res.DoNot = doNot
	res.Summary = ruleSummary(res, phLow, tdsHigh, turbHigh, improvingPH, improvingTDS, improvingTurb)
}

func isDanger(f Finding, th Thresholds) bool {
	if f.Value == nil {
		return false
	}
	v := *f.Value
	switch f.Metric {
	case "ph":
		return v < th.PHMin-0.3 || v > th.PHMax+0.4
	case "turbidity":
		return v > th.TurbidityMax
	case "tds":
		return v > th.TDSMax+140 || v < th.TDSMin-60
	case "temperature":
		return v > th.TempMax+2 || v < th.TempMin-3
	}
	return false
}

func driftingAway(f Finding, th Thresholds) bool {
	if f.Value == nil {
		return false
	}
	v := *f.Value
	switch f.Metric {
	case "ph":
		return (v <= th.PHMin+0.15 && f.Slope == "falling") || (v >= th.PHMax-0.15 && f.Slope == "rising")
	case "tds":
		return (v >= th.TDSMax-20 && f.Slope == "rising") || (v <= th.TDSMin+20 && f.Slope == "falling")
	case "turbidity":
		return v >= th.TurbidityWarn-3 && f.Slope == "rising"
	case "temperature":
		return (v >= th.TempMax-0.5 && f.Slope == "rising") || (v <= th.TempMin+0.5 && f.Slope == "falling")
	}
	return false
}

func findingFor(metric, label string, tr MetricTrend, min, max float64, unit string) Finding {
	f := Finding{Metric: metric, Value: tr.Current, Target: [2]float64{min, max}, Slope: tr.Slope, Status: "missing"}
	if tr.Current == nil {
		f.Label = "Chưa có mẫu " + label
		return f
	}
	v := *tr.Current
	switch {
	case v < min:
		f.Status = "low"
		f.Label = fmt.Sprintf("%s thấp (%.2f%s)", label, v, unitSpace(unit))
	case v > max:
		f.Status = "high"
		f.Label = fmt.Sprintf("%s cao (%.2f%s)", label, v, unitSpace(unit))
	default:
		f.Status = "ok"
		f.Label = fmt.Sprintf("%s ổn (%.2f%s)", label, v, unitSpace(unit))
	}
	if tr.Slope == "rising" || tr.Slope == "falling" {
		f.Label += slopeHint(tr.Slope)
	}
	return f
}

func findingForTurbidity(tr MetricTrend, warn, danger float64) Finding {
	f := Finding{Metric: "turbidity", Value: tr.Current, Target: [2]float64{0, warn}, Slope: tr.Slope, Status: "missing"}
	if tr.Current == nil {
		f.Label = "Chưa có mẫu độ đục"
		return f
	}
	v := *tr.Current
	switch {
	case v > danger:
		f.Status = "high"
		f.Label = fmt.Sprintf("Nước đục cao (%.1f NTU)", v)
	case v > warn:
		f.Status = "high"
		f.Label = fmt.Sprintf("Hơi đục (%.1f NTU)", v)
	default:
		f.Status = "ok"
		f.Label = fmt.Sprintf("Nước trong (%.1f NTU)", v)
	}
	if tr.Slope == "rising" || tr.Slope == "falling" {
		f.Label += slopeHint(tr.Slope)
	}
	return f
}

func unitSpace(unit string) string {
	if unit == "" {
		return ""
	}
	return " " + unit
}

func slopeHint(slope string) string {
	if slope == "rising" {
		return " · đang tăng"
	}
	if slope == "falling" {
		return " · đang giảm"
	}
	return ""
}

func ruleSummary(res *Result, phLow, tdsHigh, turbHigh, improvingPH, improvingTDS, improvingTurb bool) string {
	if !res.Ready {
		return "Chưa đủ chuỗi đo để kết luận xu hướng. Bật lịch auto vài giờ rồi phân tích lại. Các đề xuất dưới đây chỉ dựa trên điểm hiện tại, mức tin cậy thấp."
	}
	if res.Overall == "ok" {
		return "Các chỉ số trong ngưỡng và không xấu đi rõ. Giữ lịch đo, chưa cần can thiệp."
	}
	if improvingPH && improvingTDS && improvingTurb {
		return "Hồ đang xấu nhưng xu hướng đang cải thiện. Ưu tiên theo dõi, thay nước nhẹ nếu vẫn đục/TDS cao — tránh chồng hóa chất."
	}
	if phLow && tdsHigh && turbHigh {
		return "pH thấp, TDS và độ đục cùng cao/đang tăng: ưu tiên thay nước và giảm cho ăn, không tăng pH bằng hóa chất."
	}
	if phLow && tdsHigh {
		return "pH thấp đi kèm TDS cao: thay nước từng phần, đừng dùng baking soda."
	}
	if turbHigh && tdsHigh {
		return "Nước đục và TDS cao: thay nước, hút cặn, kiểm tra lọc, giảm cho ăn."
	}
	if phLow {
		return "pH đang thấp. Nếu khoáng không cao có thể tăng KH rất chậm; nếu TDS đang tăng thì chỉ thay nước."
	}
	if res.Overall == "danger" {
		return "Có chỉ số lệch mạnh. Ưu tiên thay nước và ổn định nhiệt/oxy trước khi thêm chế phẩm."
	}
	return "Một số chỉ số lệch ngưỡng hoặc đang trôi theo hướng xấu. Xem thứ tự đề xuất bên dưới."
}
