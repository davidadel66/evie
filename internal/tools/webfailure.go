package tools

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

// Web tool failures are described in the harness's own words. A server
// controls most of what Go's HTTP client quotes in its errors: an unparseable
// Location header (quoted twice, before CheckRedirect ever runs), a status
// reason phrase, a malformed status or header line, a certificate's names,
// the URL of a same-host redirect, and a media type. Error text is plain tool
// text, outside any untrusted frame, so passing it on would let a server
// close a frame that never opened and address the model as the harness, at
// any length. These helpers choose among fixed sentences by error type and
// status code; none of a server's text is copied.

// errTooManyRedirects is web_fetch's redirect-hop limit.
var errTooManyRedirects = errors.New("too many redirects")

// nonPublicAddressError is a delegated worker's refused dial; its text names
// only an address the dialer resolved, never server-supplied text.
type nonPublicAddressError struct{ detail string }

func (e *nonPublicAddressError) Error() string {
	return "research workers may fetch only public addresses; " + e.detail
}

// transportFailure describes why an HTTP exchange produced no response.
func transportFailure(err error) string {
	var refused *nonPublicAddressError
	var dns *net.DNSError
	var certificate *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var notTLS tls.RecordHeaderError
	var alert tls.AlertError
	// Some failures have no type, only fixed stdlib wording. That wording is
	// matched in the client's own error (not the request URL around it) to
	// pick a sentence; the text itself is never copied.
	text := err.Error()
	if wrapped := new(url.Error); errors.As(err, &wrapped) && wrapped.Err != nil {
		text = wrapped.Err.Error()
	}
	switch {
	case errors.As(err, &refused):
		return refused.Error()
	case errors.Is(err, errTooManyRedirects):
		return fmt.Sprintf("stopped after %d redirects", maxRedirects)
	case strings.HasPrefix(text, "failed to parse Location header"):
		return "the redirect target could not be parsed"
	case errors.As(err, &dns):
		if dns.IsNotFound {
			return "the host name could not be resolved"
		}
		return "the host name lookup failed"
	case errors.As(err, &certificate), errors.As(err, &unknownAuthority), errors.As(err, &hostname), errors.As(err, &invalid):
		return "the server's TLS certificate was not accepted"
	case errors.As(err, &notTLS):
		return "the server did not answer with TLS"
	case errors.As(err, &alert):
		return "the TLS handshake failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "the connection was refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "the connection was reset"
	case strings.Contains(text, "malformed"):
		return "the server sent a malformed HTTP response"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "the server closed the connection without a complete response"
	case errors.As(err, new(*net.OpError)):
		return "the connection failed"
	}
	return "the request failed"
}

// bodyReadFailure describes why a response body could not be read.
func bodyReadFailure(err error) string {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "the connection closed before the response body was complete"
	}
	return "the response body could not be read"
}

// httpStatus names a status code with Go's own reason text, never the
// server's reason phrase.
func httpStatus(code int) string {
	if text := http.StatusText(code); text != "" {
		return fmt.Sprintf("HTTP %d %s", code, text)
	}
	return fmt.Sprintf("HTTP %d", code)
}

// namedMediaTypes are refused media types an error may name exactly. Any
// other type is named by its registered top-level type, or not at all: the
// error then carries the harness's string, chosen by an exact match.
var namedMediaTypes = map[string]bool{
	"application/pdf": true, "application/octet-stream": true, "application/zip": true,
	"application/gzip": true, "application/x-tar": true, "application/wasm": true,
	"application/msword": true, "application/vnd.ms-excel": true, "application/vnd.ms-powerpoint": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/avif": true,
	"image/bmp": true, "image/tiff": true, "image/x-icon": true, "image/vnd.microsoft.icon": true,
	"audio/mpeg": true, "audio/ogg": true, "audio/wav": true, "audio/webm": true,
	"video/mp4": true, "video/webm": true, "video/ogg": true, "video/quicktime": true,
	"font/woff": true, "font/woff2": true, "font/ttf": true, "font/otf": true,
}

var topLevelMediaTypes = map[string]bool{
	"application": true, "audio": true, "font": true, "haptics": true, "image": true,
	"message": true, "model": true, "multipart": true, "text": true, "video": true,
}

// unsupportedMediaType refuses mediaType without echoing a server's text.
func unsupportedMediaType(mediaType string) error {
	const supported = "web_fetch reads text, HTML, JSON, and XML"
	if namedMediaTypes[mediaType] {
		return fmt.Errorf("unsupported content type %q — %s", mediaType, supported)
	}
	if top, _, ok := strings.Cut(mediaType, "/"); ok && topLevelMediaTypes[top] {
		return fmt.Errorf("unsupported content type %q — %s", top+"/*", supported)
	}
	return errors.New("unsupported content type — " + supported)
}
