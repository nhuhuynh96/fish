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
	resolved := ThresholdsFor(profile.Species)
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
		if k.DOMGL == nil && k.TANMGL == nil {
			continue
		}
		if best == nil || k.MeasuredAt.After(best.MeasuredAt) {
			best = k
		}
	}
	return best
}

func applyKit(res *Result, kits []fish.KitReading, ph, temp *float64) {
	if res.Current == nil {
		res.Current = map[string]*float64{}
	}
	if res.Trend == nil {
		res.Trend = map[string]MetricTrend{}
	}
	k := latestKit(kits)
	if k == nil {
		res.Trend["do"] = MetricTrend{Slope: "unknown"}
		res.Trend["tan"] = MetricTrend{Slope: "unknown"}
		res.Trend["nh3_free"] = MetricTrend{Slope: "unknown"}
		return
	}
	stale := time.Since(k.MeasuredAt) > kitFreshFor
	res.Kit = KitSnapshot{
		MeasuredAt: &k.MeasuredAt,
		Stale:      stale,
		Source:     "kit",
	}
	res.Current["do"] = roundPtr(k.DOMGL, 1)
	res.Current["tan"] = roundPtr(k.TANMGL, 2)
	tempC := 25.0
	if temp != nil {
		tempC = *temp
	} else if k.TempUsed != nil {
		tempC = *k.TempUsed
	}
	phV := 7.0
	if ph != nil {
		phV = *ph
	} else if k.PHUsed != nil {
		phV = *k.PHUsed
	}
	var nh3 *float64
	if k.TANMGL != nil {
		v := FreeAmmonia(*k.TANMGL, phV, tempC)
		nh3 = roundPtr(&v, 3)
		res.Current["nh3_free"] = nh3
	}
	nDO, nTAN := 0, 0
	var firstDO, lastDO, firstTAN, lastTAN time.Time
	for _, row := range kits {
		if row.DOMGL != nil {
			nDO++
			if firstDO.IsZero() || row.MeasuredAt.Before(firstDO) {
				firstDO = row.MeasuredAt
			}
			if lastDO.IsZero() || row.MeasuredAt.After(lastDO) {
				lastDO = row.MeasuredAt
			}
		}
		if row.TANMGL != nil {
			nTAN++
			if firstTAN.IsZero() || row.MeasuredAt.Before(firstTAN) {
				firstTAN = row.MeasuredAt
			}
			if lastTAN.IsZero() || row.MeasuredAt.After(lastTAN) {
				lastTAN = row.MeasuredAt
			}
		}
	}
	doTrend := MetricTrend{Current: res.Current["do"], Slope: "unknown", N: nDO}
	if nDO >= 2 {
		doTrend.SpanHours = lastDO.Sub(firstDO).Hours()
	}
	tanTrend := MetricTrend{Current: res.Current["tan"], Slope: "unknown", N: nTAN}
	if nTAN >= 2 {
		tanTrend.SpanHours = lastTAN.Sub(firstTAN).Hours()
	}
	res.Trend["do"] = doTrend
	res.Trend["tan"] = tanTrend
	res.Trend["nh3_free"] = MetricTrend{Current: nh3, Slope: "unknown", N: nTAN}
	if !stale && (k.DOMGL != nil || k.TANMGL != nil) {
		res.Limitations = "Oxy và amonia lấy từ test kit thủ công (không phải cảm biến ESP). NH₃ tự do ước lượng từ TAN + pH/nhiệt Emerson. Chưa có NO2/NO3/KH/GH."
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
