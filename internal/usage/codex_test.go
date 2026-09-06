package usage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The test executable provides a real stdio peer, exercising subprocess
// isolation, notifications, request ordering, limits and cleanup at the seam.
func TestMain(m *testing.M) {
	mode := os.Getenv("EVIE_USAGE_TEST_PEER")
	if mode != "" {
		if mode == "hold-stdout" {
			time.Sleep(3 * time.Second)
			os.Exit(0)
		}
		if mode == "inherited-stdout" {
			executable, _ := os.Executable()
			child := exec.Command(executable)
			child.Env = []string{"EVIE_USAGE_TEST_PEER=hold-stdout"}
			child.Stdout = os.Stdout
			if child.Start() != nil {
				os.Exit(3)
			}
			time.Sleep(time.Minute)
			os.Exit(0)
		}
		if mode == "hang" {
			time.Sleep(time.Minute)
			os.Exit(0)
		}
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request struct {
				ID     *int   `json:"id"`
				Method string `json:"method"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &request)
			if request.ID == nil {
				continue
			}
			if mode == "error" {
				fmt.Fprintln(os.Stderr, "secret-stderr-canary")
				fmt.Printf(`{"id":%d,"error":{"message":"secret-error-canary"}}`+"\n", *request.ID)
				continue
			}
			var result any
			switch request.Method {
			case "initialize":
				result = map[string]any{}
			case "account/read":
				result = map[string]any{"account": map[string]any{"type": "chatgpt", "email": "example@example.test", "planType": "pro"}}
			case "account/rateLimits/read":
				result = map[string]any{"accountId": "account-one", "rateLimitsByLimitId": map[string]any{"codex": map[string]any{"primary": map[string]any{"usedPercent": 25, "windowDurationMins": 300, "resetsAt": 1788746400}}}, "rateLimitResetCredits": map[string]any{"secret": "must-not-return"}}
			case "account/usage/read":
				result = map[string]any{"summary": map[string]any{"lifetimeTokens": 9007199254740993}, "dailyUsageBuckets": []map[string]any{{"startDate": "2026-09-01", "tokens": 1500}, {"startDate": "2026-09-03", "tokens": 2500}}}
			default:
				os.Exit(3)
			}
			fmt.Println(`{"method":"harmless/notification","params":{}}`)
			data, _ := json.Marshal(map[string]any{"id": *request.ID, "result": result})
			fmt.Println(string(data))
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCodexAccountCancelsInheritedStdout(t *testing.T) {
	executable, _ := os.Executable()
	t.Setenv("EVIE_USAGE_TEST_PEER", "inherited-stdout")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := ReadCodexAccount(ctx, executable, t.TempDir())
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("inherited pipe blocked cancellation: %v", err)
	}
}
func TestCodexAccountReadsAllowlistedMetadataAndPreservesSparseDates(t *testing.T) {
	t.Setenv("EVIE_USAGE_TEST_PEER", "success")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := ReadCodexAccount(context.Background(), executable, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if source.State != "ready" || source.Account == nil || len(source.Account.Daily) != 2 || len(source.Account.Windows) != 1 || source.Account.Windows[0].UsedPercent != 25 {
		t.Fatalf("%+v", source)
	}
	if source.ID != "codex-account-"+sourceKey("account-one") {
		t.Fatal("account identity lost")
	}
	data, _ := json.Marshal(source)
	if strings.Contains(string(data), "must-not-return") || !strings.Contains(string(data), `"lifetimeTokens":"9007199254740993"`) {
		t.Fatal(string(data))
	}
}
func TestCodexAccountBoundsHungProcessAndRedactsErrors(t *testing.T) {
	executable, _ := os.Executable()
	t.Setenv("EVIE_USAGE_TEST_PEER", "error")
	if _, err := ReadCodexAccount(context.Background(), executable, t.TempDir()); err == nil || strings.Contains(err.Error(), "canary") {
		t.Fatalf("unsafe error: %v", err)
	}
	t.Setenv("EVIE_USAGE_TEST_PEER", "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := ReadCodexAccount(ctx, executable, t.TempDir())
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("hung peer not bounded: %v", err)
	}
}
