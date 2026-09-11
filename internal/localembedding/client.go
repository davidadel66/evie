// Package localembedding implements the selected local-only Ollama embedding
// protocol. Similarity output is untrusted candidate input to the Kernel.
package localembedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	Model            = "all-minilm:22m"
	ManifestSHA256   = "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"
	ModelLayerSHA256 = "797b70c4edf85907fe0a49eb85811256f65fa0f7bf52166b147fd16be2be4662"
	Dimensions       = 384
	BatchSize        = 16
	InputBytes       = 1024
	ResponseBytes    = 1 << 20
)

var ErrDisabled = errors.New("local memory embedding is disabled")

type Client struct {
	http      *http.Client
	transport *http.Transport
	base      string
}

func New(endpoint string) (*Client, error) {
	var network, address, base string
	if strings.HasPrefix(endpoint, "unix:") {
		address = strings.TrimPrefix(endpoint, "unix:")
		if !strings.HasPrefix(address, "/") || strings.ContainsAny(address, "\x00\r\n") {
			return nil, errors.New("embedding endpoint requires an absolute Unix socket")
		}
		network, base = "unix", "http://localhost"
	} else {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("embedding endpoint requires plain loopback HTTP")
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() || u.RawPath != "" {
			return nil, errors.New("embedding endpoint requires a literal loopback address")
		}
		port := u.Port()
		if port == "" {
			port = "80"
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("embedding endpoint has an invalid port")
		}
		network, address = "tcp", net.JoinHostPort(ip.String(), port)
		base = "http://" + address
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
		DisableCompression:     true,
		MaxConnsPerHost:        1,
		MaxIdleConns:           1,
		MaxResponseHeaderBytes: 16 << 10,
	}
	return &Client{base: base, transport: transport, http: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("embedding redirects are forbidden")
	}}}, nil
}

func (c *Client) Close() { c.transport.CloseIdleConnections() }

func (c *Client) request(ctx context.Context, method, path string, body []byte, output any) error {
	if os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
		return ErrDisabled
	}
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("local embedding request unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("local embedding HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, ResponseBytes+1))
	if err != nil {
		return fmt.Errorf("local embedding response unavailable: %w", err)
	}
	if len(data) > ResponseBytes {
		return errors.New("local embedding response oversized")
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errors.New("malformed local embedding response")
	}
	return nil
}

func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if len(inputs) == 0 || len(inputs) > BatchSize {
		return nil, errors.New("embedding batch exceeds its bound")
	}
	for _, text := range inputs {
		if text == "" || len(text) > InputBytes || !utf8.ValidString(text) {
			return nil, errors.New("embedding input is invalid or oversized")
		}
	}
	if err := c.verifyManifest(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		Model     string         `json:"model"`
		Input     []string       `json:"input"`
		Truncate  bool           `json:"truncate"`
		KeepAlive string         `json:"keep_alive"`
		Options   map[string]int `json:"options"`
	}{Model, inputs, false, "30s", map[string]int{"num_thread": 4}})
	if err != nil {
		return nil, err
	}
	var result struct {
		Model      string      `json:"model"`
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/embed", payload, &result); err != nil {
		return nil, err
	}
	if result.Model != Model || len(result.Embeddings) != len(inputs) {
		return nil, errors.New("local embedding model or result count mismatch")
	}
	var vectors [][]float32
	for _, values := range result.Embeddings {
		if len(values) != Dimensions {
			return nil, errors.New("local embedding dimensions mismatch")
		}
		norm := 0.0
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errors.New("local embedding contains a nonfinite value")
			}
			norm += value * value
		}
		if norm < 1e-24 || math.IsInf(norm, 0) {
			return nil, errors.New("local embedding has invalid magnitude")
		}
		norm = math.Sqrt(norm)
		vector := make([]float32, Dimensions)
		for i, value := range values {
			vector[i] = float32(value / norm)
		}
		vectors = append(vectors, vector)
	}
	if err := c.verifyManifest(ctx); err != nil {
		return nil, err
	}
	return vectors, nil
}

func (c *Client) verifyManifest(ctx context.Context) error {
	// A mutable model tag cannot silently replace the selected generation.
	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/tags", nil, &tags); err != nil {
		return err
	}
	matched := false
	for _, model := range tags.Models {
		matched = matched || model.Name == Model && strings.TrimPrefix(model.Digest, "sha256:") == ManifestSHA256
	}
	if !matched {
		return errors.New("local embedding model does not match the selected manifest")
	}
	return nil
}
