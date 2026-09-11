package localembedding_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/localembedding"
)

func writeSelectedManifest(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"models":[{"name":"all-minilm:22m","digest":"1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"}]}`)
}

func fixtureVector(first, second float64) []float64 {
	vector := make([]float64, 384)
	vector[0], vector[1] = first, second
	return vector
}

func TestEmbedUsesSelectedProtocolAndNormalizesFiniteVectors(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	inputs := []string{"garden café", "second original statement"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			writeSelectedManifest(w)
		case "/api/embed":
			var body struct {
				Model     string         `json:"model"`
				Input     []string       `json:"input"`
				Truncate  *bool          `json:"truncate"`
				KeepAlive string         `json:"keep_alive"`
				Options   map[string]int `json:"options"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "" || body.Model != "all-minilm:22m" || !reflect.DeepEqual(body.Input, inputs) || body.Truncate == nil || *body.Truncate || body.KeepAlive != "30s" || body.Options["num_thread"] != 4 {
				t.Error("request changed selected model, original input, explicit nontruncation, or local inference settings")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "all-minilm:22m", "embeddings": [][]float64{fixtureVector(3, 4), fixtureVector(-5, 0)}})
		default:
			t.Error("unexpected local model endpoint")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := localembedding.New(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	vectors, err := client.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 384 || len(vectors[1]) != 384 || vectors[0][0] != .6 || vectors[0][1] != .8 || vectors[1][0] != -1 {
		t.Fatal("known three-four-five vectors did not normalize to their expected directions")
	}
	for _, vector := range vectors {
		for _, component := range vector[2:] {
			if component != 0 {
				t.Fatal("normalization invented an absent component")
			}
		}
	}
}

func TestNewRejectsRemoteNamedCredentialedAndNonRootEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://192.0.2.1:11434", "http://8.8.8.8:11434", "http://localhost:11434", "http://example.invalid:11434",
		"https://127.0.0.1:11434", "ftp://127.0.0.1:11434", "http://user:password@127.0.0.1:11434",
		"http://127.0.0.1:11434/api", "http://127.0.0.1:11434/%2f", "http://127.0.0.1:11434?mode=x", "http://127.0.0.1:11434?", "http://127.0.0.1:11434#fragment",
		"http://127.0.0.1:0", "http://127.0.0.1:65536", "http://127.0.0.1:bad", "http://[::]:11434", "http://[fe80::1%25en0]:11434",
		"unix:relative.sock", "unix:/tmp/invalid\nname.sock", "unix:/tmp/invalid\x00name.sock", "",
	} {
		t.Run(endpoint, func(t *testing.T) {
			client, err := localembedding.New(endpoint)
			if err == nil {
				client.Close()
				t.Fatal("invalid embedding endpoint was accepted")
			}
			if client != nil {
				t.Fatal("invalid endpoint returned a usable client")
			}
		})
	}
}

func TestEmbedRejectsRedirectWithoutContactingItsDestination(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	for _, redirectPath := range []string{"/api/tags", "/api/embed"} {
		t.Run(redirectPath, func(t *testing.T) {
			var destinationRequests atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				destinationRequests.Add(1)
				writeSelectedManifest(w)
			}))
			defer destination.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == redirectPath {
					http.Redirect(w, r, destination.URL+r.URL.Path, http.StatusTemporaryRedirect)
					return
				}
				writeSelectedManifest(w)
			}))
			defer source.Close()
			client, err := localembedding.New(source.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if vectors, err := client.Embed(context.Background(), []string{"original garden words"}); err == nil || vectors != nil {
				t.Fatal("redirected local embedding was accepted")
			}
			if destinationRequests.Load() != 0 {
				t.Fatal("embedding followed the redirect to a different destination")
			}
		})
	}
}

func TestEmbedUsesUnixSocketAndDoesNotContactConfiguredProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("selected local embedding Unix-socket transport is exercised on Unix platforms")
	}
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	var proxyRequests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests.Add(1)
		http.Error(w, "proxy must not receive local embedding traffic", http.StatusBadGateway)
	}))
	defer proxy.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		t.Setenv(key, proxy.URL)
	}
	t.Setenv("NO_PROXY", "")
	// Keep the path within macOS's Unix-socket address length limit.
	directory, err := os.MkdirTemp("/tmp", "evie-embedding-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "model.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			writeSelectedManifest(w)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "all-minilm:22m", "embeddings": [][]float64{fixtureVector(3, 4)}})
	})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		_ = server.Close()
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Unix HTTP server failed: %v", err)
		}
	}()
	client, err := localembedding.New("unix:" + socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	vectors, err := client.Embed(context.Background(), []string{"local socket input"})
	if err != nil || len(vectors) != 1 || vectors[0][0] != .6 || vectors[0][1] != .8 {
		t.Fatalf("Unix embedding did not return its expected vector: %v", err)
	}
	if proxyRequests.Load() != 0 {
		t.Fatal("local embedding was forwarded to the configured proxy")
	}
}

func TestEmbedPreservesDeadlineWhileReadingResponse(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[`))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := localembedding.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Embed(ctx, []string{"garden experiment"})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("local response did not begin")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("caller cannot identify response-body deadline: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("local response cancellation did not finish promptly")
	}
}

func TestEmbedEnforcesSelectedDeadlineWithoutCallerDeadline(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	var embeddingStarted atomic.Bool
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			// Spend part of the one operation deadline verifying the model. A
			// separate deadline restarted for /api/embed would exceed the bound.
			timer := time.NewTimer(4 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				writeSelectedManifest(w)
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		embeddingStarted.Store(true)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	client, err := localembedding.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Embed(ctx, []string{"garden experiment"})
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) || !embeddingStarted.Load() {
			t.Fatalf("selected ten-second deadline did not classify the stalled endpoint: %v", err)
		}
	case <-time.After(12 * time.Second):
		cancel()
		<-result
		t.Fatal("embedding exceeded selected ten-second bound without a caller deadline")
	}
}

func TestEmbedOptOutMakesNoRequestAndStopsAfterManifestVerification(t *testing.T) {
	t.Run("already disabled", func(t *testing.T) {
		t.Setenv("EVIE_REMOTE_MEMORY", "off")
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			writeSelectedManifest(w)
		}))
		defer server.Close()
		client, err := localembedding.New(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		vectors, err := client.Embed(context.Background(), []string{"private original statement"})
		if !errors.Is(err, localembedding.ErrDisabled) || vectors != nil || requests.Load() != 0 {
			t.Fatalf("disabled embedding contacted the endpoint or lost disabled classification: %v", err)
		}
	})
	t.Run("disabled while manifest request is in flight", func(t *testing.T) {
		t.Setenv("EVIE_REMOTE_MEMORY", "on")
		manifestStarted := make(chan struct{})
		releaseManifest := make(chan struct{})
		var embedRequests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				close(manifestStarted)
				select {
				case <-releaseManifest:
				case <-r.Context().Done():
					return
				}
				writeSelectedManifest(w)
				return
			}
			embedRequests.Add(1)
			http.Error(w, "input must remain local after opt-out", http.StatusForbidden)
		}))
		defer server.Close()
		client, err := localembedding.New(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			vectors, err := client.Embed(ctx, []string{"private original statement"})
			if vectors != nil {
				t.Error("disabled embedding returned vectors")
			}
			result <- err
		}()
		select {
		case <-manifestStarted:
		case <-ctx.Done():
			t.Fatal("manifest verification did not start")
		}
		t.Setenv("EVIE_REMOTE_MEMORY", "off")
		close(releaseManifest)
		if err := <-result; !errors.Is(err, localembedding.ErrDisabled) || embedRequests.Load() != 0 {
			t.Fatalf("opt-out failed to prevent the input-bearing request: %v", err)
		}
	})
}

func TestEmbedRejectsInvalidOrOversizedInputsBeforeContactingEndpoint(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	for _, fixture := range []struct {
		name   string
		inputs []string
	}{
		{"empty batch", nil},
		{"seventeen inputs", strings.Fields(strings.Repeat("input ", 17))},
		{"empty input after valid input", []string{"valid input", ""}},
		{"invalid UTF8 after valid input", []string{"valid input", string([]byte{0xff})}},
		{"ASCII byte limit", []string{strings.Repeat("a", 1025)}},
		{"UTF8 byte limit", []string{strings.Repeat("é", 513)}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "invalid inputs must not reach the endpoint", http.StatusBadRequest)
			}))
			defer server.Close()
			client, err := localembedding.New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			vectors, err := client.Embed(context.Background(), fixture.inputs)
			if err == nil || vectors != nil || requests.Load() != 0 {
				t.Fatal("invalid batch reached the endpoint or returned vectors")
			}
		})
	}
}

func TestEmbedRejectsUnselectedManifestBeforeSendingInput(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	for _, fixture := range []struct {
		name string
		tags string
	}{
		{"model absent", `{"models":[]}`},
		{"same name different digest", `{"models":[{"name":"all-minilm:22m","digest":"changed-model-digest"}]}`},
		{"same digest different name", `{"models":[{"name":"different-model:22m","digest":"1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"}]}`},
		{"malformed manifest", `{"models":[`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var inputRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					_, _ = io.WriteString(w, fixture.tags)
					return
				}
				inputRequests.Add(1)
				http.Error(w, "unselected model must not receive input", http.StatusForbidden)
			}))
			defer server.Close()
			client, err := localembedding.New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			vectors, err := client.Embed(context.Background(), []string{"original source text"})
			if err == nil || vectors != nil || inputRequests.Load() != 0 {
				t.Fatal("unselected or invalid model manifest allowed input dispatch")
			}
		})
	}
}

func TestEmbedRejectsMalformedVectorsWithoutReturningPartialBatch(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	encoded := func(model string, vectors [][]float64) string {
		data, err := json.Marshal(map[string]any{"model": model, "embeddings": vectors})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	valid := encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), fixtureVector(3, 4)})
	for _, fixture := range []struct {
		name string
		body string
	}{
		{"wrong model", encoded("different-model", [][]float64{fixtureVector(3, 4), fixtureVector(3, 4)})},
		{"missing result", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4)})},
		{"extra result", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), fixtureVector(3, 4), fixtureVector(3, 4)})},
		{"missing dimensions after valid vector", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), nil})},
		{"too few dimensions after valid vector", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), fixtureVector(3, 4)[:383]})},
		{"too many dimensions after valid vector", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), append(fixtureVector(3, 4), 0)})},
		{"zero vector after valid vector", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), fixtureVector(0, 0)})},
		{"underflow magnitude after valid vector", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), fixtureVector(1e-15, 0)})},
		{"overflow magnitude after valid vector", encoded("all-minilm:22m", [][]float64{fixtureVector(3, 4), fixtureVector(1e308, 0)})},
		// JSON has no nonfinite numeric literals. The adapter must also reject
		// malformed spellings and numeric overflow before any vector is usable.
		{"NaN JSON", strings.Replace(valid, "[3,4,", "[NaN,4,", 1)},
		{"Infinity JSON", strings.Replace(valid, "[3,4,", "[Infinity,4,", 1)},
		{"numeric overflow JSON", strings.Replace(valid, "[3,4,", "[1e309,4,", 1)},
		{"truncated JSON", valid[:len(valid)-1]},
		{"trailing JSON", valid + `{}`},
		{"oversized JSON", valid[:len(valid)-1] + `,"padding":"` + strings.Repeat("x", 1<<20) + `"}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					writeSelectedManifest(w)
					return
				}
				_, _ = io.WriteString(w, fixture.body)
			}))
			defer server.Close()
			client, err := localembedding.New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			vectors, err := client.Embed(context.Background(), []string{"first original statement", "second original statement"})
			if err == nil || vectors != nil {
				t.Fatal("malformed embedding response returned a complete or partial vector batch")
			}
		})
	}
}

func TestEmbedPreservesCancellationAndAvoidsDispatchForCanceledCaller(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Run("canceled before dispatch", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			writeSelectedManifest(w)
		}))
		defer server.Close()
		client, err := localembedding.New(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		vectors, err := client.Embed(ctx, []string{"original input"})
		if !errors.Is(err, context.Canceled) || vectors != nil || requests.Load() != 0 {
			t.Fatalf("already canceled request was dispatched or lost its cause: %v", err)
		}
	})
	t.Run("canceled during input-bearing response", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				writeSelectedManifest(w)
				return
			}
			_, _ = io.WriteString(w, `{"model":"all-minilm:22m","embeddings":[`)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		}))
		defer server.Close()
		client, err := localembedding.New(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			vectors, err := client.Embed(ctx, []string{"original input"})
			if vectors != nil {
				t.Error("canceled embedding returned vectors")
			}
			result <- err
		}()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("input-bearing response did not start")
		}
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("embedding response lost cancellation classification: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("canceled input-bearing response did not finish promptly")
		}
	})
}

func TestEmbedReportsUnavailableEndpointsWithoutEchoingResponseContent(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Run("HTTP error", func(t *testing.T) {
		const withheld = "original-source-body-must-not-appear-in-error"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				writeSelectedManifest(w)
				return
			}
			http.Error(w, withheld, http.StatusInternalServerError)
		}))
		defer server.Close()
		client, err := localembedding.New(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		vectors, err := client.Embed(context.Background(), []string{withheld})
		if err == nil || vectors != nil || strings.Contains(err.Error(), withheld) {
			t.Fatal("unavailable endpoint returned vectors or exposed its response content")
		}
	})
	t.Run("closed loopback endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		endpoint := server.URL
		server.Close()
		client, err := localembedding.New(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if vectors, err := client.Embed(ctx, []string{"original input"}); err == nil || vectors != nil {
			t.Fatal("closed local endpoint unexpectedly returned an embedding")
		}
	})
}
