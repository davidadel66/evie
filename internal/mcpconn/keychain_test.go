package mcpconn

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeSecurity records every /usr/bin/security invocation and answers like
// the real tool: items live in a map keyed by service and account, and a
// missing item exits 44.
type fakeSecurity struct {
	items map[string]string
	calls []fakeSecurityCall
	exit  int // forced exit code for every call when non-zero
}

type fakeSecurityCall struct {
	args  []string
	stdin string
}

func (f *fakeSecurity) run(_ context.Context, stdin string, args ...string) ([]byte, int, error) {
	f.calls = append(f.calls, fakeSecurityCall{args: append([]string(nil), args...), stdin: stdin})
	if f.exit != 0 {
		return nil, f.exit, nil
	}
	if f.items == nil {
		f.items = map[string]string{}
	}
	flag := func(fields []string, name string) string {
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == name {
				return fields[i+1]
			}
		}
		return ""
	}
	switch args[0] {
	case "-i":
		fields := strings.Fields(strings.TrimSpace(stdin))
		if len(fields) == 0 || fields[0] != "add-generic-password" {
			return nil, 1, nil
		}
		f.items[flag(fields, "-s")+"/"+flag(fields, "-a")] = strings.Trim(flag(fields, "-w"), `"`)
		return nil, 0, nil
	case "find-generic-password":
		value, ok := f.items[flag(args, "-s")+"/"+flag(args, "-a")]
		if !ok {
			return nil, 44, nil
		}
		return []byte(value + "\n"), 0, nil
	case "delete-generic-password":
		key := flag(args, "-s") + "/" + flag(args, "-a")
		if _, ok := f.items[key]; !ok {
			return nil, 44, nil
		}
		delete(f.items, key)
		return nil, 0, nil
	}
	return nil, 1, nil
}

func TestKeychainRoundTripsCredentialsOverStdinOnly(t *testing.T) {
	ctx := context.Background()
	fake := &fakeSecurity{}
	keychain := Keychain{Run: fake.run}
	want := Credentials{
		AccessToken: "secret-access-token", RefreshToken: "secret-refresh-token",
		Expiry: time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC), ClientID: "client-1", ClientSecret: "secret-client",
	}
	if err := keychain.Save(ctx, "mcp-square", want); err != nil {
		t.Fatal(err)
	}
	got, err := keychain.Load(ctx, "mcp-square")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	for _, call := range fake.calls {
		joined := strings.Join(call.args, " ")
		for _, secret := range []string{"secret-access-token", "secret-refresh-token", "secret-client", base64.StdEncoding.EncodeToString([]byte("secret"))[:6]} {
			if strings.Contains(joined, secret) {
				t.Fatalf("security argv %q carries secret material", joined)
			}
		}
	}
	save := fake.calls[0]
	if strings.Join(save.args, " ") != "-i" || !strings.HasPrefix(save.stdin, "add-generic-password -U -s dev.evie.mcp -a mcp-square -w \"") {
		t.Fatalf("save call = %q with stdin %q", save.args, save.stdin)
	}
	if strings.Contains(save.stdin, "secret-access-token") {
		t.Fatal("stdin carries the token unencoded")
	}
}

func TestKeychainMissingItemIsNoCredentials(t *testing.T) {
	keychain := Keychain{Run: (&fakeSecurity{}).run}
	if _, err := keychain.Load(context.Background(), "mcp-square"); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load() error = %v, want ErrNoCredentials", err)
	}
	if err := keychain.Delete(context.Background(), "mcp-square"); err != nil {
		t.Fatalf("Delete() of a missing item = %v, want nil", err)
	}
}

func TestKeychainRefusesCredentialsOverTheLineLimit(t *testing.T) {
	fake := &fakeSecurity{}
	keychain := Keychain{Run: fake.run}
	err := keychain.Save(context.Background(), "mcp-square", Credentials{ClientID: "c", AccessToken: strings.Repeat("a", 3001)})
	if !errors.Is(err, ErrCredentialsTooLarge) {
		t.Fatalf("Save() error = %v, want ErrCredentialsTooLarge", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("security ran %d times for an oversized value", len(fake.calls))
	}
}

func TestKeychainFailureIsUnavailable(t *testing.T) {
	keychain := Keychain{Run: (&fakeSecurity{exit: 51}).run}
	if _, err := keychain.Load(context.Background(), "mcp-square"); !errors.Is(err, ErrKeychainUnavailable) {
		t.Fatalf("Load() error = %v, want ErrKeychainUnavailable", err)
	}
	if err := keychain.Save(context.Background(), "mcp-square", Credentials{ClientID: "c"}); !errors.Is(err, ErrKeychainUnavailable) {
		t.Fatalf("Save() error = %v, want ErrKeychainUnavailable", err)
	}
}

func TestKeychainRejectsUnsafeIDs(t *testing.T) {
	fake := &fakeSecurity{}
	keychain := Keychain{Run: fake.run}
	for _, id := range []string{"square", "mcp-Square", "mcp-a b", `mcp-a"b`, "mcp-"} {
		if err := keychain.Save(context.Background(), id, Credentials{ClientID: "c"}); err == nil {
			t.Fatalf("Save(%q) succeeded", id)
		}
	}
	if len(fake.calls) != 0 {
		t.Fatal("security ran for an unsafe ID")
	}
}

// The real Keychain, opt-in: CI has none and the item lands in the owner's
// login keychain until the test deletes it.
func TestKeychainRealRoundTrip(t *testing.T) {
	if os.Getenv("EVIE_KEYCHAIN_TEST") != "1" {
		t.Skip("set EVIE_KEYCHAIN_TEST=1 to round-trip against the macOS Keychain")
	}
	ctx := context.Background()
	keychain := Keychain{Run: RunSecurity}
	id := "mcp-evie-test"
	t.Cleanup(func() { _ = keychain.Delete(ctx, id) })
	want := Credentials{AccessToken: strings.Repeat("a", 900), RefreshToken: strings.Repeat("r", 900), ClientID: "client", Expiry: time.Now().UTC().Truncate(time.Second)}
	if err := keychain.Save(ctx, id, want); err != nil {
		t.Fatal(err)
	}
	got, err := keychain.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("expiry = %v, want %v", got.Expiry, want.Expiry)
	}
	got.Expiry = want.Expiry
	if got != want {
		t.Fatal("real Keychain round trip changed the credentials")
	}
	if err := keychain.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := keychain.Load(ctx, id); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load() after Delete = %v, want ErrNoCredentials", err)
	}
}
