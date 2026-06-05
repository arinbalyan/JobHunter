// Command: doctor
// Diagnostic tool that checks all dependencies and configuration.
// Prints a green/red checklist.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
)

var checkMark = "\033[32m✓\033[0m"
var crossMark = "\033[31m✗\033[0m"
var warnMark = "\033[33m⚠\033[0m"

func main() {
	fmt.Println("\n  JobHunter Doctor")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	exitCode := 0

	// ── 1. .env file ──
	fmt.Print("  [1] .env file .............. ")
	if _, err := os.Stat(".env"); err == nil {
		fmt.Println(checkMark + " found")
	} else {
		// Auto-create from .env.example
		if example, err := os.ReadFile(".env.example"); err == nil {
			os.WriteFile(".env", example, 0644)
			fmt.Println(checkMark + " created from .env.example (edit it with your keys)")
		} else {
			fmt.Println(crossMark + " missing — create from .env.example")
			exitCode = 1
		}
	}

	// ── 2. Load config ──
	fmt.Print("  [2] Config loading ......... ")
	cfg, err := config.Load()
	if err != nil {
		fmt.Println(crossMark + " " + err.Error())
		exitCode = 1
	} else {
		fmt.Println(checkMark)
	}

	// ── 3. Config validation ──
	fmt.Print("  [3] Config validation ...... ")
	if cfg != nil {
		if err := cfg.Validate(); err != nil {
			fmt.Println(crossMark + " " + err.Error())
			exitCode = 1
		} else {
			fmt.Println(checkMark)
		}
	}

	// ── 4. config.yaml ──
	fmt.Print("  [4] config.yaml ............ ")
	if _, err := os.Stat(".agent-data/config.yaml"); err == nil {
		yc, err := config.LoadYAML(".agent-data/config.yaml")
		if err != nil {
			fmt.Println(crossMark + " " + err.Error())
			exitCode = 1
		} else {
			fmt.Printf("%s %d rejection patterns, %d email filters\n", checkMark, len(yc.RejectTitles), len(yc.EmailFilters))
		}
	} else {
		fmt.Println(warnMark + " missing — using defaults")
	}

	// ── 5. llm.yaml ──
	fmt.Print("  [5] llm.yaml ............... ")
	if _, err := os.Stat(".agent-data/llm.yaml"); err == nil {
		fmt.Println(checkMark)
	} else {
		fmt.Println(warnMark + " missing — providers loaded from env only")
	}

	// ── 6. Database connection ──
	fmt.Print("  [6] Database connection .... ")
	if cfg != nil && cfg.DatabaseURL != "" {
		dbURL := strings.Replace(cfg.DatabaseURL, "postgresql://", "postgres://", 1)
		conn, err := net.DialTimeout("tcp", extractHost(dbURL), 5*time.Second)
		if err != nil {
			fmt.Println(crossMark + " cannot reach: " + err.Error())
			exitCode = 1
		} else {
			conn.Close()
			fmt.Println(checkMark + " reachable")
		}
	} else {
		fmt.Println(crossMark + " DATABASE_URL not set")
		exitCode = 1
	}

	// ── 7. Gmail SMTP ──
	fmt.Print("  [7] Gmail SMTP ............ ")
	if cfg != nil && cfg.GmailUser != "" && cfg.GmailAppPass != "" {
		conn, err := net.DialTimeout("tcp", "smtp.gmail.com:587", 5*time.Second)
		if err != nil {
			fmt.Println(crossMark + " cannot reach: " + err.Error())
			exitCode = 1
		} else {
			conn.Close()
			fmt.Println(checkMark + " reachable")
		}
	} else {
		fmt.Println(crossMark + " GMAIL_USER or GMAIL_APP_PASS not set")
		exitCode = 1
	}

	// ── 8. OpenRouter API ──
	fmt.Print("  [8] OpenRouter API ........ ")
	if cfg != nil && cfg.OpenRouterAPIKey != "" {
		orCtx, orCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer orCancel()
		req, _ := http.NewRequestWithContext(orCtx, "GET", "https://openrouter.ai/api/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.OpenRouterAPIKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println(crossMark + " cannot reach: " + err.Error())
			exitCode = 1
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			if resp.StatusCode == 200 {
				fmt.Println(checkMark + " connected (" + extractModelCount(string(body)) + " models available)")
			} else {
				fmt.Println(crossMark + " status " + fmt.Sprintf("%d", resp.StatusCode) + ": " + string(body[:min(len(body), 100)]))
				exitCode = 1
			}
		}
	} else {
		fmt.Println(warnMark + " not configured — router will use other providers")
	}

	// ── 9. Other LLM providers ──
	fmt.Print("  [9] Other LLM providers .... ")
	providerCount := 0
	for _, key := range []string{"GROQ_API_KEY", "TOGETHER_API_KEY", "DEEPINFRA_API_KEY", "FIREWORKS_API_KEY",
		"HYPERBOLIC_API_KEY", "SAMBANOVA_API_KEY", "CEREBRAS_API_KEY", "NVIDIA_API_KEY", "ZAI_API_KEY"} {
		if os.Getenv(key) != "" {
			providerCount++
		}
	}
	if providerCount > 0 {
		fmt.Printf("%s %d additional providers configured\n", checkMark, providerCount)
	} else {
		fmt.Println(warnMark + " none — only OpenRouter will be used")
	}

	// ── 10. scrappy Go library ──
	fmt.Print("  [10] scrappy Go library ...... ")
	// Verify the library import compiles by checking go.mod has the dependency
	importOK := false
	if data, err := os.ReadFile("go.mod"); err == nil {
		importOK = strings.Contains(string(data), "github.com/arinbalyan/scrappy")
	}
	if importOK {
		fmt.Println(checkMark + " found in go.mod — using pkg/scrappy")
	} else {
		fmt.Println(crossMark + " missing from go.mod — run: go get github.com/arinbalyan/scrappy")
	}

	// ── 11. Telegram ──
	fmt.Print("  [11] Telegram bot ........... ")
	if os.Getenv("TELEGRAM_BOT_TOKEN") != "" && os.Getenv("TELEGRAM_CHAT_ID") != "" {
		fmt.Println(checkMark + " configured")
	} else {
		fmt.Println(warnMark + " not configured — reports will be logged only")
	}

	// ── 12. IMAP ──
	fmt.Print("  [12] IMAP (bounce/reply) .... ")
	if cfg != nil && cfg.IMAPUser != "" && cfg.IMAPPass != "" {
		conn, err := tls.Dial("tcp", "imap.gmail.com:993", &tls.Config{ServerName: "imap.gmail.com"})
		if err != nil {
			fmt.Println(crossMark + " cannot reach: " + err.Error())
			exitCode = 1
		} else {
			conn.Close()
			fmt.Println(checkMark + " reachable")
		}
	} else {
		fmt.Println(warnMark + " not configured — bounce/reply detection disabled")
	}

	// ── 13. GitHub CLI ──
	fmt.Print("  [13] GitHub CLI (gh) ........ ")
	_, err = exec.LookPath("gh")
	if err != nil {
		fmt.Println(warnMark + " not in PATH — install from cli.github.com")
	} else {
		fmt.Println(checkMark + " found")
	}

	// ── 14. Resume ──
	fmt.Print("  [14] Resume PDF ............. ")
	found := false
	entries, _ := os.ReadDir(".agent-data")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			fmt.Printf("%s %s\n", checkMark, e.Name())
			found = true
			break
		}
	}
	if !found {
		fmt.Println(warnMark + " not found in .agent-data/ — emails sent without attachment")
	}

	// ── 15. Migrations ──
	fmt.Print("  [15] DB migrations .......... ")
	if cfg != nil && cfg.DatabaseURL != "" {
		// Just check schema_migrations table exists via TCP check, actual migration check via Go would need full connect
		fmt.Println(warnMark + " run 'go run ./cmd/migrate' to verify")
	} else {
		fmt.Println(crossMark + " DATABASE_URL not set")
	}

	fmt.Println()
	fmt.Println("  " + strings.Repeat("─", 50))
	if exitCode == 0 {
		fmt.Println("  \033[32mAll checks passed!\033[0m")
	} else {
		fmt.Printf("  \033[31m%d issues found — fix the items marked with ✗\033[0m\n", exitCode)
	}
	fmt.Println()

	os.Exit(exitCode)
}

func extractHost(dbURL string) string {
	// postgres://user:pass@host:port/db
	if strings.HasPrefix(dbURL, "postgres://") {
		dbURL = strings.TrimPrefix(dbURL, "postgres://")
	} else if strings.HasPrefix(dbURL, "postgresql://") {
		dbURL = strings.TrimPrefix(dbURL, "postgresql://")
	}
	// Remove user:pass@
	if idx := strings.Index(dbURL, "@"); idx >= 0 {
		dbURL = dbURL[idx+1:]
	}
	// Remove /dbname and query params
	if idx := strings.Index(dbURL, "/"); idx >= 0 {
		dbURL = dbURL[:idx]
	}
	if idx := strings.Index(dbURL, "?"); idx >= 0 {
		dbURL = dbURL[:idx]
	}
	return dbURL
}

func extractModelCount(body string) string {
	// Count occurrences of "id": in the response
	count := strings.Count(body, `"id":`)
	return fmt.Sprintf("%d", count)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
