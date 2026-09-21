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
	return AnalyzeWith(history, profile, nil)
}

func AnalyzeWith(history []fish.Measurement, profile Profile, th *Thresholds) *Result {
	profile = NormalizeProfile(profile)
	resolved := ThresholdsFor(profile.Species)
	if th != nil {
		resolved = *th
	}
	series := BuildSeries(history)

	res := &Result{
		Profile:      profile,
		Thresholds:   resolved,
		Current:      map[string]*float64{},
		Trend:        map[string]MetricTrend{},
		Limitations:  "Chưa đo NH3, NO2, NO3, DO, KH/GH. pH/TDS/độ đục mô tả môi trường nước, không kết luận ngộ độc ammonia.",
		SampleWindow: "24h",
	}

	if len(series) == 0 {
		res.Overall = "unknown"
		res.Ready = false
		res.Reason = "no_data"
		res.Summary = "Chưa có dữ liệu đo. Bấm Measure hoặc bật lịch tự động trước khi phân tích."
		res.Findings = nil
		res.Actions = []Action{{Priority: 1, Type: "nothing", Title: "Chưa đủ dữ liệu", Detail: res.Summary}}
		res.DoNot = []string{"Không đổ hóa chất khi chưa có số đo."}
		return res
	}

	last := series[len(series)-1]
	res.LatestAt = &last.Time
	res.Stale = time.Since(last.Time) > 6*time.Hour
	res.Current = map[string]*float64{
		"temperature": roundPtr(last.Temperature, 1),
		"ph":          roundPtr(last.PH, 2),
		"turbidity":   roundPtr(last.Turbidity, 1),
		"tds":         roundPtr(last.TDS, 0),
	}
	res.Trend = map[string]MetricTrend{
		"temperature": buildTrend(history, series, "temperature", 0.3, 1),
		"ph":          buildTrend(history, series, "ph", 0.1, 2),
		"turbidity":   buildTrend(history, series, "turbidity", 3, 1),
		"tds":         buildTrend(history, series, "tds", 20, 0),
	}
	res.Series6h = downsample(series, last.Time.Add(-6*time.Hour), 30*time.Minute, 12)

	res.Ready, res.Reason = readiness(res.Trend)
	applyRules(res)
	if res.Stale && res.LatestAt != nil {
		res.Summary = "Dữ liệu đo đã cũ hơn 6 giờ (mẫu cuối " + res.LatestAt.Local().Format("02/01 15:04") + "). " + res.Summary
	}
	return res
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
