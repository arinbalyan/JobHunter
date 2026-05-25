// Command: inbox
// Scans Gmail inbox for bounces, replies, and checks tracking pixel stats.
// Provides comprehensive telemetry about email campaign performance.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/arinbalyan/jobhunter/internal/config"
	"github.com/arinbalyan/jobhunter/internal/db"
)

func main() {
	fmt.Println()
	fmt.Println("  JobHunter Inbox Telemetry")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// ── Database stats ──
	fmt.Println("  \033[1m📊 Database Stats\033[0m")
	fmt.Println()

	dbPool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Printf("  \033[31m✗ Database: %v\033[0m\n", err)
	} else {
		defer dbPool.Close()

		// Count emails by status
		type statusCount struct {
			name  string
			count int
		}
		statuses := []statusCount{}
		for _, s := range []struct{ name, status string }{
			{"Total sent", "sent"},
			{"Pending", "pending"},
			{"Failed", "failed"},
			{"Bounced", "bounced"},
			{"Replied", "replied"},
		} {
			var count int
			dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE status=$1", s.status).Scan(&count)
			statuses = append(statuses, statusCount{s.name, count})
		}

		// Today's stats
		var todaySent, todayOpened, todayClicked, todayBounced int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE sent_at > CURRENT_DATE AND status='sent'").Scan(&todaySent)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE opened=true AND opened_at > CURRENT_DATE").Scan(&todayOpened)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE clicked=true AND clicked_at > CURRENT_DATE").Scan(&todayClicked)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE bounced=true AND bounced_at > CURRENT_DATE").Scan(&todayBounced)

		// All-time opens/clicks
		var totalOpened, totalClicked, totalReplied, totalBounced int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE opened=true").Scan(&totalOpened)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE clicked=true").Scan(&totalClicked)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE replied=true").Scan(&totalReplied)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE bounced=true").Scan(&totalBounced)

		// Jobs in queue
		var pendingQueue int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM email_queue WHERE status='pending'").Scan(&pendingQueue)

		var totalSent int
		for _, s := range statuses {
			if s.name == "Total sent" {
				totalSent = s.count
			}
			fmt.Printf("  %-20s %d\n", s.name+":", s.count)
		}
		fmt.Printf("  %-20s %d\n", "In queue:", pendingQueue)

		fmt.Println()
		fmt.Printf("  \033[1m📈 Engagement Rates\033[0m\n")
		fmt.Println()
		if totalSent > 0 {
			fmt.Printf("  %-20s %d/%d (%.1f%%)\n", "Opened:", totalOpened, totalSent, float64(totalOpened)/float64(totalSent)*100)
			fmt.Printf("  %-20s %d/%d (%.1f%%)\n", "Clicked:", totalClicked, totalSent, float64(totalClicked)/float64(totalSent)*100)
			fmt.Printf("  %-20s %d/%d (%.1f%%)\n", "Replied:", totalReplied, totalSent, float64(totalReplied)/float64(totalSent)*100)
			fmt.Printf("  %-20s %d/%d (%.1f%%)\n", "Bounced:", totalBounced, totalSent, float64(totalBounced)/float64(totalSent)*100)
			sentRate := float64(totalSent-totalBounced) / float64(totalSent) * 100
			fmt.Printf("  %-20s %.1f%%\n", "Deliverability:", sentRate)
		} else {
			fmt.Println("  No emails sent yet.")
		}

		fmt.Println()
		fmt.Printf("  \033[1m📅 Today\033[0m\n")
		fmt.Println()
		fmt.Printf("  %-20s %d\n", "Sent:", todaySent)
		fmt.Printf("  %-20s %d\n", "Opened:", todayOpened)
		fmt.Printf("  %-20s %d\n", "Clicked:", todayClicked)
		fmt.Printf("  %-20s %d\n", "Bounced:", todayBounced)
	}

	// ── IMAP Scan ──
	fmt.Println()
	fmt.Println("  \033[1m📬 IMAP Inbox Scan\033[0m")
	fmt.Println()

	if cfg.IMAPUser == "" || cfg.IMAPPass == "" {
		fmt.Println("  \033[33m⚠ IMAP not configured — set IMAP_USER and IMAP_PASS\033[0m")
	} else {
		conn, err := tls.Dial("tcp", "imap.gmail.com:993", &tls.Config{ServerName: "imap.gmail.com"})
		if err != nil {
			fmt.Printf("  \033[31m✗ IMAP connection failed: %v\033[0m\n", err)
		} else {
			defer conn.Close()

			// Login
			conn.SetDeadline(time.Now().Add(10 * time.Second))
			conn.Write([]byte(fmt.Sprintf("a001 LOGIN %s %s\r\n", cfg.IMAPUser, cfg.IMAPPass)))
			resp := make([]byte, 4096)
			n, _ := conn.Read(resp)
			loginResp := string(resp[:n])
			if strings.Contains(loginResp, "NO") || strings.Contains(loginResp, "BAD") {
				fmt.Printf("  \033[31m✗ IMAP login failed\033[0m\n")
			} else {
				fmt.Printf("  \033[32m✓ Connected to Gmail IMAP\033[0m\n")

				// Select INBOX
				conn.Write([]byte("a002 SELECT INBOX\r\n"))
				n, _ = conn.Read(resp)

				// Search for bounces today
				today := time.Now().UTC().Format("02-Jan-2006")
				conn.Write([]byte(fmt.Sprintf("a003 SEARCH FROM \"mailer-daemon\" SINCE %s\r\n", today)))
				n, _ = conn.Read(resp)
				bounceIDs := parseSearchResult(string(resp[:n]))
				fmt.Printf("  \033[32m✓ Bounces today: %d\033[0m\n", len(bounceIDs))

				// Search for replies today
				conn.Write([]byte(fmt.Sprintf("a004 SEARCH SUBJECT \"Re:\" SINCE %s\r\n", today)))
				n, _ = conn.Read(resp)
				replyIDs := parseSearchResult(string(resp[:n]))
				fmt.Printf("  \033[32m✓ Replies today: %d\033[0m\n", len(replyIDs))

				// Search for unseen (unread)
				conn.Write([]byte("a005 SEARCH UNSEEN\r\n"))
				n, _ = conn.Read(resp)
				unseenIDs := parseSearchResult(string(resp[:n]))
				fmt.Printf("  \033[32m✓ Unread emails: %d\033[0m\n", len(unseenIDs))

				// Total inbox count
				conn.Write([]byte("a006 STATUS INBOX (MESSAGES)\r\n"))
				n, _ = conn.Read(resp)
				statusResp := string(resp[:n])
				fmt.Printf("  \033[32m✓ Inbox total: %s\033[0m\n", extractInboxCount(statusResp))

				// Logout
				conn.Write([]byte("a007 LOGOUT\r\n"))
			}
		}
	}

	// ── Email deliverability check ──
	fmt.Println()
	fmt.Println("  \033[1m📋 Provider Health\033[0m")
	fmt.Println()

	if dbPool != nil {
		// Provider stats from last 7 days
		var sent7d, bounced7d, failed7d int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE sent_at > NOW() - INTERVAL '7 days' AND status='sent'").Scan(&sent7d)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE bounced=true AND bounced_at > NOW() - INTERVAL '7 days'").Scan(&bounced7d)
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE status='failed' AND sent_at > NOW() - INTERVAL '7 days'").Scan(&failed7d)

		fmt.Printf("  %-25s %d\n", "Last 7 days — Sent:", sent7d)
		fmt.Printf("  %-25s %d\n", "Bounced:", bounced7d)
		fmt.Printf("  %-25s %d\n", "Failed:", failed7d)
		if sent7d > 0 {
			deliverRate := float64(sent7d-bounced7d) / float64(sent7d) * 100
			fmt.Printf("  %-25s %.1f%%\n", "Deliverability:", deliverRate)
		}

		// Daily quota usage
		var todayCount int
		dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM emails WHERE sent_at > CURRENT_DATE").Scan(&todayCount)
		quota := cfg.DailyEmailLimit
		if quota == 0 {
			quota = 500
		}
		fmt.Printf("  %-25s %d/%d\n", "Daily quota used:", todayCount, quota)
		remaining := quota - todayCount
		if remaining < 0 {
			remaining = 0
		}
		fmt.Printf("  %-25s %d\n", "Remaining today:", remaining)
	}

	fmt.Println()
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println("  \033[32m✓ Telemetry complete\033[0m")
	fmt.Println()
}

func parseSearchResult(resp string) []string {
	if !strings.Contains(resp, "SEARCH") {
		return nil
	}
	parts := strings.Fields(resp)
	var ids []string
	for _, p := range parts {
		isNum := true
		for _, c := range p {
			if c < '0' || c > '9' {
				isNum = false
				break
			}
		}
		if isNum && p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

func extractInboxCount(resp string) string {
	// Response format: * STATUS "INBOX" (MESSAGES 42)
	if idx := strings.Index(resp, "MESSAGES"); idx >= 0 {
		after := resp[idx+8:]
		parts := strings.Fields(after)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "?"
}
