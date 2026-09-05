package localextractor_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localextractor"
)

func TestEndpointUnavailableKeepsTypedReasonAndReleaseDisposition(t *testing.T) {
	for _, kind := range []string{"closed", "503", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			g := generation(map[string]any{"tokenizer.ggml.model": "gpt2"})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
			defer server.Close()
			adapter, err := localextractor.New(localextractor.Config{Endpoint: server.URL, Generation: g})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if kind == "closed" {
				server.Close()
			}
			if kind == "cancelled" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			result, err := adapter.Extract(ctx, g, request(g))
			if !errors.Is(err, eviedb.ErrCompilerEndpointUnavailable) || result.ReleaseEvidence != "not_dispatched" {
				t.Fatalf("unavailable %+v %v", result, err)
			}
			if kind == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation identity lost: %v", err)
			}
		})
	}
}
