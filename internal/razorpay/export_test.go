// export_test.go exposes unexported Client fields to the razorpay_test package.
// This file is compiled only during go test; it adds nothing to the production binary.
package razorpay

import "time"

// ClientBaseURL returns the base URL configured on c.
func ClientBaseURL(c *Client) string { return c.baseURL }

// ClientTimeout returns the timeout configured on c's underlying HTTP client.
func ClientTimeout(c *Client) time.Duration { return c.httpClient.Timeout }
