// Package localextractor implements the explicitly configured, pinned local
// compiler transport. It has no default model or installed live configuration.
package localextractor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"text/template"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type Config struct {
	Endpoint   string                    `json:"endpoint"`
	Generation memory.CompilerGeneration `json:"generation"`
}
type Ollama struct {
	endpoint, identity, generationID string
	client                           *http.Client
}

func New(config Config) (*Ollama, error) {
	id, _, err := memory.CompilerGenerationIdentity(config.Generation)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme != "http" || ip == nil || !ip.IsLoopback() || endpoint.Port() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("compiler endpoint must be explicit literal loopback HTTP")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, DisableKeepAlives: true, ResponseHeaderTimeout: 120 * time.Second}
	return &Ollama{endpoint: config.Endpoint, identity: config.Endpoint + "#" + config.Generation.RuntimeVersion + "#" + config.Generation.ModelManifestSHA256, generationID: id, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("compiler redirects forbidden") }}}, nil
}
func (o *Ollama) ServerIdentity() string { return o.identity }
func (o *Ollama) Extract(ctx context.Context, g memory.CompilerGeneration, request memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	noDispatch := eviedb.CompilerExtraction{ReleaseEvidence: "not_dispatched"}
	id, _, err := memory.CompilerGenerationIdentity(g)
	if err != nil || id != o.generationID || request.GenerationID != id {
		return noDispatch, fmt.Errorf("%w: compiler generation identity mismatch", eviedb.ErrCompilerConfiguration)
	}
	if err := memory.CompilerInputBudget(g, request); err != nil {
		return noDispatch, err
	}
	if err := o.VerifyCompilerConfiguration(ctx, g); err != nil {
		return noDispatch, err
	}
	prompt, err := render(g, request)
	if err != nil {
		return noDispatch, err
	}
	body := map[string]any{"model": g.ModelArtifact, "prompt": prompt, "raw": true, "stream": false, "format": g.Schema, "options": map[string]any{"num_ctx": g.Decoding.ContextTokens, "num_predict": g.Decoding.OutputTokens, "seed": g.Decoding.Seed, "temperature": g.Decoding.Temperature}}
	encoded, err := json.Marshal(body)
	if err != nil {
		return noDispatch, err
	}
	if len(encoded) > memory.CompilerMaxBytes {
		return noDispatch, errors.New("serialized transport input limit")
	}
	var response struct {
		Model      string  `json:"model"`
		Response   *string `json:"response"`
		Done       bool    `json:"done"`
		DoneReason string  `json:"done_reason"`
		Error      string  `json:"error"`
	}
	if err := o.call(ctx, "POST", "/api/generate", body, &response); err != nil {
		return eviedb.CompilerExtraction{}, err
	}
	if !response.Done {
		return eviedb.CompilerExtraction{}, errors.New("missing runtime completion marker")
	}
	result := eviedb.CompilerExtraction{ReleaseEvidence: "completed"}
	if response.Model != g.ModelArtifact || response.Response == nil || response.Error != "" || response.DoneReason != "stop" {
		return result, errors.New("invalid or truncated runtime output")
	}
	result.Raw = []byte(*response.Response)
	return result, nil
}
func (o *Ollama) call(ctx context.Context, method, path string, body any, target any) error {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, o.endpoint+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := o.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	limit := memory.CompilerMaxBytes
	if path == "/api/show" {
		limit = memory.CompilerMetadataMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return errors.New("runtime response limit")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("local runtime HTTP %d", response.StatusCode)
	}
	// Runtime metadata may add fields. Strictness applies to duplicate keys and
	// framing here; the Kernel separately closes the model-output shape.
	if path == "/api/show" {
		if err := memory.ValidateCompilerMetadataJSON(data); err != nil {
			return err
		}
	} else if err := memory.ValidateCompilerJSON(data); err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

type boundedWriter struct{ bytes.Buffer }

func (w *boundedWriter) Write(p []byte) (int, error) {
	if w.Len()+len(p) > memory.CompilerMaxBytes {
		return 0, errors.New("rendered input limit")
	}
	return w.Buffer.Write(p)
}
func render(g memory.CompilerGeneration, r memory.CompilerRequest) (string, error) {
	input, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	prompt := string(input) + "\nExtraction schema:\n" + string(g.Schema)
	parsed, err := template.New("pinned").Option("missingkey=error").Parse(g.Template)
	if err != nil {
		return "", err
	}
	data := map[string]any{"System": g.Prompt, "Prompt": prompt, "Response": "", "Messages": []map[string]any{{"Role": "system", "Content": g.Prompt}, {"Role": "user", "Content": prompt}}, "Tools": nil, "Thinking": false}
	var output boundedWriter
	if err := parsed.Execute(&output, data); err != nil {
		return "", err
	}
	bound := int64(output.Len())*int64(g.TokensPerByte) + int64(g.TemplateTokenOverhead) + int64(g.Decoding.OutputTokens)
	if bound > int64(g.Decoding.ContextTokens) {
		return "", errors.New("rendered proven context limit")
	}
	return output.String(), nil
}
