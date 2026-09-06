package usage

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrCodexUnavailable = errors.New("Codex account activity is unavailable")

type accountUsageResponse struct {
	Summary struct {
		Lifetime *int64 `json:"lifetimeTokens"`
	} `json:"summary"`
	Daily []struct {
		Date   string `json:"startDate"`
		Tokens *int64 `json:"tokens"`
	} `json:"dailyUsageBuckets"`
}
type limitWindow struct {
	Used    *float64 `json:"usedPercent"`
	Minutes int      `json:"windowDurationMins"`
	Resets  int64    `json:"resetsAt"`
}
type limitSnapshot struct {
	ID        string       `json:"limitId"`
	Name      string       `json:"limitName"`
	Primary   *limitWindow `json:"primary"`
	Secondary *limitWindow `json:"secondary"`
}
type limitsResponse struct {
	AccountID string                   `json:"accountId"`
	Legacy    limitSnapshot            `json:"rateLimits"`
	Buckets   map[string]limitSnapshot `json:"rateLimitsByLimitId"`
}

// ReadCodexAccount runs only allowlisted account reads in an isolated stdio
// process. It never forwards raw errors, stderr, reset credits or credentials.
func ReadCodexAccount(ctx context.Context, binary, home string) (Source, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server", "--stdio", "-c", "mcp_servers={}", "-c", "analytics.enabled=false", "-c", "features.apps=false")
	cmd.Dir = os.TempDir()
	cmd.WaitDelay = time.Second
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "CODEX_HOME=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Source{}, ErrCodexUnavailable
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Source{}, ErrCodexUnavailable
	}
	defer stdout.Close()
	if err := cmd.Start(); err != nil {
		return Source{}, ErrCodexUnavailable
	}
	stopClosing := context.AfterFunc(ctx, func() {
		_ = stdin.Close()
		_ = stdout.Close()
	})
	defer stopClosing()
	defer func() {
		_ = stdin.Close()
		_ = stdout.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	scanner := bufio.NewScanner(io.LimitReader(stdout, 4<<20))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	encoder := json.NewEncoder(stdin)
	call := func(id int, method string, params any, dst any) error {
		if err := encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return ErrCodexUnavailable
		}
		for messages := 0; messages < 128 && scanner.Scan(); messages++ {
			var envelope struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
				Method string          `json:"method"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
				return ErrCodexUnavailable
			}
			if envelope.ID == nil {
				continue
			}
			if envelope.Method != "" || *envelope.ID != id || (len(envelope.Error) > 0 && string(envelope.Error) != "null") {
				return ErrCodexUnavailable
			}
			if dst != nil && json.Unmarshal(envelope.Result, dst) != nil {
				return ErrCodexUnavailable
			}
			return nil
		}
		return ErrCodexUnavailable
	}
	if err := call(0, "initialize", map[string]any{"clientInfo": map[string]string{"name": "evie_usage", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true, "requestAttestation": false}}, nil); err != nil {
		return Source{}, err
	}
	if encoder.Encode(map[string]string{"method": "initialized"}) != nil {
		return Source{}, ErrCodexUnavailable
	}
	var identity struct {
		Account *struct {
			Type  string `json:"type"`
			Email string `json:"email"`
			Plan  string `json:"planType"`
		} `json:"account"`
	}
	if err := call(1, "account/read", map[string]bool{"refreshToken": false}, &identity); err != nil {
		return Source{}, err
	}
	now := time.Now().UTC()
	source := Source{ID: "codex-account-" + sourceKey(home), Kind: "codex-account", Label: "Codex account", State: "unavailable", Coverage: "Account activity · provider calendar dates; coverage and missing days are not specified", CollectedAt: &now}
	if identity.Account == nil || identity.Account.Type != "chatgpt" {
		source.Message = "Sign in to Codex with ChatGPT to read account activity."
		return source, nil
	}
	source.Label = "Codex · " + identity.Account.Email
	if identity.Account.Email == "" {
		source.Label = "Codex account"
	}
	account := &Account{Plan: identity.Account.Plan, Daily: []AccountDay{}, Windows: []Window{}}
	source.Account = account
	var limits limitsResponse
	if err := call(2, "account/rateLimits/read", map[string]any{}, &limits); err == nil {
		if limits.AccountID != "" {
			source.ID = "codex-account-" + sourceKey(limits.AccountID)
		}
		if len(limits.Buckets) == 0 {
			limits.Buckets = map[string]limitSnapshot{"codex": limits.Legacy}
		}
		for key, bucket := range limits.Buckets {
			name := bucket.Name
			if name == "" {
				name = key
			}
			for _, window := range []*limitWindow{bucket.Primary, bucket.Secondary} {
				if window == nil || window.Used == nil || *window.Used < 0 || *window.Used > 100 || window.Minutes <= 0 || window.Resets <= 0 {
					continue
				}
				account.Windows = append(account.Windows, Window{Name: name, UsedPercent: *window.Used, Minutes: window.Minutes, ResetsAt: time.Unix(window.Resets, 0).UTC()})
			}
		}
		sort.Slice(account.Windows, func(i, j int) bool {
			a, b := account.Windows[i], account.Windows[j]
			if a.Name == b.Name {
				return a.Minutes < b.Minutes
			}
			return a.Name < b.Name
		})
	}
	var activity accountUsageResponse
	if err := call(3, "account/usage/read", map[string]any{}, &activity); err != nil {
		source.Message = "Account token activity could not be read. Allowances are a separate snapshot."
		return source, nil
	}
	account.LifetimeTokens = activity.Summary.Lifetime
	if account.LifetimeTokens != nil && *account.LifetimeTokens < 0 {
		account.LifetimeTokens = nil
	}
	dates := map[string]int64{}
	for _, day := range activity.Daily {
		if _, err := time.Parse(time.DateOnly, day.Date); err != nil || day.Tokens == nil || *day.Tokens < 0 {
			continue
		}
		dates[day.Date] = *day.Tokens
	}
	for date, tokens := range dates {
		account.Daily = append(account.Daily, AccountDay{Date: date, Tokens: tokens})
	}
	sort.Slice(account.Daily, func(i, j int) bool { return account.Daily[i].Date < account.Daily[j].Date })
	source.State = "ready"
	source.Message = "Account totals do not include an input/cache breakdown. Missing provider dates remain unknown."
	if activity.Daily == nil {
		source.State = "partial"
		source.Message = "Daily account activity is unavailable; any lifetime total and allowances are shown separately."
	}
	return source, nil
}
func sourceKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func CodexBinary() string {
	if binary := os.Getenv("EVIE_CODEX_BIN"); binary != "" {
		return binary
	}
	// The desktop bundle may work even when an older PATH shim is broken.
	bundled := "/Applications/ChatGPT.app/Contents/Resources/codex"
	if info, err := os.Stat(bundled); err == nil && !info.IsDir() {
		return bundled
	}
	if binary, err := exec.LookPath("codex"); err == nil {
		return binary
	}
	return "codex"
}
func CodexHomes() ([]string, error) {
	values := filepath.SplitList(os.Getenv("EVIE_CODEX_HOMES"))
	if len(values) == 0 {
		home := os.Getenv("CODEX_HOME")
		if home == "" {
			userHome, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			home = filepath.Join(userHome, ".codex")
		}
		values = []string{home}
	}
	if len(values) > 4 {
		return nil, fmt.Errorf("at most four Codex homes may be configured")
	}
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("Codex homes must be absolute paths")
		}
		value = filepath.Clean(value)
		if !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result, nil
}
