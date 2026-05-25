package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"
)

// IMAPConfig holds Gmail IMAP connection configuration.
type IMAPConfig struct {
	User     string
	Password string
	Host     string
	Port     int
}

// Database defines the minimal interface the IMAP scanner needs.
type Database interface {
	MarkBounced(ctx context.Context, messageID string, bounceType string) error
	MarkReplied(ctx context.Context, messageID string) error
	GetRecentEmails(ctx context.Context, hours int) ([]interface{}, error)
}

// Scanner scans the Gmail inbox for bounces and replies.
type Scanner struct {
	config  IMAPConfig
	db      Database
}

// New creates a new IMAP scanner.
func New(cfg IMAPConfig, db Database) *Scanner {
	return &Scanner{
		config: cfg,
		db:     db,
	}
}

// ScanResult holds the results of a scan.
type ScanResult struct {
	Bounces int
	Replies int
	Errors  []error
}

// Scan checks the inbox for bounces and replies since the last check.
func (s *Scanner) Scan(ctx context.Context) (*ScanResult, error) {
	fmt.Printf("[imap] scanning inbox for bounces and replies...")

	result := &ScanResult{}

	// Establish TLS connection to Gmail IMAP
	conn, err := s.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect to IMAP: %w", err)
	}
	defer conn.Close()

	// Login
	if err := s.login(conn); err != nil {
		return nil, fmt.Errorf("IMAP login: %w", err)
	}

	// Select INBOX
	if err := s.selectInbox(conn); err != nil {
		return nil, fmt.Errorf("select inbox: %w", err)
	}

	// Search for bounces (Mailer-Daemon)
	bounceIDs, err := s.searchBounces(conn)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("search bounces: %w", err))
	} else {
		for _, msgID := range bounceIDs {
			if err := s.processBounce(conn, msgID); err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.Bounces++
			}
		}
	}

	// Search for replies (Re: subject)
	replyIDs, err := s.searchReplies(conn)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("search replies: %w", err))
	} else {
		for _, msgID := range replyIDs {
			if err := s.processReply(conn, msgID); err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.Replies++
			}
		}
	}

	// Mark all scanned messages as SEEN
	if len(bounceIDs) > 0 || len(replyIDs) > 0 {
		if err := s.markSeen(conn, append(bounceIDs, replyIDs...)); err != nil {
			fmt.Printf("[imap] error marking messages as seen: %v", err)
		}
	}

	fmt.Printf("[imap] scan complete: %d bounces, %d replies", result.Bounces, result.Replies)
	return result, nil
}

// IMAP is not available in Go's standard library without importing a package.
// The full implementation requires go-imap or similar.
// This file provides the interface and a lightweight implementation using
// raw TLS sockets for the IMAP protocol.
//
// NOTE: For production use, consider using github.com/emersion/go-imap/v2
// which provides a full IMAP client. This implementation works with raw
// IMAP commands for zero dependencies.

func (s *Scanner) connect(ctx context.Context) (*tls.Conn, error) {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	tlsConfig := &tls.Config{
		ServerName: s.config.Host,
		MinVersion: tls.VersionTLS12,
	}

	dialer := tls.Dialer{
		Config: tlsConfig,
	}

	tlsConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tls dial: %w", err)
	}

	// Type-assert to *tls.Conn (tls.Dialer always returns *tls.Conn)
	tlsTyped, ok := tlsConn.(*tls.Conn)
	if !ok {
		tlsConn.Close()
		return nil, fmt.Errorf("expected *tls.Conn, got %T", tlsConn)
	}

	// Consume greeting
	greeting := make([]byte, 1024)
	if _, err := tlsTyped.Read(greeting); err != nil {
		tlsTyped.Close()
		return nil, fmt.Errorf("read greeting: %w", err)
	}

	return tlsTyped, nil
}

func (s *Scanner) login(conn *tls.Conn) error {
	cmd := fmt.Sprintf("a001 LOGIN %s %s\r\n", s.config.User, s.config.Password)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}

	response := string(resp[:n])
	if strings.Contains(response, "NO") || strings.Contains(response, "BAD") {
		return fmt.Errorf("login failed: %s", response)
	}

	return nil
}

func (s *Scanner) selectInbox(conn *tls.Conn) error {
	cmd := "a002 SELECT INBOX\r\n"
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("send select: %w", err)
	}

	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("read select response: %w", err)
	}

	_ = string(resp[:n])
	return nil
}

func (s *Scanner) searchBounces(conn *tls.Conn) ([]string, error) {
	// Search for unseen messages from mailer-daemon in the last 24 hours
	cmd := "a003 SEARCH UNSEEN FROM \"mailer-daemon\" SINCE " +
		time.Now().Add(-24*time.Hour).UTC().Format("02-Jan-2006") + "\r\n"
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("send search bounces: %w", err)
	}

	return s.readSearchResults(conn)
}

func (s *Scanner) searchReplies(conn *tls.Conn) ([]string, error) {
	// Search for unseen messages with Re: subject in the last 24 hours
	cmd := "a004 SEARCH UNSEEN SUBJECT \"Re:\" SINCE " +
		time.Now().Add(-24*time.Hour).UTC().Format("02-Jan-2006") + "\r\n"
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("send search replies: %w", err)
	}

	return s.readSearchResults(conn)
}

func (s *Scanner) readSearchResults(conn *tls.Conn) ([]string, error) {
	resp := make([]byte, 8192)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("read search results: %w", err)
	}

	response := string(resp[:n])

	// Parse SEARCH response: "* SEARCH 1 2 3"
	var ids []string
	if strings.Contains(response, "SEARCH") {
		parts := strings.Fields(response)
		for _, part := range parts {
			if isNumeric(part) {
				ids = append(ids, part)
			}
		}
	}

	return ids, nil
}

func (s *Scanner) processBounce(conn *tls.Conn, msgID string) error {
	// Fetch the message body to extract the original Message-ID
	cmd := fmt.Sprintf("a005 FETCH %s BODY[HEADER.FIELDS (MESSAGE-ID)]\r\n", msgID)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("fetch bounce headers: %w", err)
	}

	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("read bounce headers: %w", err)
	}

	response := string(resp[:n])

	// Extract original Message-ID from bounce body
	// Gmail bounces include the original message text
	originalMsgID := extractMessageID(response)
	if originalMsgID == "" {
		fmt.Printf("[imap] could not extract original message ID from bounce %s", msgID)
		return nil
	}

	// Classify bounce type
	bounceType := classifyBounce(response)

	// Update database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.db.MarkBounced(ctx, originalMsgID, bounceType); err != nil {
		return fmt.Errorf("mark bounced %s: %w", originalMsgID, err)
	}

	fmt.Printf("[imap] processed bounce: msg=%s, type=%s", originalMsgID, bounceType)
	return nil
}

func (s *Scanner) processReply(conn *tls.Conn, msgID string) error {
	// Fetch headers to get In-Reply-To and Message-ID
	cmd := fmt.Sprintf("a006 FETCH %s BODY[HEADER.FIELDS (MESSAGE-ID IN-REPLY-TO REFERENCES)]\r\n", msgID)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("fetch reply headers: %w", err)
	}

	resp := make([]byte, 4096)
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("read reply headers: %w", err)
	}

	response := string(resp[:n])

	// Extract In-Reply-To or References to find original message
	originalMsgID := extractInReplyTo(response)
	if originalMsgID == "" {
		fmt.Printf("[imap] could not extract In-Reply-To from reply %s", msgID)
		return nil
	}

	// Update database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.db.MarkReplied(ctx, originalMsgID); err != nil {
		return fmt.Errorf("mark replied %s: %w", originalMsgID, err)
	}

	fmt.Printf("[imap] processed reply to: %s", originalMsgID)
	return nil
}

func (s *Scanner) markSeen(conn *tls.Conn, ids []string) error {
	idList := strings.Join(ids, ",")
	cmd := fmt.Sprintf("a007 STORE %s +FLAGS (\\Seen)\r\n", idList)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}

	resp := make([]byte, 4096)
	_, err := conn.Read(resp)
	return err
}

// ─── Helper Functions ──────────────────────────────────────────────

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// extractMessageID finds a Message-ID header in raw email text.
func extractMessageID(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "message-id:") {
			val := strings.TrimPrefix(line, "Message-ID:")
			val = strings.TrimPrefix(val, "message-id:")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, "<>")
			return val
		}
	}
	return ""
}

// extractInReplyTo finds an In-Reply-To or References header.
func extractInReplyTo(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "in-reply-to:") {
			val := strings.TrimPrefix(line, "In-Reply-To:")
			val = strings.TrimPrefix(val, "in-reply-to:")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, "<>")
			if val != "" {
				return val
			}
		}
		if strings.HasPrefix(lower, "references:") {
			val := strings.TrimPrefix(line, "References:")
			val = strings.TrimPrefix(val, "references:")
			val = strings.TrimSpace(val)
			// Take the last message ID from references
			parts := strings.Fields(val)
			if len(parts) > 0 {
				last := parts[len(parts)-1]
				last = strings.Trim(last, "<>")
				return last
			}
		}
	}
	return ""
}

// classifyBounce determines the type of bounce from the email body.
func classifyBounce(body string) string {
	bodyLower := strings.ToLower(body)

	switch {
	case strings.Contains(bodyLower, "address rejected"):
		return "hard_bounce"
	case strings.Contains(bodyLower, "mailbox full"):
		return "soft_bounce"
	case strings.Contains(bodyLower, "user unknown"):
		return "hard_bounce"
	case strings.Contains(bodyLower, "no such user"):
		return "hard_bounce"
	case strings.Contains(bodyLower, "temporary failure"):
		return "soft_bounce"
	case strings.Contains(bodyLower, "suspected spam"):
		return "spam_blocked"
	case strings.Contains(bodyLower, "quota exceeded"):
		return "soft_bounce"
	case strings.Contains(bodyLower, "domain not found"):
		return "hard_bounce"
	default:
		return "unknown_bounce"
	}
}
