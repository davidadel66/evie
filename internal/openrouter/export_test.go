package openrouter

import "time"

// SetMetadataEndpointForTest is a test-only seam for the external
// openrouter_test package.
func SetMetadataEndpointForTest(c *Client, apiBaseURL string, discoveryTimeout time.Duration) {
	c.apiBaseURL = apiBaseURL
	c.contextDiscoveryTimeout = discoveryTimeout
}
