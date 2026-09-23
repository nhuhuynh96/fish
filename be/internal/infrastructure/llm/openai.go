package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/pkg/advice"
)

type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

func New(apiKey, baseURL, model string) *Client {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *Client) Narrate(ctx context.Context, result *advice.Result) (*advice.Narration, error) {
	if c == nil || result == nil {
		return nil, fmt.Errorf("llm client not configured")
	}

	facts, err := json.Marshal(map[string]any{
		"overall":     result.Overall,
		"profile":     result.Profile,
		"thresholds":  result.Thresholds,
		"current":     result.Current,
		"trend":       result.Trend,
		"findings":    result.Findings,
		"actions":     result.Actions,
		"do_not":      result.DoNot,
		"limitations": result.Limitations,
		"series_6h":   result.Series6h,
	})
	if err != nil {
		return nil, err
	}

	sys := `Bạn là trợ lý hồ cá. Chỉ diễn giải dữ liệu JSON đã có.
DO (do) và TAN/NH3 (tan, nh3_free) chỉ dùng khi có trong current — đó là số test kit thủ công, không phải cảm biến ESP.
Không bịa cảm biến còn thiếu (NO2, NO3, KH/GH, hoặc DO/NH3 nếu current không có). Không đổi type/priority của actions.
Không nêu liều hóa chất theo gram nếu volume_l <= 0.
Trả JSON: {"summary":"...","action_details":{"water_change_percent":"..."},"do_not":["..."]}
summary: 1-2 câu tiếng Việt, nêu xu hướng (đang tăng/giảm) chứ không chỉ giá trị hiện tại. Nếu có NH3/DO kit thì nhắc trong summary.
action_details: key = action type, value = giải thích ngắn.
do_not: 2-4 ý cấm.`

	body := map[string]any{
		"model":           c.model,
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": string(facts)},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm empty choices")
	}

	var n advice.Narration
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), &n); err != nil {
		return nil, fmt.Errorf("llm json: %w", err)
	}
	return &n, nil
}
