// Command: botid
// One-time utility to discover your Telegram chat ID.
// Run once, send a message to your bot, and it prints your chat ID.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		fmt.Println("Set TELEGRAM_BOT_TOKEN env var first.")
		fmt.Println("Get a token from @BotFather on Telegram.")
		os.Exit(1)
	}

	fmt.Println("Waiting for a message from you...")
	fmt.Println("Open Telegram, find your bot, and send it any message (even just 'hi').")
	fmt.Println("")

	client := &http.Client{Timeout: 30 * time.Second}
	lastOffset := 0

	for i := 0; i < 60; i++ {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=10", token, lastOffset)
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("API error: %v — retrying...", err)
			time.Sleep(2 * time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
		resp.Body.Close()

		var result struct {
			Ok     bool `json:"ok"`
			Result []struct {
				UpdateID int `json:"update_id"`
				Message  struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			} `json:"result"`
		}
		json.Unmarshal(body, &result)

		if !result.Ok {
			time.Sleep(2 * time.Second)
			continue
		}

		if len(result.Result) > 0 {
			lastUpdate := result.Result[len(result.Result)-1]
			lastOffset = lastUpdate.UpdateID + 1
			chatID := lastUpdate.Message.Chat.ID

			fmt.Println("")
			fmt.Println("═══════════════════════════════════════════")
			fmt.Println("  Your Telegram Chat ID:", chatID)
			fmt.Println("═══════════════════════════════════════════")
			fmt.Println("")
			fmt.Println("Add this to your .env:")
			fmt.Printf("  TELEGRAM_CHAT_ID=%d\n", chatID)
			fmt.Println("")
			fmt.Println("Or add to GitHub Secrets:")
			fmt.Printf("  TELEGRAM_CHAT_ID = %d\n", chatID)
			return
		}

		if i%10 == 0 && i > 0 {
			fmt.Printf("Still waiting... (%d seconds elapsed)\n", i*2)
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Println("No message received within 120 seconds.")
	fmt.Println("Make sure you've messaged your bot on Telegram and try again.")
	os.Exit(1)
}
