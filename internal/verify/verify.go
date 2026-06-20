// Package verify provides email verification via DNS lookups.
package verify

import (
	"net"
	"strings"
)

// Email checks that the email's domain can receive mail.
// Uses RFC 5321 logic: checks MX records first, falls back to A/AAAA.
func Email(email string) bool {
	_, domain, ok := strings.Cut(email, "@")
	if !ok || domain == "" {
		return false
	}
	// RFC 5321 §5.1: try MX first
	mxs, err := net.LookupMX(domain)
	if err == nil && len(mxs) > 0 {
		return true
	}
	// Fall back: if domain has an A/AAAA record, it can receive mail
	addrs, err := net.LookupHost(domain)
	return err == nil && len(addrs) > 0
}
