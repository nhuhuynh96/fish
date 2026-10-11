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
		t.NO2Warn, t.NO2Max, t.NO3Warn, t.NO3Max = 0.1, 0.5, 20, 50
	default:
		t.DOMin, t.TANWarn, t.TANMax, t.NH3FreeWarn, t.NH3FreeMax = 5, 0.5, 1.5, 0.02, 0.05
		t.NO2Warn, t.NO2Max, t.NO3Warn, t.NO3Max = 0.25, 1.0, 50, 100
	}
	t.KHMin = 4
	return t
}

// RAS chạy mật độ cao, nitrate tích dần và cần độ kiềm cho lọc sinh học.
func withSystemDefaults(system string, t Thresholds) Thresholds {
	if system != "ras" {
		return t
	}
	t.KHMin = 6 // khoảng 107 mg/L CaCO3
	if t.NO3Warn < 100 {
		t.NO3Warn, t.NO3Max = 100, 200
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
	if out.NO2Warn == 0 {
		out.NO2Warn = base.NO2Warn
	}
	if out.NO2Max == 0 {
		out.NO2Max = base.NO2Max
	}
	if out.NO3Warn == 0 {
		out.NO3Warn = base.NO3Warn
	}
	if out.NO3Max == 0 {
		out.NO3Max = base.NO3Max
	}
	if out.KHMin == 0 {
		out.KHMin = base.KHMin
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
	no2 := trend["no2"]
	no3 := trend["no3"]
	kh := trend["kh"]
	ras := res.Profile.IsRAS()

	res.Findings = []Finding{
		findingFor("temperature", "Nhiệt độ", temp, th.TempMin, th.TempMax, "°C"),
		findingFor("ph", "pH", ph, th.PHMin, th.PHMax, ""),
		findingFor("tds", "TDS", tds, th.TDSMin, th.TDSMax, "ppm"),
		findingFor("do", "Oxy (kit)", do, th.DOMin, 20, "mg/L"),
		findingForTAN(tan, th.TANWarn, th.TANMax),
		findingForNH3(nh3, th.NH3FreeWarn, th.NH3FreeMax),
	}
	if ras || no2.Current != nil {
		res.Findings = append(res.Findings, findingForUpper("no2", "Nitrite NO₂ (kit)", no2, th.NO2Warn, th.NO2Max))
	}
	if ras || no3.Current != nil {
		res.Findings = append(res.Findings, findingForUpper("no3", "Nitrate NO₃ (kit)", no3, th.NO3Warn, th.NO3Max))
	}
	if ras || kh.Current != nil {
		res.Findings = append(res.Findings, findingFor("kh", "KH (kit)", kh, th.KHMin, 30, "dKH"))
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
	no2High := no2.Current != nil && *no2.Current > th.NO2Warn
	no2Danger := no2.Current != nil && *no2.Current > th.NO2Max
	no3High := no3.Current != nil && *no3.Current > th.NO3Warn
	no3Danger := no3.Current != nil && *no3.Current > th.NO3Max
	khLow := kh.Current != nil && *kh.Current < th.KHMin

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

	// RAS: pH tụt và TDS tăng dần là chuyện vận hành bình thường, chỉ thay nước gấp khi amonia/nitrite nguy hiểm.
	toxicDanger := tanDanger || nh3Danger || no2Danger || (nh3High && tanHigh)
	needChange := toxicDanger
	if !ras {
		needChange = needChange || (tdsHigh && tds.Slope == "rising") ||
			(phLow && (tdsHigh || tds.Slope == "rising")) ||
			(phHigh && ph.Slope != "falling") || no3Danger
	}

	if needChange {
		amount := 20
		if !ras && ((ph.Current != nil && (*ph.Current < th.PHMin-0.3 || *ph.Current > th.PHMax+0.4)) ||
			(tds.Current != nil && *tds.Current > th.TDSMax+140)) {
			amount = 30
		}
		if !ras && improvingPH && improvingTDS {
			amount = 15
		}
		detail := "Thay nước đã khử chlorine, cùng nhiệt độ. Ưu tiên thay nước hơn đổ hóa chất khi pH và TDS đang xấu đi cùng lúc."
		if phLow && tdsHigh && !ras {
			detail = "pH thấp đi kèm TDS cao: thường do hữu cơ tích tụ. Thay nước xử lý cả hai; không tăng pH bằng baking soda (TDS sẽ còn tăng)."
		}
		if no3Danger && !ras {
			detail = "Nitrate cao: thay nước từng phần, giảm cho ăn. Nitrate chỉ giảm bằng thay nước hoặc cây thủy sinh."
		}
		if no2Danger {
			detail = "Nitrite kit cao: thay nước (đã khử chlorine, cùng nhiệt), giảm mạnh cho ăn, tăng oxy. Nitrite làm cá thiếu oxy trong máu."
		}
		if tanDanger || nh3Danger {
			detail = "Amonia kit cao: thay nước ngay (đã khử chlorine, cùng nhiệt), ngưng cho ăn, tăng sục khí. Không dùng ammonia lock thay cho thay nước."
			amount = 30
		}
		if ras {
			detail += " RAS: không thay quá 30% một lần để lọc sinh học không bị sốc."
		}
		actions = append(actions, Action{
			Type:      "water_change_percent",
			Title:     fmt.Sprintf("Thay %d%% nước", amount),
			Detail:    detail,
			AmountPct: pct(amount),
		})
	}

	if no2High || (ras && tanHigh) {
		detail := "Lọc sinh học chưa xử lý kịp amonia/nitrite."
		if no2High {
			detail = fmt.Sprintf("Nitrite %.2f mg/L: vi khuẩn lọc sinh học chưa chuyển kịp sang nitrate.", *no2.Current)
		}
		detail += " Giảm cho ăn 30–50%, tăng oxy cho bể lọc, giữ KH đủ. Không rửa giá thể lọc bằng nước máy. Có thể bổ sung muối (chloride) theo khuyến cáo để giảm độc nitrite."
		actions = append(actions, Action{
			Type:   "check_biofilter",
			Title:  "Kiểm tra lọc sinh học",
			Detail: detail,
		})
	}

	feedBecauseTDS := !ras && tdsHigh && tds.Slope != "falling"
	if feedBecauseTDS || tanHigh || nh3High || no2High {
		feedDetail := "TDS tăng thường do thức ăn thừa. Cho ăn ít hơn, vớt thức ăn không ăn hết."
		if no2High {
			feedDetail = "Nitrite đang cao: giảm cho ăn tới khi nitrite về dưới ngưỡng, vớt thức ăn thừa."
		}
		if tanHigh || nh3High {
			feedDetail = "Amonia từ kit đang cao: ngưng hoặc giảm mạnh cho ăn 1–2 ngày, vớt thức ăn thừa, tăng sục khí."
		}
		actions = append(actions, Action{
			Type:   "reduce_feeding",
			Title:  "Giảm cho ăn 1–2 ngày",
			Detail: feedDetail,
		})
	}

	if ras {
		actions = append(actions, rasActions(res, phLow, phHigh, tdsHigh, no3High, khLow, needChange)...)
	}

	allowKH := !ras && phLow && !tdsHigh && (tdsLow || tds.Current == nil || tds.Slope != "rising")
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

	if !ras && phHigh && !needChange {
		actions = append(actions, Action{
			Type:      "water_change_percent",
			Title:     "Thay 15% nước",
			Detail:    "pH cao: thay nước nguồn thấp pH hơn. Có thể dùng gỗ lũa/lá bàng về sau. Không dùng axit mạnh.",
			AmountPct: pct(15),
		})
	}

	if !ras && phLow && tdsHigh {
		doNot = append(doNot, "Không đổ baking soda / pH Up khi TDS đang cao.")
	}
	if ras {
		doNot = append(doNot,
			"Không rửa giá thể lọc sinh học bằng nước máy có chlorine.",
			"Không dùng kháng sinh / thuốc diệt khuẩn khi chưa cần: giết luôn vi khuẩn lọc.",
		)
	}
	if improvingPH || improvingTDS {
		doNot = append(doNot, "Chỉ số đang về vùng tốt — đừng chồng thêm hóa chất hôm nay.")
	}
	if nh3Danger || tanDanger {
		doNot = append(doNot, "Không tắt sục khí khi amonia/oxy xấu. Không đổ ammonia lock rồi thôi thay nước.")
	}
	if hasKitDO || hasKitNH3 || no2.Current != nil || no3.Current != nil || kh.Current != nil {
		doNot = append(doNot, "Số test kit (oxy, amonia, nitrite, nitrate, KH) là đo thủ công, không phải cảm biến ESP — nhập lại khi đo kit mới.")
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
	res.Summary = ruleSummary(res, phLow, tdsHigh, improvingPH, improvingTDS, doLow, nh3High || tanHigh, no2High)
}

// 1 dKH ≈ 17,86 mg/L CaCO3 ≈ 30 mg/L NaHCO3, tức 30 g cho mỗi 1000 lít.
const bicarbGramPerDKHPerM3 = 30.0

func rasActions(res *Result, phLow, phHigh, tdsHigh, no3High, khLow, needChange bool) []Action {
	th := res.Thresholds
	kh := res.Trend["kh"]
	var out []Action

	bicarbDose := ""
	if res.Profile.VolumeL > 0 {
		g := bicarbGramPerDKHPerM3 * res.Profile.VolumeL / 1000
		bicarbDose = fmt.Sprintf(" Với %.0f lít, khoảng %.0f g NaHCO₃ tăng 1 dKH; chia làm nhiều lần trong ngày.", res.Profile.VolumeL, g)
	}

	switch {
	case phLow && (kh.Current == nil || khLow):
		detail := "pH tụt trong RAS thường do lọc sinh học ăn dần độ kiềm. Châm sodium bicarbonate (NaHCO₃, baking soda) từ từ, giữ KH từ " +
			fmt.Sprintf("%.0f dKH", th.KHMin) + " trở lên. Không tăng quá 0,3 pH mỗi ngày." + bicarbDose
		if kh.Current == nil {
			detail = "Chưa có số KH. Đo KH bằng test kit trước, rồi mới châm. " + detail
		}
		out = append(out, Action{Type: "add_alkalinity", Title: "Châm bicarbonate giữ độ kiềm", Detail: detail})
	case phLow:
		out = append(out, Action{
			Type:   "degas_co2",
			Title:  "Tăng tách CO₂",
			Detail: fmt.Sprintf("KH %.1f dKH đã đủ mà pH vẫn thấp: CO₂ tích tụ do mật độ cá cao. Tăng sục khí, tháp tách khí, thông gió phòng nuôi. Không châm thêm bicarbonate khi KH đã đủ.", *kh.Current),
		})
	case khLow:
		out = append(out, Action{
			Type:   "add_alkalinity",
			Title:  "Châm bicarbonate trước khi pH tụt",
			Detail: fmt.Sprintf("KH %.1f dKH dưới %.0f dKH: lọc sinh học sắp thiếu độ kiềm, pH sẽ tụt. Châm NaHCO₃ từ từ.", *kh.Current, th.KHMin) + bicarbDose,
		})
	}

	if phHigh {
		out = append(out, Action{
			Type:   "check_dosing",
			Title:  "Giảm châm kiềm",
			Detail: "pH cao trong RAS thường do châm bicarbonate quá tay hoặc nguồn nước cấp có pH cao. Ngưng châm, đo KH, kiểm tra nước cấp.",
		})
	}

	if (tdsHigh || no3High) && !needChange {
		detail := "TDS và nitrate tích dần là bình thường trong RAS. Tăng lượng thay nước đều mỗi ngày (ví dụ từ 5% lên 10%) thay vì thay lớn một lần."
		if res.Profile.VolumeL > 0 {
			detail += fmt.Sprintf(" 10%% của %.0f lít là khoảng %.0f lít/ngày.", res.Profile.VolumeL, res.Profile.VolumeL*0.1)
		}
		out = append(out, Action{Type: "increase_daily_exchange", Title: "Tăng thay nước hằng ngày", Detail: detail})
	}
	return out
}

func findingForUpper(metric, label string, tr MetricTrend, warn, danger float64) Finding {
	f := Finding{Metric: metric, Value: tr.Current, Target: [2]float64{0, warn}, Slope: tr.Slope, Status: "missing"}
	if tr.Current == nil {
		f.Label = "Chưa nhập " + label
		return f
	}
	v := *tr.Current
	switch {
	case v > danger:
		f.Status = "high"
		f.Label = fmt.Sprintf("%s cao (%.2f mg/L)", label, v)
	case v > warn:
		f.Status = "high"
		f.Label = fmt.Sprintf("%s hơi cao (%.2f mg/L)", label, v)
	default:
		f.Status = "ok"
		f.Label = fmt.Sprintf("%s ổn (%.2f mg/L)", label, v)
	}
	return f
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
	case "no2":
		return v > th.NO2Max
	case "no3":
		return v > th.NO3Max
	case "kh":
		return v < th.KHMin/2
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

func ruleSummary(res *Result, phLow, tdsHigh, improvingPH, improvingTDS, doLow, ammoniaHigh, no2High bool) string {
	ras := res.Profile.IsRAS()
	if ammoniaHigh {
		if ras {
			return "Amonia từ test kit đang cao: lọc sinh học RAS đang quá tải. Ngưng cho ăn, tăng oxy, kiểm tra lọc trước các chỉ số khác."
		}
		return "Amonia từ test kit đang cao (TAN và/hoặc NH₃ tự do). Ưu tiên thay nước, ngưng cho ăn, tăng sục khí trước các chỉ số khác."
	}
	if no2High {
		return "Nitrite từ test kit đang cao: lọc sinh học chưa xử lý kịp. Giảm cho ăn, tăng oxy, kiểm tra lọc."
	}
	if doLow {
		return "Oxy hòa tan (kit) thấp. Tăng sục khí, hạn chế khuấy đáy, đo kit lại sau vài giờ."
	}
	if ras && phLow && res.Ready {
		return "pH đang thấp. Trong RAS thường do thiếu độ kiềm (châm bicarbonate) hoặc CO₂ tích tụ (tăng tách khí). Đo KH để biết hướng xử lý."
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
