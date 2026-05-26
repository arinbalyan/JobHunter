// Package telegram provides a simple helper for sending messages via the Telegram Bot API.
package telegram

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SendMessage sends a message via Telegram Bot API.
func SendMessage(ctx context.Context, token, chatID, msg string) error {
	body := fmt.Sprintf(`{"chat_id":"%s","text":"%s","parse_mode":"HTML"}`, chatID, strings.ReplaceAll(msg, `"`, `\"`))
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	return err
}
