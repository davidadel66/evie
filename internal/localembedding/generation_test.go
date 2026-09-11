package localembedding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/davidadel66/evie/internal/localembedding"
)

func TestEmbedRejectsModelReplacementDuringInference(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	var replaced atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			if !replaced.Load() {
				writeSelectedManifest(w)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": localembedding.Model, "digest": "replacement-model"}}})
			return
		}
		replaced.Store(true)
		json.NewEncoder(w).Encode(map[string]any{"model": localembedding.Model, "embeddings": [][]float64{fixtureVector(1, 0)}})
	}))
	defer server.Close()
	client, err := localembedding.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Embed(context.Background(), []string{"original source"}); err == nil {
		t.Fatal("a changed local model was accepted into the selected immutable generation")
	}
}
