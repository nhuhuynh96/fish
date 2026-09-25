package advice

import (
	"fmt"
	"strings"
)

func ThresholdsFor(species string) Thresholds {
	var t Thresholds
	switch strings.ToLower(strings.TrimSpace(species)) {
	case "ca_chinh", "chinh", "eel", "anguilla":
		t = Thresholds{TempMin: 26, TempMax: 32, PHMin: 7.0, PHMax: 8.5, TurbidityWarn: 25, TurbidityMax: 50, TDSMin: 100, TDSMax: 800}
	case "discus", "ca_dia":
		t = Thresholds{TempMin: 28, TempMax: 31, PHMin: 6.0, PHMax: 7.0, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 50, TDSMax: 150}
	case "neon", "ca_neon":
		t = Thresholds{TempMin: 24, TempMax: 27, PHMin: 6.0, PHMax: 7.0, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 80, TDSMax: 180}
	case "koi":
		t = Thresholds{TempMin: 18, TempMax: 28, PHMin: 7.0, PHMax: 8.2, TurbidityWarn: 20, TurbidityMax: 35, TDSMin: 150, TDSMax: 400}
	case "ca_vang", "goldfish":
		t = Thresholds{TempMin: 18, TempMax: 24, PHMin: 7.0, PHMax: 8.0, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 120, TDSMax: 300}
	default:
		t = Thresholds{TempMin: 25, TempMax: 29, PHMin: 6.8, PHMax: 7.8, TurbidityWarn: 15, TurbidityMax: 25, TDSMin: 120, TDSMax: 260}
	}
	return withKitDefaults(species, t)
}

func withKitDefaults(species string, t Thresholds) Thresholds {
	switch strings.ToLower(strings.TrimSpace(species)) {
	case "discus", "ca_dia", "neon", "ca_neon":
		t.DOMin, t.TANWarn, t.TANMax, t.NH3FreeWarn, t.NH3FreeMax = 6, 0.25, 0.5, 0.01, 0.02
	default:
		t.DOMin, t.TANWarn, t.TANMax, t.NH3FreeWarn, t.NH3FreeMax = 5, 0.5, 1.5, 0.02, 0.05
	}
	return t
}

func mergeThresholds(base, overlay Thresholds) Thresholds {
	out := overlay
	if out.DOMin == 0 {
		out.DOMin = base.DOMin
	}
	if out.TANWarn == 0 {
		out.TANWarn = base.TANWarn
	}
	if out.TANMax == 0 {
		out.TANMax = base.TANMax
	}
	if out.NH3FreeWarn == 0 {
		out.NH3FreeWarn = base.NH3FreeWarn
	}
	if out.NH3FreeMax == 0 {
		out.NH3FreeMax = base.NH3FreeMax
	}
	return out
}

func pct(v int) *int { return &v }

func applyRules(res *Result) {
	th := res.Thresholds
	trend := res.Trend

	ph := trend["ph"]
	tds := trend["tds"]
	temp := trend["temperature"]
	do := trend["do"]
	tan := trend["tan"]
	nh3 := trend["nh3_free"]

	res.Findings = []Finding{
		findingFor("temperature", "Nhiệt độ", temp, th.TempMin, th.TempMax, "°C"),
		findingFor("ph", "pH", ph, th.PHMin, th.PHMax, ""),
		findingFor("tds", "TDS", tds, th.TDSMin, th.TDSMax, "ppm"),
		findingFor("do", "Oxy (kit)", do, th.DOMin, 20, "mg/L"),
		findingForTAN(tan, th.TANWarn, th.TANMax),
		findingForNH3(nh3, th.NH3FreeWarn, th.NH3FreeMax),
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
	hasKitNH3 := tan.Current != nil || nh3.Current != nil
	hasKitDO := do.Current != nil
	doNot := []string{
		"Không xử lý cả 4 chỉ số cùng lúc — ưu tiên 1 việc rồi đo lại.",
	}
	if !hasKitNH3 {
		doNot = append(doNot, "Chưa nhập NH3/TAN từ test kit: nếu cá nổi đầu, thở gấp thì đo kit rồi nhập form.")
	}

	phLow := ph.Current != nil && *ph.Current < th.PHMin
	phHigh := ph.Current != nil && *ph.Current > th.PHMax
	tdsHigh := tds.Current != nil && *tds.Current > th.TDSMax
	tdsLow := tds.Current != nil && *tds.Current < th.TDSMin
	tempHigh := temp.Current != nil && *temp.Current > th.TempMax
	tempLow := temp.Current != nil && *temp.Current < th.TempMin
	doLow := do.Current != nil && *do.Current < th.DOMin
	tanHigh := tan.Current != nil && *tan.Current > th.TANWarn
	tanDanger := tan.Current != nil && *tan.Current > th.TANMax
	nh3High := nh3.Current != nil && *nh3.Current > th.NH3FreeWarn
	nh3Danger := nh3.Current != nil && *nh3.Current > th.NH3FreeMax

	worseningWater := (phLow && ph.Slope == "falling") ||
		(tdsHigh && tds.Slope == "rising")

	improvingPH := phLow && ph.Slope == "rising"
	improvingTDS := tdsHigh && tds.Slope == "falling"

	if tempHigh || doLow {
		detail := "Nước nóng làm giảm oxy. Hạ nhiệt từ từ (quạt mặt nước, giảm đèn), tăng sục khí. Không đổ đá trực tiếp vào hồ."
		title := "Tăng sục khí / hạ nhiệt"
		if doLow && !tempHigh {
			title = "Tăng sục khí (oxy thấp)"
			detail = fmt.Sprintf("DO kit %.1f mg/L dưới ngưỡng %.0f. Tăng sục khí, giảm cho ăn, không tắt máy sục. Đo kit lại sau 2–4 giờ.", *do.Current, th.DOMin)
		} else if doLow {
			detail = fmt.Sprintf("Nước nóng và DO kit %.1f mg/L. Hạ nhiệt từ từ, tăng sục khí mạnh. Không đổ đá trực tiếp vào hồ.", *do.Current)
		}
		actions = append(actions, Action{
			Type:   "increase_aeration",
			Title:  title,
			Detail: detail,
		})
	}
	if tempLow {
		actions = append(actions, Action{
			Type:   "warm_water",
			Title:  "Tăng nhiệt từ từ",
			Detail: "Bật sưởi, che hồ. Tăng khoảng 1°C/giờ, không hâm nóng đột ngột.",
		})
	}

	needChange := (tdsHigh && tds.Slope == "rising") ||
		(phLow && (tdsHigh || tds.Slope == "rising")) ||
		(phHigh && ph.Slope != "falling") ||
		tanDanger || nh3Danger || (nh3High && tanHigh)

	if needChange {
		amount := 20
		if (ph.Current != nil && (*ph.Current < th.PHMin-0.3 || *ph.Current > th.PHMax+0.4)) ||
			(tds.Current != nil && *tds.Current > th.TDSMax+140) {
			amount = 30
		}
		if improvingPH && improvingTDS {
			amount = 15
		}
		detail := "Thay nước đã khử chlorine, cùng nhiệt độ. Ưu tiên thay nước hơn đổ hóa chất khi pH và TDS đang xấu đi cùng lúc."
		if phLow && tdsHigh {
			detail = "pH thấp đi kèm TDS cao: thường do hữu cơ tích tụ. Thay nước xử lý cả hai; không tăng pH bằng baking soda (TDS sẽ còn tăng)."
		}
		if tanDanger || nh3Danger {
			detail = "Amonia kit cao: thay nước ngay (đã khử chlorine, cùng nhiệt), ngưng cho ăn, tăng sục khí. Không dùng ammonia lock thay cho thay nước."
			amount = 30
		}
		actions = append(actions, Action{
			Type:      "water_change_percent",
			Title:     fmt.Sprintf("Thay %d%% nước", amount),
			Detail:    detail,
			AmountPct: pct(amount),
			Hardware:  map[string]any{"can_auto": true, "drain": true, "inlet": true},
		})
	}

	if (tdsHigh && tds.Slope != "falling") || tanHigh || nh3High {
		feedDetail := "TDS tăng thường do thức ăn thừa. Cho ăn ít hơn, vớt thức ăn không ăn hết."
		if tanHigh || nh3High {
			feedDetail = "Amonia từ kit đang cao: ngưng hoặc giảm mạnh cho ăn 1–2 ngày, vớt thức ăn thừa, tăng sục khí."
		}
		actions = append(actions, Action{
			Type:   "reduce_feeding",
			Title:  "Giảm cho ăn 1–2 ngày",
			Detail: feedDetail,
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
	if improvingPH || improvingTDS {
		doNot = append(doNot, "Chỉ số đang về vùng tốt — đừng chồng thêm hóa chất hôm nay.")
	}
	if nh3Danger || tanDanger {
		doNot = append(doNot, "Không tắt sục khí khi amonia/oxy xấu. Không đổ ammonia lock rồi thôi thay nước.")
	}
	if hasKitDO || hasKitNH3 {
		doNot = append(doNot, "Số Oxi/Amonia là test kit thủ công, không phải cảm biến ESP — nhập lại khi đo kit mới.")
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
	res.Summary = ruleSummary(res, phLow, tdsHigh, improvingPH, improvingTDS, doLow, nh3High || tanHigh)
}

func isDanger(f Finding, th Thresholds) bool {
	if f.Value == nil {
		return false
	}
	v := *f.Value
	switch f.Metric {
	case "ph":
		return v < th.PHMin-0.3 || v > th.PHMax+0.4
	case "tds":
		return v > th.TDSMax+140 || v < th.TDSMin-60
	case "temperature":
		return v > th.TempMax+2 || v < th.TempMin-3
	case "do":
		return v < 3
	case "tan":
		return v > th.TANMax
	case "nh3_free":
		return v > th.NH3FreeMax
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

func findingForTAN(tr MetricTrend, warn, danger float64) Finding {
	f := Finding{Metric: "tan", Value: tr.Current, Target: [2]float64{0, warn}, Slope: tr.Slope, Status: "missing"}
	if tr.Current == nil {
		f.Label = "Chưa nhập TAN (kit amonia)"
		return f
	}
	v := *tr.Current
	switch {
	case v > danger:
		f.Status = "high"
		f.Label = fmt.Sprintf("TAN kit cao (%.2f mg/L)", v)
	case v > warn:
		f.Status = "high"
		f.Label = fmt.Sprintf("TAN kit hơi cao (%.2f mg/L)", v)
	default:
		f.Status = "ok"
		f.Label = fmt.Sprintf("TAN kit ổn (%.2f mg/L)", v)
	}
	return f
}

func findingForNH3(tr MetricTrend, warn, danger float64) Finding {
	f := Finding{Metric: "nh3_free", Value: tr.Current, Target: [2]float64{0, warn}, Slope: tr.Slope, Status: "missing"}
	if tr.Current == nil {
		f.Label = "Chưa ước lượng NH₃ tự do"
		return f
	}
	v := *tr.Current
	switch {
	case v > danger:
		f.Status = "high"
		f.Label = fmt.Sprintf("NH₃ tự do nguy hiểm (%.3f mg/L)", v)
	case v > warn:
		f.Status = "high"
		f.Label = fmt.Sprintf("NH₃ tự do hơi cao (%.3f mg/L)", v)
	default:
		f.Status = "ok"
		f.Label = fmt.Sprintf("NH₃ tự do ổn (%.3f mg/L)", v)
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

func ruleSummary(res *Result, phLow, tdsHigh, improvingPH, improvingTDS, doLow, ammoniaHigh bool) string {
	if ammoniaHigh {
		return "Amonia từ test kit đang cao (TAN và/hoặc NH₃ tự do). Ưu tiên thay nước, ngưng cho ăn, tăng sục khí trước các chỉ số khác."
	}
	if doLow {
		return "Oxy hòa tan (kit) thấp. Tăng sục khí, hạn chế khuấy đáy, đo kit lại sau vài giờ."
	}
	if !res.Ready {
		return "Chưa đủ chuỗi đo để kết luận xu hướng. Bật lịch auto vài giờ rồi phân tích lại. Các đề xuất dưới đây chỉ dựa trên điểm hiện tại, mức tin cậy thấp."
	}
	if res.Overall == "ok" {
		return "Các chỉ số trong ngưỡng và không xấu đi rõ. Giữ lịch đo, chưa cần can thiệp."
	}
	if improvingPH && improvingTDS {
		return "Hồ đang xấu nhưng xu hướng đang cải thiện. Ưu tiên theo dõi, thay nước nhẹ nếu TDS vẫn cao — tránh chồng hóa chất."
	}
	if phLow && tdsHigh {
		return "pH thấp đi kèm TDS cao: thay nước từng phần, đừng dùng baking soda."
	}
	if phLow {
		return "pH đang thấp. Nếu khoáng không cao có thể tăng KH rất chậm; nếu TDS đang tăng thì chỉ thay nước."
	}
	if res.Overall == "danger" {
		return "Có chỉ số lệch mạnh. Ưu tiên thay nước và ổn định nhiệt/oxy trước khi thêm chế phẩm."
	}
	return "Một số chỉ số lệch ngưỡng hoặc đang trôi theo hướng xấu. Xem thứ tự đề xuất bên dưới."
}
