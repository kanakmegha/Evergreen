package main

import (
	"testing"

	"everroot/internal/ai"
)

// /api/v1/explain and /api/v1/questions answer inside the request, so the write
// deadline has to outlast a single inference round trip. Before this guard the
// server allowed 15s while the provider client allowed 25s, and a slow answer
// was killed mid-flight: the browser received an aborted connection (HTTP 000)
// rather than a response.
func TestWriteTimeoutExceedsProviderTimeout(t *testing.T) {
	if writeTimeout() <= ai.ProviderTimeout {
		t.Fatalf("write timeout %s must exceed the provider timeout %s", writeTimeout(), ai.ProviderTimeout)
	}
}
