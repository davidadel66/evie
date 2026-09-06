package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type ConversationReader interface {
	ReadConversationUsage(context.Context, Period) (Summary, error)
}

type localCache struct {
	snapshot    LocalSnapshot
	at          *time.Time
	err         error
	busy        bool
	lastAttempt time.Time
}
type accountCache struct {
	source      Source
	busy        bool
	lastAttempt time.Time
}
type homeCollector struct {
	home    string
	index   *LocalIndex
	local   localCache
	account accountCache
}

// Service coordinates bounded background reads. A failed external source never
// prevents the owner's other usage sources from being inspected.
type Service struct {
	ctx           context.Context
	conversations ConversationReader
	binary        string
	mu            sync.Mutex
	homes         []*homeCollector
}

func NewService(ctx context.Context, conversations ConversationReader, binary string, homes []string) *Service {
	service := &Service{ctx: ctx, conversations: conversations, binary: binary}
	for _, home := range homes {
		service.homes = append(service.homes, &homeCollector{home: home, index: NewLocalIndex(home)})
	}
	return service
}
func (s *Service) InspectUsage(ctx context.Context, p Period) (Report, error) {
	if p.location == nil {
		return Report{}, ErrRange
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	s.refresh()
	result := Report{Period: p, Sources: []Source{}}
	s.mu.Lock()
	homes := make([]homeCollector, len(s.homes))
	for i, home := range s.homes {
		homes[i] = *home
	}
	s.mu.Unlock()
	seenAccounts := map[string]int{}
	for i, home := range homes {
		account := home.account.source
		if account.ID == "" {
			account = Source{ID: "codex-account-" + sourceKey(home.home), Kind: "codex-account", Label: "Codex account", State: "indexing", Coverage: "Account activity", Message: "Reading the current Codex account…"}
		}
		if previous, exists := seenAccounts[account.ID]; exists {
			if preferAccount(account, result.Sources[previous]) {
				result.Sources[previous] = account
			}
		} else {
			seenAccounts[account.ID] = len(result.Sources)
			result.Sources = append(result.Sources, account)
		}
		local := home.local
		source := Source{ID: "codex-local-" + sourceKey(home.home), Kind: "codex-local", Label: "Codex · Local (unassigned)", State: "partial", Coverage: "Available local per-response records · account unknown · legacy snapshots and deleted files excluded", CollectedAt: local.at}
		if len(s.homes) > 1 {
			source.Label = fmt.Sprintf("Codex · Local %d (unassigned)", i+1)
		}
		source.Message = "Historical account ownership and model attribution are unavailable. This overlaps account activity; do not add the totals."
		if local.busy || !local.snapshot.Complete {
			source.State = "indexing"
			source.Message = fmt.Sprintf("Reading local token records: %d of %d files scanned. Counts are partial until collection finishes.", local.snapshot.FinishedFiles, local.snapshot.Files)
		}
		if local.err != nil {
			source.State = "partial"
			source.Message = "Some local records could not be indexed. Available measurements are shown; collection will retry."
			if errors.Is(local.err, ErrLimit) {
				source.Message = "The local collection limit was reached. Counts cover only indexed per-response records."
			}
		}
		if local.snapshot.PendingFiles > 0 {
			source.Message += fmt.Sprintf(" %d unfinished file tails excluded until complete.", local.snapshot.PendingFiles)
		}
		if local.snapshot.InvalidRecords > 0 {
			source.Message += fmt.Sprintf(" %d invalid or conflicting records excluded.", local.snapshot.InvalidRecords)
		}
		if local.at != nil {
			summary, err := Summarize(p, local.snapshot.Observations)
			if err == nil {
				source.Summary = &summary
			} else {
				source.State = "unavailable"
				source.Message = "Local totals could not be aggregated safely. Narrow the date range."
			}
		}
		result.Sources = append(result.Sources, source)
	}
	now := time.Now().UTC()
	evie := Source{ID: "evie-conversation", Kind: "evie", Label: "Evie", State: "partial", Coverage: "Recorded conversation responses · all sessions", Message: "Compaction, extraction and unrecorded failed calls are excluded. Models identify requested, not billed, models.", CollectedAt: &now}
	if s.conversations != nil {
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		summary, err := s.conversations.ReadConversationUsage(readCtx, p)
		cancel()
		if err == nil {
			evie.Summary = &summary
		} else {
			evie.State = "unavailable"
			evie.Message = "Conversation usage could not be read. Try a narrower date range."
		}
	} else {
		evie.State = "unavailable"
		evie.Message = "Conversation usage is unavailable."
	}
	result.Sources = append(result.Sources, evie)
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	return result, nil
}

func preferAccount(candidate, previous Source) bool {
	rank := map[string]int{"ready": 4, "partial": 3, "stale": 2, "unavailable": 1}
	if rank[candidate.State] != rank[previous.State] {
		return rank[candidate.State] > rank[previous.State]
	}
	return candidate.CollectedAt != nil && (previous.CollectedAt == nil || candidate.CollectedAt.After(*previous.CollectedAt))
}
func (s *Service) refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	now := time.Now()
	for _, home := range s.homes {
		if !home.account.busy && now.Sub(home.account.lastAttempt) >= time.Minute {
			home.account.busy = true
			home.account.lastAttempt = now
			go s.readAccount(home)
		}
		interval := 30 * time.Second
		if !home.local.snapshot.Complete {
			interval = 3 * time.Second
		}
		if !home.local.busy && now.Sub(home.local.lastAttempt) >= interval {
			home.local.busy = true
			home.local.lastAttempt = now
			go s.readLocal(home)
		}
	}
}
func (s *Service) readAccount(home *homeCollector) {
	source, err := ReadCodexAccount(s.ctx, s.binary, home.home)
	s.mu.Lock()
	defer s.mu.Unlock()
	home.account.busy = false
	if err != nil {
		source = home.account.source
		if source.ID == "" {
			source = Source{ID: "codex-account-" + sourceKey(home.home), Kind: "codex-account", Label: "Codex account", Coverage: "Account activity"}
		}
		source.State = "unavailable"
		source.Message = "Codex account activity could not be refreshed. Check the existing Codex sign-in and executable."
		if source.Account != nil {
			source.State = "stale"
			source.Message = "Showing the last successful account snapshot; refresh is currently unavailable."
		}
	}
	home.account.source = source
}
func (s *Service) readLocal(home *homeCollector) {
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	var previousBytes int64 = -1
	for {
		snapshot, err := home.index.Scan(ctx, 128<<20)
		now := time.Now().UTC()
		done := err != nil || snapshot.Complete || snapshot.ScannedBytes == previousBytes
		s.mu.Lock()
		home.local.snapshot = snapshot
		home.local.at = &now
		home.local.err = err
		home.local.busy = !done
		s.mu.Unlock()
		if done {
			return
		}
		previousBytes = snapshot.ScannedBytes
	}
}
