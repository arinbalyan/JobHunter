package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Bot sends messages to a Telegram chat via the Bot API.
type Bot struct {
	token   string
	chatID  string
	client  *http.Client
	enabled bool
}

// New creates a Telegram bot. Disabled if token or chatID is empty.
func New(token, chatID string) *Bot {
	enabled := token != "" && chatID != ""
	return &Bot{
		token:   token,
		chatID:  chatID,
		client:  &http.Client{Timeout: 15 * time.Second},
		enabled: enabled,
	}
}

// Enabled returns whether the bot is configured.
func (b *Bot) Enabled() bool {
	return b.enabled
}

// Send sends a plain text message with HTML parse mode.
func (b *Bot) Send(ctx context.Context, text string) error {
	if !b.enabled {
		return nil
	}
	return b.call(ctx, "sendMessage", map[string]interface{}{
		"chat_id":                  b.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
}

// SendReport sends a structured report, splitting if too long.
func (b *Bot) SendReport(ctx context.Context, report *ReportMessage) error {
	if !b.enabled {
		return nil
	}
	msg := report.HTML()
	if len(msg) <= 4000 {
		return b.Send(ctx, msg)
	}
	for _, chunk := range splitMessage(msg, 4000) {
		if err := b.Send(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bot) call(ctx context.Context, method string, params map[string]interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var apiResp struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !apiResp.Ok {
		return fmt.Errorf("telegram API error: %s", apiResp.Description)
	}
	return nil
}

// ReportMessage holds a structured run report.
type ReportMessage struct {
	Title        string
	PluginResults []PluginReportItem
	Stats        map[string]float64
	Duration     time.Duration
	Timestamp    time.Time
}

// PluginReportItem holds a single plugin's execution result.
type PluginReportItem struct {
	PluginID   string
	PluginName string
	Success    bool
	Message    string
	Duration   time.Duration
	Metrics    map[string]float64
}

// HTML renders the report as formatted HTML for Telegram.
func (r *ReportMessage) HTML() string {
	ts := r.Timestamp.Format("2006-01-02 15:04:05 UTC")

	msg := fmt.Sprintf(`<b>Run Report</b>
<code>Time:</code> %s
<code>Duration:</code> %s
<code>Plugins:</code> %d
<code>─────────────────</code>
`, ts, formatDuration(r.Duration), len(r.PluginResults))

	allMetrics := make(map[string]float64)
	for _, pr := range r.PluginResults {
		status := "OK"
		if !pr.Success {
			status = "FAIL"
		}
		msg += fmt.Sprintf("\n<b>%s</b> [%s]\n  %s (%s)", pr.PluginName, status, pr.Message, formatDuration(pr.Duration))
		for k, v := range pr.Metrics {
			allMetrics[k] += v
			msg += fmt.Sprintf("\n  %s: %.0f", k, v)
		}
	}

	// Add global stats
	if len(r.Stats) > 0 {
		msg += "\n\n<b>Run Stats:</b>\n"
		allMetrics["total_duration_seconds"] = r.Duration.Seconds()
	}
	for k, v := range r.Stats {
		allMetrics[k] = v
		msg += fmt.Sprintf("\n  %s: %.0f", k, v)
	}

	return msg
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func splitMessage(msg string, maxLen int) []string {
	var chunks []string
	runes := []rune(msg)
	for len(runes) > 0 {
		if len(runes) <= maxLen {
			chunks = append(chunks, string(runes))
			break
		}
		cut := maxLen
		for i := maxLen; i > maxLen-200 && i > 0; i-- {
			if runes[i] == '\n' {
				cut = i + 1
				break
			}
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	return chunks
}
