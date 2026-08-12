// export_test.go exposes unexported Client fields to the msg91_test package.
// This file is compiled only during go test; it adds nothing to the production binary.
package msg91

import (
	"net/http"
	"time"
)

// ClientBaseURL returns the base URL configured on c.
func ClientBaseURL(c *Client) string { return c.baseURL }

// ClientTimeout returns the timeout configured on c's underlying HTTP client.
func ClientTimeout(c *Client) time.Duration { return c.httpClient.Timeout }

// ClientHasIPv4Dialer reports whether c's HTTP transport has a custom
// DialContext set. The production client forces tcp4 so that calls always egress
// from the whitelisted IPv4 address — deleting that dialer silently reintroduces
// IPv6 egress and MSG91 will reject every call with code 418.
func ClientHasIPv4Dialer(c *Client) bool {
	t, ok := c.httpClient.Transport.(*http.Transport)
	return ok && t.DialContext != nil
}
