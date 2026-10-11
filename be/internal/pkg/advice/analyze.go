package advice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
)

var (
	ErrLLMNotConfigured = errors.New("Chưa cấu hình OPENAI_API_KEY, không phân tích")
	ErrLLMUnavailable   = errors.New("Không kết nối được OpenAI, không phân tích")
)

const (
	minPointsReady = 3
	minSpanReady   = 1 * time.Hour
)

func NormalizeProfile(p Profile) Profile {
	p.Species = strings.ToLower(strings.TrimSpace(p.Species))
	if p.Species == "" {
		p.Species = "ca_chinh"
	}
	if p.VolumeL <= 0 {
		p.VolumeL = 9000 // mặc định 9 khối
	}
	if strings.ToLower(strings.TrimSpace(p.SystemType)) == "ras" {
		p.SystemType = "ras"
	} else {
		p.SystemType = "pond"
	}
	return p
}

func Analyze(history []fish.Measurement, profile Profile) *Result {
	return AnalyzeWithKit(history, nil, profile, nil)
}

func AnalyzeWith(history []fish.Measurement, profile Profile, th *Thresholds) *Result {
	return AnalyzeWithKit(history, nil, profile, th)
}

const kitFreshFor = 24 * time.Hour

func AnalyzeWithKit(history []fish.Measurement, kits []fish.KitReading, profile Profile, th *Thresholds) *Result {
	profile = NormalizeProfile(profile)
	resolved := withSystemDefaults(profile.SystemType, ThresholdsFor(profile.Species))
	if th != nil {
		resolved = mergeThresholds(resolved, *th)
	}
	series := BuildSeries(history)
	hasKit := latestKit(kits) != nil

	res := &Result{
		Profile:      profile,
		Thresholds:   resolved,
		Current:      map[string]*float64{},
		Trend:        map[string]MetricTrend{},
		Limitations:  "Chưa đo NH3, NO2, NO3, DO, KH/GH. pH/TDS/nhiệt độ mô tả môi trường nước, không kết luận ngộ độc ammonia.",
		SampleWindow: "24h",
	}

	if len(series) == 0 && !hasKit {
		res.Overall = "unknown"
		res.Ready = false
		res.Reason = "no_data"
		res.Summary = "Chưa có dữ liệu đo. Bấm Measure hoặc bật lịch tự động trước khi phân tích."
		res.Findings = nil
		res.Actions = []Action{{Priority: 1, Type: "nothing", Title: "Chưa đủ dữ liệu", Detail: res.Summary}}
		res.DoNot = []string{"Không đổ hóa chất khi chưa có số đo."}
		return res
	}

	var lastPH, lastTemp *float64
	if len(series) > 0 {
		last := series[len(series)-1]
		res.LatestAt = &last.Time
		res.Stale = time.Since(last.Time) > 6*time.Hour
		lastPH, lastTemp = last.PH, last.Temperature
		res.Current = map[string]*float64{
			"temperature": roundPtr(last.Temperature, 1),
			"ph":          roundPtr(last.PH, 2),
			"tds":         roundPtr(last.TDS, 0),
		}
		res.Trend = map[string]MetricTrend{
			"temperature": buildTrend(history, series, "temperature", 0.3, 1),
			"ph":          buildTrend(history, series, "ph", 0.1, 2),
			"tds":         buildTrend(history, series, "tds", 20, 0),
		}
		res.Series6h = downsample(series, last.Time.Add(-6*time.Hour), 30*time.Minute, 12)
		res.Ready, res.Reason = readiness(res.Trend)
	} else {
		res.Ready = false
		res.Reason = "kit_only"
	}

	applyKit(res, kits, lastPH, lastTemp)
	applyRules(res)
	if res.Stale && res.LatestAt != nil {
		res.Summary = "Dữ liệu đo đã cũ hơn 6 giờ (mẫu cuối " + res.LatestAt.Local().Format("02/01 15:04") + "). " + res.Summary
	}
	if res.Kit.Stale && res.Kit.MeasuredAt != nil {
		res.Summary = "Số test kit đã cũ hơn 24 giờ (nhập " + res.Kit.MeasuredAt.Local().Format("02/01 15:04") + "). " + res.Summary
	}
	return res
}

func latestKit(kits []fish.KitReading) *fish.KitReading {
	var best *fish.KitReading
	for i := range kits {
		k := &kits[i]
		if !k.HasAny() {
			continue
		}
		if best == nil || k.MeasuredAt.After(best.MeasuredAt) {
			best = k
		}
	}
	return best
}

type kitField struct {
	metric string
	digits int
	get    func(*fish.KitReading) *float64
}

var kitFields = []kitField{
	{"do", 1, func(k *fish.KitReading) *float64 { return k.DOMGL }},
	{"tan", 2, func(k *fish.KitReading) *float64 { return k.TANMGL }},
	{"no2", 2, func(k *fish.KitReading) *float64 { return k.NO2MGL }},
	{"no3", 0, func(k *fish.KitReading) *float64 { return k.NO3MGL }},
	{"kh", 1, func(k *fish.KitReading) *float64 { return k.KHDKH }},
}

// Mỗi chỉ số lấy mẫu mới nhất có giá trị, vì một lần nhập kit thường chỉ có vài chỉ số.
func applyKit(res *Result, kits []fish.KitReading, ph, temp *float64) {
	if res.Current == nil {
		res.Current = map[string]*float64{}
	}
	if res.Trend == nil {
		res.Trend = map[string]MetricTrend{}
	}
	k := latestKit(kits)
	if k == nil {
		for _, f := range kitFields {
			res.Trend[f.metric] = MetricTrend{Slope: "unknown"}
		}
		res.Trend["nh3_free"] = MetricTrend{Slope: "unknown"}
		return
	}
	stale := time.Since(k.MeasuredAt) > kitFreshFor
	res.Kit = KitSnapshot{
		MeasuredAt: &k.MeasuredAt,
		Stale:      stale,
		Source:     "kit",
	}

	var latestTAN *fish.KitReading
	for _, f := range kitFields {
		var latest *fish.KitReading
		n := 0
		var first, last time.Time
		for i := range kits {
			row := &kits[i]
			if f.get(row) == nil {
				continue
			}
			n++
			if first.IsZero() || row.MeasuredAt.Before(first) {
				first = row.MeasuredAt
			}
			if last.IsZero() || row.MeasuredAt.After(last) {
				last = row.MeasuredAt
				latest = row
			}
		}
		tr := MetricTrend{Slope: "unknown", N: n}
		if latest != nil {
			res.Current[f.metric] = roundPtr(f.get(latest), f.digits)
			tr.Current = res.Current[f.metric]
		}
		if n >= 2 {
			tr.SpanHours = last.Sub(first).Hours()
		}
		res.Trend[f.metric] = tr
		if f.metric == "tan" {
			latestTAN = latest
		}
	}

	var nh3 *float64
	if latestTAN != nil {
		tempC := 25.0
		if temp != nil {
			tempC = *temp
		} else if latestTAN.TempUsed != nil {
			tempC = *latestTAN.TempUsed
		}
		phV := 7.0
		if ph != nil {
			phV = *ph
		} else if latestTAN.PHUsed != nil {
			phV = *latestTAN.PHUsed
		}
		v := FreeAmmonia(*latestTAN.TANMGL, phV, tempC)
		nh3 = roundPtr(&v, 3)
		res.Current["nh3_free"] = nh3
	}
	res.Trend["nh3_free"] = MetricTrend{Current: nh3, Slope: "unknown", N: res.Trend["tan"].N}
	if !stale {
		res.Limitations = "Oxy, amonia, nitrite, nitrate, KH lấy từ test kit thủ công (không phải cảm biến ESP). NH₃ tự do ước lượng từ TAN + pH/nhiệt theo Emerson. Chưa đo CO₂."
	}
}

func readiness(trends map[string]MetricTrend) (bool, string) {
	bestN := 0
	bestSpan := 0.0
	for _, tr := range trends {
		if tr.N > bestN {
			bestN = tr.N
		}
		if tr.SpanHours > bestSpan {
			bestSpan = tr.SpanHours
		}
		if tr.N >= minPointsReady && tr.SpanHours >= minSpanReady.Hours() {
			return true, ""
		}
	}
	if bestN < minPointsReady {
		return false, "need_more_samples"
	}
	return false, "need_longer_span"
}

func ApplyNarration(res *Result, n *Narration) {
	if res == nil || n == nil {
		return
	}
	if s := strings.TrimSpace(n.Summary); s != "" {
		res.Summary = s
	}
	if len(n.DoNot) > 0 {
		res.DoNot = n.DoNot
	}
	if len(n.Details) == 0 {
		return
	}
	for i := range res.Actions {
		if d, ok := n.Details[res.Actions[i].Type]; ok && strings.TrimSpace(d) != "" {
			res.Actions[i].Detail = strings.TrimSpace(d)
		}
	}
}

func Enrich(ctx context.Context, res *Result, narrator Narrator) error {
	if narrator == nil {
		return ErrLLMNotConfigured
	}
	if res == nil || !res.Ready {
		return nil
	}
	n, err := narrator.Narrate(ctx, res)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLLMUnavailable, err)
	}
	if n == nil || strings.TrimSpace(n.Summary) == "" {
		return ErrLLMUnavailable
	}
	ApplyNarration(res, n)
	res.UsedLLM = true
	return nil
}
