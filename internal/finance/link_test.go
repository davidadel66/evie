package finance

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type cliAccountLinkStub struct {
	link                  SpendingAccountLink
	startErr              error
	complete              func(context.Context, string) (SpendingAccountLinkResult, error)
	starts, reads, checks int
	cancels               int
	cancelContextErr      error
}

func (s *cliAccountLinkStub) StartAccountLink(context.Context) (SpendingAccountLink, error) {
	s.starts++
	if s.startErr != nil {
		return SpendingAccountLink{}, s.startErr
	}
	return s.link, nil
}

func (s *cliAccountLinkStub) InspectAccounts(context.Context) (SpendingAccountsReport, error) {
	s.reads++
	return SpendingAccountsReport{PendingLink: &s.link}, nil
}

func (s *cliAccountLinkStub) CompleteAccountLink(ctx context.Context, id string) (SpendingAccountLinkResult, error) {
	s.checks++
	if id != s.link.ID {
		return SpendingAccountLinkResult{}, errors.New("wrong saved connection")
	}
	return s.complete(ctx, id)
}

func (s *cliAccountLinkStub) CancelAccountLink(ctx context.Context, _ string) error {
	s.cancels++
	s.cancelContextErr = ctx.Err()
	return nil
}

func newCLIAccountLinkStub() *cliAccountLinkStub {
	return &cliAccountLinkStub{
		link: SpendingAccountLink{ID: "private-local-id", HostedURL: "https://secure.plaid.com/link/fixture", ExpiresAt: time.Now().Add(time.Hour)},
		complete: func(context.Context, string) (SpendingAccountLinkResult, error) {
			return SpendingAccountLinkResult{Status: "linked", InstitutionsLinked: 1}, nil
		},
	}
}

func TestLinkCLIResumesSavedConnectionAndPrintsOnlyPublicStatus(t *testing.T) {
	service := newCLIAccountLinkStub()
	service.complete = func(context.Context, string) (SpendingAccountLinkResult, error) {
		if service.checks == 1 {
			return SpendingAccountLinkResult{}, ErrSpendingLinkInProgress
		}
		if service.checks == 2 {
			return SpendingAccountLinkResult{Status: "pending"}, nil
		}
		return SpendingAccountLinkResult{Status: "linked", InstitutionsLinked: 1}, nil
	}
	var output bytes.Buffer
	if err := linkWithService(context.Background(), service, &output, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if service.starts != 1 || service.checks != 3 || service.cancels != 0 {
		t.Fatalf("unexpected service lifecycle: %+v", service)
	}
	if !strings.Contains(output.String(), service.link.HostedURL) || !strings.Contains(output.String(), "Bank connection saved") || strings.Contains(output.String(), service.link.ID) {
		t.Fatalf("unexpected CLI output: %q", output.String())
	}
}

func TestLinkCLIRecoversExpiredHostedURLBeforeStartingAnotherFlow(t *testing.T) {
	service := newCLIAccountLinkStub()
	service.startErr = ErrSpendingLinkInProgress
	service.link.ExpiresAt = time.Now().Add(-time.Minute)
	var output bytes.Buffer
	if err := linkWithService(context.Background(), service, &output, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if service.starts != 1 || service.reads != 1 || service.checks != 1 || strings.Contains(output.String(), service.link.HostedURL) {
		t.Fatalf("expired flow was not recovered safely: %+v, output=%q", service, output.String())
	}
}

func TestLinkCLIErrorsNeverExposeProviderDetails(t *testing.T) {
	for _, phase := range []string{"start", "complete"} {
		t.Run(phase, func(t *testing.T) {
			service := newCLIAccountLinkStub()
			providerErr := errors.New("secret-access-token secret-public-token raw-provider-response")
			if phase == "start" {
				service.startErr = providerErr
			} else {
				service.complete = func(context.Context, string) (SpendingAccountLinkResult, error) {
					return SpendingAccountLinkResult{}, providerErr
				}
			}
			var output bytes.Buffer
			err := linkWithService(context.Background(), service, &output, time.Millisecond)
			if !errors.Is(err, ErrSpendingLinkUnavailable) || strings.Contains(output.String()+err.Error(), "secret-") || strings.Contains(output.String()+err.Error(), "raw-provider") || service.cancels != 0 {
				t.Fatalf("provider error leaked or pending recovery lost: err=%v, output=%q, cancels=%d", err, output.String(), service.cancels)
			}
		})
	}
}

func TestLinkCLIInterruptCancelsWaitingWithoutWaitingForPollTimer(t *testing.T) {
	service := newCLIAccountLinkStub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.complete = func(context.Context, string) (SpendingAccountLinkResult, error) {
		cancel()
		return SpendingAccountLinkResult{Status: "pending"}, nil
	}
	var output bytes.Buffer
	err := linkWithService(ctx, service, &output, time.Hour)
	if !errors.Is(err, context.Canceled) || service.cancels != 1 || service.cancelContextErr != nil {
		t.Fatalf("interrupt cleanup: err=%v, cancels=%d, cleanup context=%v", err, service.cancels, service.cancelContextErr)
	}
}

func TestLinkCLITimeoutPreservesPendingRecovery(t *testing.T) {
	service := newCLIAccountLinkStub()
	service.complete = func(context.Context, string) (SpendingAccountLinkResult, error) {
		return SpendingAccountLinkResult{Status: "pending"}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	err := linkWithService(ctx, service, &output, time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) || service.cancels != 0 {
		t.Fatalf("timeout discarded pending recovery: err=%v, cancels=%d", err, service.cancels)
	}
}

func TestLinkCLITerminalResultsStopPolling(t *testing.T) {
	for _, status := range []string{"expired", "cancelled", "failed", "unknown"} {
		t.Run(status, func(t *testing.T) {
			service := newCLIAccountLinkStub()
			service.complete = func(context.Context, string) (SpendingAccountLinkResult, error) {
				return SpendingAccountLinkResult{Status: status}, nil
			}
			var output bytes.Buffer
			if err := linkWithService(context.Background(), service, &output, time.Hour); err == nil || service.checks != 1 || service.cancels != 0 {
				t.Fatalf("terminal result kept polling: err=%v, checks=%d, cancels=%d", err, service.checks, service.cancels)
			}
		})
	}
}

func TestLinkCLISavedSuccessWinsOverConcurrentInterrupt(t *testing.T) {
	service := newCLIAccountLinkStub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.complete = func(context.Context, string) (SpendingAccountLinkResult, error) {
		cancel()
		return SpendingAccountLinkResult{Status: "linked", InstitutionsLinked: 1}, nil
	}
	var output bytes.Buffer
	if err := linkWithService(ctx, service, &output, time.Hour); err != nil || service.cancels != 0 {
		t.Fatalf("saved connection misreported after interrupt: err=%v, cancels=%d", err, service.cancels)
	}
}
