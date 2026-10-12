package mcpconn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// OAuth 2.1 for hosted MCP servers, run by Evie itself (story 0: the SDK's
// handler repeats registration and hides it): protected-resource and
// authorization-server discovery, Dynamic Client Registration, PKCE S256,
// the code exchange, refresh and revocation. Every request here carries no
// bearer token.

// maxOAuthBody bounds every discovery and token response.
const maxOAuthBody = 1 << 20

var resourceMetadataParam = regexp.MustCompile(`resource_metadata="([^"]+)"`)

// discover finds the OAuth metadata for a hosted MCP server URL.
func discover(ctx context.Context, client *http.Client, resourceURL string) (OAuthMetadata, error) {
	prmURL, err := probeResourceMetadata(ctx, client, resourceURL)
	if err != nil {
		return OAuthMetadata{}, err
	}
	candidates := []string{}
	if prmURL != "" {
		candidates = append(candidates, prmURL)
	}
	u, _ := url.Parse(resourceURL)
	origin := u.Scheme + "://" + u.Host
	if path := strings.TrimSuffix(u.Path, "/"); path != "" {
		candidates = append(candidates, origin+"/.well-known/oauth-protected-resource"+path)
	}
	candidates = append(candidates, origin+"/.well-known/oauth-protected-resource")
	var prm struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		Scopes               []string `json:"scopes_supported"`
	}
	found := false
	for _, candidate := range candidates {
		if err := getJSON(ctx, client, candidate, &prm); err == nil && len(prm.AuthorizationServers) > 0 {
			found = true
			break
		}
	}
	if !found {
		return OAuthMetadata{}, errors.New("the server does not publish OAuth protected-resource metadata")
	}
	if prm.Resource != "" && strings.TrimSuffix(prm.Resource, "/") != strings.TrimSuffix(resourceURL, "/") {
		return OAuthMetadata{}, fmt.Errorf("the server's metadata names resource %q, not %q", prm.Resource, resourceURL)
	}
	issuer := prm.AuthorizationServers[0]
	as, err := authorizationServerMetadata(ctx, client, issuer)
	if err != nil {
		return OAuthMetadata{}, err
	}
	resource := prm.Resource
	if resource == "" {
		resource = resourceURL
	}
	as.Resource, as.Scopes = resource, prm.Scopes
	return as, nil
}

// probeResourceMetadata sends one unauthenticated request and reads the
// resource_metadata URL from a 401's WWW-Authenticate header.
func probeResourceMetadata(ctx context.Context, client *http.Client, resourceURL string) (string, error) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"evie","version":"1"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resourceURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("the server could not be reached: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxOAuthBody))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return "", nil
	}
	if m := resourceMetadataParam.FindStringSubmatch(resp.Header.Get("WWW-Authenticate")); m != nil {
		if err := ValidateURL(m[1]); err != nil {
			return "", fmt.Errorf("the server's resource_metadata URL is not https: %q", m[1])
		}
		return m[1], nil
	}
	return "", nil
}

// authorizationServerMetadata reads RFC 8414 (then OpenID) metadata for an
// issuer and checks what Evie relies on.
func authorizationServerMetadata(ctx context.Context, client *http.Client, issuer string) (OAuthMetadata, error) {
	if err := ValidateURL(issuer); err != nil {
		return OAuthMetadata{}, fmt.Errorf("the authorization server %q is not https", issuer)
	}
	u, _ := url.Parse(issuer)
	origin, path := u.Scheme+"://"+u.Host, strings.TrimSuffix(u.Path, "/")
	candidates := []string{origin + "/.well-known/oauth-authorization-server" + path, origin + "/.well-known/openid-configuration" + path}
	if path != "" {
		candidates = append(candidates, origin+path+"/.well-known/openid-configuration")
	}
	var raw struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RevocationEndpoint    string   `json:"revocation_endpoint"`
		RegistrationEndpoint  string   `json:"registration_endpoint"`
		CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
		TokenAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
	}
	var lastErr error
	for _, candidate := range candidates {
		if lastErr = getJSON(ctx, client, candidate, &raw); lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return OAuthMetadata{}, fmt.Errorf("the authorization server publishes no metadata: %w", lastErr)
	}
	if strings.TrimSuffix(raw.Issuer, "/") != strings.TrimSuffix(issuer, "/") {
		return OAuthMetadata{}, fmt.Errorf("the authorization server metadata names issuer %q, not %q", raw.Issuer, issuer)
	}
	if !slices.Contains(raw.CodeChallengeMethods, "S256") {
		return OAuthMetadata{}, errors.New("the authorization server does not support PKCE S256")
	}
	for _, endpoint := range []string{raw.AuthorizationEndpoint, raw.TokenEndpoint} {
		if ValidateURL(endpoint) != nil {
			return OAuthMetadata{}, fmt.Errorf("the authorization server endpoint %q is not https", endpoint)
		}
	}
	for _, optional := range []*string{&raw.RevocationEndpoint, &raw.RegistrationEndpoint} {
		if *optional != "" && ValidateURL(*optional) != nil {
			*optional = ""
		}
	}
	return OAuthMetadata{Issuer: raw.Issuer, AuthorizationEndpoint: raw.AuthorizationEndpoint, TokenEndpoint: raw.TokenEndpoint,
		RevocationEndpoint: raw.RevocationEndpoint, RegistrationEndpoint: raw.RegistrationEndpoint, TokenAuthMethods: raw.TokenAuthMethods}, nil
}

// register runs Dynamic Client Registration as a public client.
func register(ctx context.Context, client *http.Client, meta OAuthMetadata, redirect string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"client_name":                "Evie",
		"redirect_uris":              []string{redirect},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.RegistrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	var registered struct {
		ClientID string `json:"client_id"`
	}
	if status, err := doJSON(client, req, &registered); err != nil || status/100 != 2 || registered.ClientID == "" {
		return "", fmt.Errorf("client registration failed (status %d): %v", status, err)
	}
	return registered.ClientID, nil
}

func authorizationURL(meta OAuthMetadata, clientID, redirect, state, challenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {meta.Resource},
	}
	if len(meta.Scopes) > 0 {
		q.Set("scope", strings.Join(meta.Scopes, " "))
	}
	sep := "?"
	if strings.Contains(meta.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return meta.AuthorizationEndpoint + sep + q.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// errInvalidGrant is an authorization server's OAuth refusal of the grant
// or the client: the owner has to sign in again. Any other failure leaves
// the stored tokens alone.
var errInvalidGrant = errors.New("the authorization server refused the grant")

func requestToken(ctx context.Context, client *http.Client, meta OAuthMetadata, credentials Credentials, form url.Values) (tokenResponse, error) {
	// RFC 8414: without a listed method, a confidential client uses HTTP
	// Basic. A public client sends only its client_id.
	basic := credentials.ClientSecret != "" &&
		(len(meta.TokenAuthMethods) == 0 || slices.Contains(meta.TokenAuthMethods, "client_secret_basic") || !slices.Contains(meta.TokenAuthMethods, "client_secret_post"))
	switch {
	case basic:
	case credentials.ClientSecret != "":
		form.Set("client_id", credentials.ClientID)
		form.Set("client_secret", credentials.ClientSecret)
	default:
		form.Set("client_id", credentials.ClientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basic {
		req.SetBasicAuth(url.QueryEscape(credentials.ClientID), url.QueryEscape(credentials.ClientSecret))
	}
	var token tokenResponse
	status, err := doJSON(client, req, &token)
	if err != nil {
		return tokenResponse{}, err
	}
	if (status == http.StatusBadRequest || status == http.StatusUnauthorized) &&
		slices.Contains([]string{"invalid_grant", "invalid_client", "unauthorized_client"}, token.Error) {
		return tokenResponse{}, fmt.Errorf("%w: %s", errInvalidGrant, oauthErrorText(token))
	}
	if status/100 != 2 || token.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("token request failed with status %d: %s", status, oauthErrorText(token))
	}
	if token.TokenType != "" && !strings.EqualFold(token.TokenType, "bearer") {
		return tokenResponse{}, fmt.Errorf("token type %q is not bearer", token.TokenType)
	}
	return token, nil
}

func exchangeCode(ctx context.Context, client *http.Client, meta OAuthMetadata, credentials Credentials, code, verifier, redirect string) (tokenResponse, error) {
	return requestToken(ctx, client, meta, credentials, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect},
		"code_verifier": {verifier}, "resource": {meta.Resource},
	})
}

func refreshToken(ctx context.Context, client *http.Client, meta OAuthMetadata, credentials Credentials) (tokenResponse, error) {
	return requestToken(ctx, client, meta, credentials, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {credentials.RefreshToken}, "resource": {meta.Resource},
	})
}

// revoke asks the server to forget a token (RFC 7009); best effort.
func revoke(ctx context.Context, client *http.Client, meta OAuthMetadata, credentials Credentials, token, hint string) {
	if meta.RevocationEndpoint == "" || token == "" {
		return
	}
	form := url.Values{"token": {token}, "token_type_hint": {hint}, "client_id": {credentials.ClientID}}
	if credentials.ClientSecret != "" {
		form.Set("client_secret", credentials.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.RevocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resp, err := client.Do(req); err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxOAuthBody))
		resp.Body.Close()
	}
}

func (t tokenResponse) credentials(previous Credentials, now time.Time) Credentials {
	next := previous
	next.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		next.RefreshToken = t.RefreshToken
	}
	next.Expiry = time.Time{}
	if t.ExpiresIn > 0 {
		next.Expiry = now.Add(time.Duration(t.ExpiresIn) * time.Second)
	}
	return next
}

func oauthErrorText(t tokenResponse) string {
	if t.Error == "" {
		return "no error code"
	}
	if t.Description == "" {
		return t.Error
	}
	return t.Error + ": " + t.Description
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	status, err := doJSON(client, req, out)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s answered %d", endpoint, status)
	}
	return nil
}

func doJSON(client *http.Client, req *http.Request, out any) (int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuthBody))
	if err != nil {
		return resp.StatusCode, err
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, out); err != nil && resp.StatusCode/100 == 2 {
			return resp.StatusCode, fmt.Errorf("the response is not JSON: %w", err)
		}
	}
	return resp.StatusCode, nil
}
