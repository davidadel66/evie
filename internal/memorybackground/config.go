// Package memorybackground is the always-on background memory pass
// (docs/memory-architecture.md, Stage 4 and decision 4): after a root
// conversation goes idle, or every N new owner messages, it reads only the
// owner's new messages, extracts candidate memories, compares each with the
// most similar existing memories, and queues ADD / UPDATE / INVALIDATE
// proposals (Mem0's operations; NOOP queues nothing) in the owner's review
// inbox. It never applies memory.
package memorybackground

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// DefaultModel is pinned: a dated, non-alias OpenRouter model. A floating
// alias such as "~deepseek/deepseek-flash-latest" could change behavior
// without a configuration change.
const DefaultModel = "deepseek/deepseek-v4.1-flash"

// Providers.
const (
	ProviderOpenRouter = "openrouter"
	ProviderLocal      = "local"
)

// Conservative defaults (decision record 2026-10-03, Stage 4). A pass sends
// at most 30 owner messages and 24 KiB of owner text, so the extraction and
// decision calls together stay well inside the 32,000-token pass budget.
// The daily caps bind first: about 300,000 tokens or $0.25 a day.
const (
	DefaultIdle              = 10 * time.Minute
	DefaultTurns             = 20
	DefaultPassTokens        = 32_000
	DefaultDailyTokens       = 300_000
	DefaultDailyMicroUSD     = 250_000
	DefaultBatchMessages     = 30
	DefaultBatchBytes        = 24 << 10
	DefaultMessageBytes      = 4 << 10
	DefaultTick              = time.Minute
	DefaultSessionsPerTick   = 4
	DefaultCandidateLimit    = 8
	DefaultSimilarLimit      = 8
	DefaultExtractTokens     = 4_000
	DefaultDecideTokens      = 3_000
	DefaultMalformedSkip     = 3
	DefaultCommitWait        = 5 * time.Second
	DefaultCommitAttempts    = 12
	DefaultCallTimeout       = 2 * time.Minute
	DefaultMaxBackoff        = time.Hour
	DefaultBaseBackoff       = time.Minute
	DefaultProviderRetries   = 2
	DefaultProviderRetryBase = 2 * time.Second
)

// Pricing is USD per million tokens. A local model costs nothing.
type Pricing struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
	Source           string  `json:"source"`
}

// FallbackPricing is used when the provider catalog cannot be read. It is
// deliberately above DeepSeek V4.1 Flash's catalog price ($0.30 / $1.20 per
// million on 2026-10-03), so an unknown price can only make the USD cap bind
// sooner.
var FallbackPricing = Pricing{InputPerMillion: 2, OutputPerMillion: 8, Source: "fallback"}

// MicroUSD is the cost of a token count in millionths of a dollar, rounded
// up so the cap is never under-counted.
func (p Pricing) MicroUSD(input, output int64) int64 {
	return int64(math.Ceil(float64(input)*p.InputPerMillion + float64(output)*p.OutputPerMillion))
}

// Config is the resolved configuration. When Enabled is false, Reason says
// why in words the owner can act on.
type Config struct {
	Enabled          bool
	Reason           string
	Provider         string
	Model            string
	LocalConfigPath  string
	Idle             time.Duration
	Turns            int
	PassTokens       int64
	DailyTokens      int64
	DailyMicroUSD    int64
	BatchMessages    int
	BatchBytes       int
	MessageBytes     int
	Tick             time.Duration
	SessionsPerTick  int
	CandidateLimit   int
	SimilarLimit     int
	ExtractTokens    int64
	DecideTokens     int64
	MalformedSkip    int
	CommitWait       time.Duration
	CommitAttempts   int
	CallTimeout      time.Duration
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
	ProviderRetries  int
	ProviderRetryGap time.Duration
}

// Defaults returns a disabled configuration with every bound at its default.
func Defaults() Config {
	return Config{
		Model: DefaultModel, Idle: DefaultIdle, Turns: DefaultTurns, PassTokens: DefaultPassTokens,
		DailyTokens: DefaultDailyTokens, DailyMicroUSD: DefaultDailyMicroUSD, BatchMessages: DefaultBatchMessages,
		BatchBytes: DefaultBatchBytes, MessageBytes: DefaultMessageBytes, Tick: DefaultTick, SessionsPerTick: DefaultSessionsPerTick,
		CandidateLimit: DefaultCandidateLimit, SimilarLimit: DefaultSimilarLimit, ExtractTokens: DefaultExtractTokens,
		DecideTokens: DefaultDecideTokens, MalformedSkip: DefaultMalformedSkip, CommitWait: DefaultCommitWait,
		CommitAttempts: DefaultCommitAttempts, CallTimeout: DefaultCallTimeout, BaseBackoff: DefaultBaseBackoff,
		MaxBackoff: DefaultMaxBackoff, ProviderRetries: DefaultProviderRetries, ProviderRetryGap: DefaultProviderRetryBase,
	}
}

// Environment variables. EVIE_MEMORY_BACKGROUND is empty or "on" (automatic:
// remote when EVIE_REMOTE_MEMORY=on, else local when a local extractor is
// configured), "remote", "local" or "off".
const (
	EnvMode        = "EVIE_MEMORY_BACKGROUND"
	EnvModel       = "EVIE_MEMORY_BACKGROUND_MODEL"
	EnvIdle        = "EVIE_MEMORY_BACKGROUND_IDLE_MINUTES"
	EnvTurns       = "EVIE_MEMORY_BACKGROUND_TURNS"
	EnvPassTokens  = "EVIE_MEMORY_BACKGROUND_PASS_TOKENS"
	EnvDailyTokens = "EVIE_MEMORY_BACKGROUND_DAILY_TOKENS"
	EnvDailyUSD    = "EVIE_MEMORY_BACKGROUND_DAILY_USD"
	EnvRemote      = "EVIE_REMOTE_MEMORY"
	EnvAPIKey      = "OPENROUTER_API_KEY"
	EnvLocal       = "EVIE_COMPILER_CONFIG"
)

// FromEnv resolves the configuration from the environment. It never fails:
// any problem disables the pass with a reason, so a bad setting cannot stop
// Evie from starting.
func FromEnv(getenv func(string) string) Config {
	c := Defaults()
	disable := func(reason string) Config {
		c.Enabled, c.Reason = false, reason
		return c
	}
	var err error
	if c.Idle, err = minutesEnv(getenv, EnvIdle, c.Idle); err != nil {
		return disable(err.Error())
	}
	if c.Turns, err = intEnv(getenv, EnvTurns, c.Turns, 1, 1000); err != nil {
		return disable(err.Error())
	}
	var pass, daily int
	if pass, err = intEnv(getenv, EnvPassTokens, int(c.PassTokens), DefaultPassTokens, 1_000_000); err != nil {
		return disable(err.Error())
	}
	if daily, err = intEnv(getenv, EnvDailyTokens, int(c.DailyTokens), DefaultPassTokens, 100_000_000); err != nil {
		return disable(err.Error())
	}
	c.PassTokens, c.DailyTokens = int64(pass), int64(daily)
	if raw := strings.TrimSpace(getenv(EnvDailyUSD)); raw != "" {
		usd, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil || usd < 0 || usd > 100 || math.IsNaN(usd) {
			return disable(EnvDailyUSD + " must be a dollar amount from 0 to 100")
		}
		c.DailyMicroUSD = int64(math.Round(usd * 1_000_000))
	}
	if model := strings.TrimSpace(getenv(EnvModel)); model != "" {
		c.Model = model
	}
	mode := strings.ToLower(strings.TrimSpace(getenv(EnvMode)))
	remoteAllowed := getenv(EnvRemote) == "on"
	local := strings.TrimSpace(getenv(EnvLocal))
	switch mode {
	case "off":
		return disable("turned off (" + EnvMode + "=off)")
	case "local":
		if local == "" {
			return disable(EnvMode + "=local needs " + EnvLocal + " (a pinned local extractor configuration)")
		}
		c.Provider, c.LocalConfigPath = ProviderLocal, local
	case "", "on", "remote":
		switch {
		case remoteAllowed && strings.TrimSpace(getenv(EnvAPIKey)) != "":
			if err := ValidateRemoteModel(c.Model); err != nil {
				return disable(err.Error())
			}
			c.Provider = ProviderOpenRouter
		case remoteAllowed:
			return disable("needs " + EnvAPIKey + " to reach " + c.Model)
		case mode != "remote" && local != "":
			c.Provider, c.LocalConfigPath = ProviderLocal, local
		default:
			return disable("needs " + EnvRemote + "=on to send your messages to " + c.Model + " through OpenRouter, or " + EnvLocal + " for a local model")
		}
	default:
		return disable(EnvMode + " must be on, off, remote or local")
	}
	c.Enabled = true
	return c
}

// ValidateRemoteModel accepts only a pinned OpenRouter model ID: vendor/model,
// never a "~" latest alias, a "-latest"/":latest" name, OpenRouter's auto
// router, or the ":batch" variant. OpenRouter serves ":batch" only through
// its asynchronous Batch API (submit, then poll), which this pass does not
// use; it is therefore refused rather than failing on every request.
func ValidateRemoteModel(model string) error {
	lower := strings.ToLower(model)
	vendor, name, ok := strings.Cut(model, "/")
	switch {
	case !ok || vendor == "" || name == "" || strings.ContainsAny(model, " \t\n") || strings.Count(model, "/") != 1:
		return fmt.Errorf("%s must be an OpenRouter model ID such as %s", EnvModel, DefaultModel)
	case strings.HasPrefix(model, "~") || strings.Contains(lower, "latest") || strings.HasPrefix(lower, "openrouter/"):
		return fmt.Errorf("%s must be pinned, not a floating alias (%s)", EnvModel, model)
	case strings.HasSuffix(lower, ":batch"):
		return errors.New("the :batch variant needs OpenRouter's asynchronous Batch API, which the background pass does not use yet; choose " + strings.TrimSuffix(model, ":batch"))
	}
	return nil
}

func minutesEnv(getenv func(string) string, name string, fallback time.Duration) (time.Duration, error) {
	minutes, err := intEnv(getenv, name, int(fallback/time.Minute), 1, 24*60)
	return time.Duration(minutes) * time.Minute, err
}

func intEnv(getenv func(string) string, name string, fallback, low, high int) (int, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < low || value > high {
		return 0, fmt.Errorf("%s must be a whole number from %d to %d", name, low, high)
	}
	return value, nil
}
