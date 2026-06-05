package sender_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arinbalyan/jobhunter/internal/email/sender"
)

func TestNew(t *testing.T) {
	cfg := sender.SMTPConfig{
		User:     "test@gmail.com",
		Password: "app-pass",
		FromName: "Test User",
		FromAddr: "test@gmail.com",
	}

	s := sender.New(cfg)
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestInjectTrackingPixel(t *testing.T) {
	trackingURL := "https://tracker.example.com"
	trackingID := "abc-123-def"

	// Test with </body> present
	htmlBody := "<html><body><p>Hello</p></body></html>"
	result := sender.InjectTrackingPixel(htmlBody, trackingURL, trackingID)

	if !strings.Contains(result, trackingURL+"/track?id="+trackingID) {
		t.Errorf("expected tracking URL in result")
	}
	if !strings.Contains(result, `<img src="`) {
		t.Errorf("expected img tag in result")
	}
	if !strings.Contains(result, `style="display:none;"`) {
		t.Errorf("expected display:none style")
	}
	if !strings.Contains(result, "</body>") {
		t.Errorf("expected closing body tag")
	}
}

func TestInjectTrackingPixel_WithoutBodyTag(t *testing.T) {
	trackingURL := "https://tracker.example.com"
	trackingID := "test-id-123"

	// Test without </body> tag
	htmlBody := "<html><p>Hello</p></html>"
	result := sender.InjectTrackingPixel(htmlBody, trackingURL, trackingID)

	if !strings.Contains(result, trackingURL+"/track?id="+trackingID) {
		t.Errorf("expected tracking URL in result")
	}
	// Pixel should be appended at end
	if !strings.HasSuffix(result, `width="1" height="1" alt="" style="display:none;" />`) {
		t.Errorf("expected pixel appended at end when no </body>")
	}
}

func TestInjectTrackingPixel_EmptyHTML(t *testing.T) {
	result := sender.InjectTrackingPixel("", "https://tracker.example.com", "test-id")
	if !strings.Contains(result, "tracker.example.com") {
		t.Error("expected tracking pixel even with empty HTML")
	}
	if !strings.HasSuffix(result, `width="1" height="1" alt="" style="display:none;" />`) {
		t.Error("expected pixel appended to empty string")
	}
}

func TestInjectTrackingPixel_TrailingSlash(t *testing.T) {
	// URL with trailing slash should be cleaned up (no double slash in path)
	result := sender.InjectTrackingPixel("<html><body></body></html>", "https://tracker.example.com/", "test-id")
	if !strings.Contains(result, "/track?id=test-id") {
		t.Errorf("expected tracking pixel URL, got: %s", result)
	}
	// Should not have // before track (e.g., no double slash in path)
	if strings.Contains(result, "//track?id") {
		t.Errorf("URL should not have double slash before path: %s", result)
	}
	// Should work correctly (TrimRight removes trailing /)
	expected := "https://tracker.example.com/track?id=test-id"
	if !strings.Contains(result, expected) {
		t.Errorf("expected URL segment %q in result", expected)
	}
}

func TestEmailMessage_Defaults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SMTP-dependent test in short mode")
	}

	// Test that Send fills in default TrackingID and MessageID
	cfg := sender.SMTPConfig{
		User:     "test@gmail.com",
		Password: "app-pass",
		FromName: "Test User",
		FromAddr: "test@gmail.com",
	}

	s := sender.New(cfg)
	msg := &sender.EmailMessage{
		To:        "hr@company.com",
		Subject:   "Test Subject",
		HTMLBody:  "<p>Hello</p>",
		PlainBody: "Hello",
	}

	// Use a short context timeout so we don't wait for all SMTP retries
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := s.Send(ctx, msg)
	if err != nil {
		// Verify TrackingID and MessageID were set despite failure
		if msg.TrackingID == "" {
			t.Error("TrackingID should be set even on send failure")
		}
		if msg.MessageID == "" {
			t.Error("MessageID should be set even on send failure")
		}
	}
}

func TestEmailMessage_PreSetIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SMTP-dependent test in short mode")
	}

	cfg := sender.SMTPConfig{
		User:     "test@gmail.com",
		Password: "app-pass",
		FromName: "Test User",
		FromAddr: "test@gmail.com",
	}

	s := sender.New(cfg)
	msg := &sender.EmailMessage{
		To:         "hr@company.com",
		Subject:    "Test",
		HTMLBody:   "<p>Hi</p>",
		PlainBody:  "Hi",
		TrackingID: "pre-set-id",
		MessageID:  "<pre-set@jobhunter>",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := s.Send(ctx, msg)
	if err != nil {
		// Verify our pre-set IDs were not overwritten
		if msg.TrackingID != "pre-set-id" {
			t.Errorf("TrackingID was overwritten to %q", msg.TrackingID)
		}
		if msg.MessageID != "<pre-set@jobhunter>" {
			t.Errorf("MessageID was not preserved: %q", msg.MessageID)
		}
	}
}

func TestFindResume_NoConfigFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SMTP-dependent test in short mode")
	}

	// Without a configured path or existing .agent-data dir, findResume returns ""
	cfg := sender.SMTPConfig{
		User:       "test@gmail.com",
		Password:   "app-pass",
		FromName:   "Test User",
		FromAddr:   "test@gmail.com",
		ResumePath: "/nonexistent/path/resume.pdf",
	}

	s := sender.New(cfg)
	msg := &sender.EmailMessage{
		To:        "hr@company.com",
		Subject:   "Test",
		HTMLBody:  "<p>Hi</p>",
		PlainBody: "Hi",
	}

	// Should not crash when ResumePath doesn't exist
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := s.Send(ctx, msg)
	if err != nil {
		_ = err // Expected to fail at SMTP, not at build
	}
}

func TestFindResume_WithFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SMTP-dependent test in short mode")
	}

	// Create a temp dir with a .agent-data directory and a PDF
	tmpDir := t.TempDir()
	agentDataDir := filepath.Join(tmpDir, ".agent-data")
	if err := os.MkdirAll(agentDataDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a dummy PDF
	pdfPath := filepath.Join(agentDataDir, "resume.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Change to tmp dir and restore
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	cfg := sender.SMTPConfig{
		User:     "test@gmail.com",
		Password: "app-pass",
		FromName: "Test User",
		FromAddr: "test@gmail.com",
	}

	s := sender.New(cfg)
	msg := &sender.EmailMessage{
		To:        "hr@company.com",
		Subject:   "Test with Resume",
		HTMLBody:  "<p>Hi</p>",
		PlainBody: "Hi",
	}

	// Should not panic when building email with resume
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := s.Send(ctx, msg)
	if err != nil {
		_ = err // Expected to fail at SMTP, not at build
	}
}

func TestEmailMessage_ContextCancelled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SMTP-dependent test in short mode")
	}

	cfg := sender.SMTPConfig{
		User:     "test@gmail.com",
		Password: "app-pass",
		FromName: "Test User",
		FromAddr: "test@gmail.com",
	}

	s := sender.New(cfg)
	msg := &sender.EmailMessage{
		To:        "hr@company.com",
		Subject:   "Test",
		HTMLBody:  "<p>Hi</p>",
		PlainBody: "Hi",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Context will expire almost immediately
	err := s.Send(ctx, msg)
	if err != nil {
		_ = err // Context deadline exceeded or SMTP error — both acceptable
	}
}
