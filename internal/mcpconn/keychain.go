package mcpconn

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// keychainService names every MCP item in the login keychain; the account
// is the Connection ID.
const keychainService = "dev.evie.mcp"

// maxCredentialsJSON keeps one `security -i` line under its ~4 KB limit
// once base64-encoded (story 0 measured 3,027 bytes).
const maxCredentialsJSON = 3000

// securityItemNotFound is security(1)'s exit status for a missing item.
const securityItemNotFound = 44

var (
	ErrNoCredentials       = errors.New("no credentials stored for this connection")
	ErrCredentialsTooLarge = errors.New("credentials too large")
	ErrKeychainUnavailable = errors.New("the Keychain is unavailable")
)

// SecurityRunner runs /usr/bin/security with stdin and reports its stdout
// and exit status. err is set only when the process could not run.
type SecurityRunner func(ctx context.Context, stdin string, args ...string) (stdout []byte, exitCode int, err error)

// RunSecurity is the production SecurityRunner.
func RunSecurity(ctx context.Context, stdin string, args ...string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.Bytes(), exit.ExitCode(), nil
	}
	if err != nil {
		return nil, 0, err
	}
	return out.Bytes(), 0, nil
}

// Keychain stores each connection's Credentials as one generic-password
// item. Secret material reaches security(1) only on stdin, as base64 inside
// a quoted `-w` argument of `security -i`, never in argv.
type Keychain struct {
	Run SecurityRunner
}

func (k Keychain) Save(ctx context.Context, id string, credentials Credentials) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("keychain: %q is not an MCP Connection ID", id)
	}
	encoded, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	if len(encoded) > maxCredentialsJSON {
		return fmt.Errorf("%w: %d bytes, the limit is %d", ErrCredentialsTooLarge, len(encoded), maxCredentialsJSON)
	}
	line := fmt.Sprintf("add-generic-password -U -s %s -a %s -w \"%s\"\n", keychainService, id, base64.StdEncoding.EncodeToString(encoded))
	_, code, err := k.Run(ctx, line, "-i")
	if err != nil || code != 0 {
		return unavailable("save", code, err)
	}
	return nil
}

func (k Keychain) Load(ctx context.Context, id string) (Credentials, error) {
	if !idPattern.MatchString(id) {
		return Credentials{}, fmt.Errorf("keychain: %q is not an MCP Connection ID", id)
	}
	out, code, err := k.Run(ctx, "", "find-generic-password", "-s", keychainService, "-a", id, "-w")
	if err == nil && code == securityItemNotFound {
		return Credentials{}, ErrNoCredentials
	}
	if err != nil || code != 0 {
		return Credentials{}, unavailable("load", code, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return Credentials{}, fmt.Errorf("%w: the stored item is not Evie's encoding", ErrKeychainUnavailable)
	}
	var credentials Credentials
	if err := json.Unmarshal(decoded, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("%w: the stored item is not Evie's encoding", ErrKeychainUnavailable)
	}
	return credentials, nil
}

// Delete removes the item; a missing item is already deleted.
func (k Keychain) Delete(ctx context.Context, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("keychain: %q is not an MCP Connection ID", id)
	}
	_, code, err := k.Run(ctx, "", "delete-generic-password", "-s", keychainService, "-a", id)
	if err == nil && (code == 0 || code == securityItemNotFound) {
		return nil
	}
	return unavailable("delete", code, err)
}

func unavailable(op string, code int, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrKeychainUnavailable, op, err)
	}
	return fmt.Errorf("%w: %s exited %d", ErrKeychainUnavailable, op, code)
}
