package mcpconn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
)

// The HTTP transport for one connection (story 0: the SDK gets no
// OAuthHandler). It adds the bearer token only over https to the
// connection's own host, re-sends a request exactly once after a 401
// answered by a successful refresh, never re-sends otherwise (redirects
// included), and records whether each request reached the server so a
// failed call can be reported as not sent, rejected, or of unknown outcome.

// maxRequestBody bounds the copy kept for the single re-send.
const maxRequestBody = 10 << 20

// ErrRedirect refuses a redirect: following one would re-send a POST.
var ErrRedirect = errors.New("the MCP server answered with a redirect; Evie does not follow MCP redirects")

// TokenSource supplies a connection's bearer tokens. Refresh is called
// with the token the server rejected; if another caller already replaced
// it, the replacement is returned without refreshing again.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Refresh(ctx context.Context, rejected string) (string, error)
}

type transport struct {
	base   http.RoundTripper
	host   string
	tokens TokenSource
}

// NewTransport wraps base for the connection whose server is at host
// (host[:port] as it appears in the URL).
func NewTransport(base http.RoundTripper, host string, tokens TokenSource) http.RoundTripper {
	return &transport{base: base, host: strings.ToLower(host), tokens: tokens}
}

// NewHTTPClient is the connection's client: the transport above, and no
// redirect following. Timeouts come from each call's context.
func NewHTTPClient(base http.RoundTripper, host string, tokens TokenSource) *http.Client {
	return &http.Client{
		Transport:     NewTransport(base, host, tokens),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Response != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, ErrRedirect
	}
	var body []byte
	if req.Body != nil && req.Body != http.NoBody {
		read, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBody+1))
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(read) > maxRequestBody {
			return nil, fmt.Errorf("request body exceeds %d bytes", maxRequestBody)
		}
		body = read
	}
	if req.URL.Scheme != "https" || strings.ToLower(req.URL.Host) != t.host {
		return t.send(req, body, "")
	}
	token, err := t.tokens.Token(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.send(req, body, token)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	// The server rejected the request before acting on it; one refresh
	// and one re-send are safe. Anything else stands.
	refreshed, refreshErr := t.tokens.Refresh(req.Context(), token)
	if refreshErr != nil {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxRequestBody))
	resp.Body.Close()
	return t.send(req, body, refreshed)
}

// send hands net/http a fresh request with no GetBody, so net/http cannot
// replay it on its own, and records this attempt in the call's trace.
func (t *transport) send(original *http.Request, body []byte, token string) (*http.Response, error) {
	ctx := original.Context()
	trace := sendTraceFrom(ctx)
	if trace != nil {
		trace.reset()
		ctx = httptrace.WithClientTrace(ctx, trace.clientTrace())
	}
	req := original.Clone(ctx)
	req.Header = original.Header.Clone()
	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	} else {
		req.Body = nil
		req.ContentLength = 0
	}
	req.GetBody = nil
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Del("Authorization")
	}
	resp, err := t.base.RoundTrip(req)
	if trace != nil && resp != nil {
		trace.answered(resp.StatusCode)
	}
	return resp, err
}

// SendOutcome classifies a call's last HTTP request.
type SendOutcome string

const (
	OutcomeSent     SendOutcome = "sent"
	OutcomeNotSent  SendOutcome = "not_sent"
	OutcomeRejected SendOutcome = "rejected"
	OutcomeUnknown  SendOutcome = "unknown"
)

// SendTrace records, for the latest request made under its context,
// whether a connection was obtained and what status answered it.
type SendTrace struct {
	mu      sync.Mutex
	gotConn bool
	status  int
}

type sendTraceKey struct{}

// WithSendTrace returns a context whose HTTP requests through this
// package's transport are traced.
func WithSendTrace(ctx context.Context) (context.Context, *SendTrace) {
	trace := &SendTrace{}
	return context.WithValue(ctx, sendTraceKey{}, trace), trace
}

func sendTraceFrom(ctx context.Context) *SendTrace {
	trace, _ := ctx.Value(sendTraceKey{}).(*SendTrace)
	return trace
}

func (s *SendTrace) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotConn, s.status = false, 0
}

func (s *SendTrace) answered(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *SendTrace) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) {
		s.mu.Lock()
		s.gotConn = true
		s.mu.Unlock()
	}}
}

// Outcome classifies a call that ended with err:
//   - no error: sent;
//   - no connection obtained: the request never left;
//   - a complete 4xx answer: the server refused it without acting;
//   - anything else (no answer, a broken stream, a 5xx from a gateway
//     that may have forwarded it): the server may have acted.
func (s *SendTrace) Outcome(err error) SendOutcome {
	if err == nil {
		return OutcomeSent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.gotConn:
		return OutcomeNotSent
	case s.status >= 400 && s.status < 500:
		return OutcomeRejected
	default:
		return OutcomeUnknown
	}
}
